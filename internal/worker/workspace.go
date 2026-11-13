package worker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neko233-com/buildworld/internal/processtree"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
)

// prepareWorkspace serializes a repository/branch for its entire build. This
// preserves engine caches while independent repositories execute concurrently.
func (e *Executor) prepareWorkspace(ctx context.Context, req *pb.BuildRequest) (string, func(), error) {
	root := e.environment.WorkspacesRoot()
	if req.RepoUrl == "" {
		dir, err := os.MkdirTemp(root, "buildworld-worker-")
		return dir, func() { _ = os.RemoveAll(dir) }, err
	}
	digest := sha256.Sum256([]byte(req.RepoUrl + "\n" + req.Branch))
	lock, err := os.OpenFile(filepath.Join(root, fmt.Sprintf(".reuse-%x.lock", digest[:8])), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", nil, err
	}
	for !tryFileLock(lock) {
		select {
		case <-ctx.Done():
			lock.Close()
			return "", nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	release := func() { unlockFile(lock); lock.Close() }
	dir := filepath.Join(root, fmt.Sprintf("reuse-%x", digest[:8]))
	// Adopt a pre-existing reuse checkout only by its Git origin and symbolic
	// branch identity, never by project display name or a caller supplied path.
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "reuse-") {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		origin, originErr := processtree.CommandContext(ctx, "git", "-C", candidate, "config", "--get", "remote.origin.url").Output()
		branch, branchErr := processtree.CommandContext(ctx, "git", "-C", candidate, "symbolic-ref", "--short", "HEAD").Output()
		if originErr == nil && branchErr == nil && strings.TrimSpace(string(origin)) == req.RepoUrl && strings.TrimSpace(string(branch)) == req.Branch {
			dir = candidate
			break
		}
	}
	fail := func() (string, func(), error) {
		release()
		return "", nil, fmt.Errorf("worker checkout failed; repository and commit remain fixed")
	}
	if _, err = os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if err = processtree.CommandContext(ctx, "git", "clone", "--no-checkout", "--", req.RepoUrl, dir).Run(); err != nil {
			return fail()
		}
	}
	if err = processtree.CommandContext(ctx, "git", "-C", dir, "fetch", "--no-tags", "origin", req.Branch).Run(); err != nil {
		return fail()
	}
	if err = processtree.CommandContext(ctx, "git", "-C", dir, "reset", "--hard").Run(); err != nil {
		return fail()
	}
	commit := req.CommitSha
	if commit == "" {
		commit = "FETCH_HEAD"
	}
	if err = processtree.CommandContext(ctx, "git", "-C", dir, "checkout", "-B", req.Branch, commit).Run(); err != nil {
		return fail()
	}
	if err = processtree.CommandContext(ctx, "git", "-C", dir, "reset", "--hard", commit).Run(); err != nil {
		return fail()
	}
	return dir, release, nil
}
