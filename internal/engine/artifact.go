package engine

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/neko233-com/buildworld/internal/store"
)

// CollectJenkins archives regular files using workspace-relative Ant patterns.
// It preserves relative paths and fails instead of silently omitting files.
func (am *ArtifactManager) CollectJenkins(buildID int64, workspace, patterns string, allowEmpty bool) (int, error) {
	build, err := am.store.GetBuild(buildID)
	if err != nil {
		return 0, err
	}
	var matchers []*jenkinsPathPattern
	for _, raw := range strings.Split(patterns, ",") {
		pattern := strings.TrimSpace(filepath.ToSlash(raw))
		if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, `\`) {
			return 0, errors.New("artifact patterns must be nonempty workspace-relative paths")
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." || part == "." || strings.ContainsRune(part, '\x00') {
				return 0, errors.New("artifact patterns cannot traverse the workspace")
			}
		}
		matcher, err := compileJenkinsPathPattern(pattern, true)
		if err != nil {
			return 0, err
		}
		matchers = append(matchers, matcher)
	}
	destination := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
	count := 0
	err = filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == workspace {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".svn", ".hg", ".bzr", "CVS":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		matched := false
		for _, matcher := range matchers {
			if matcher.matches(name) {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("artifact %q is not a regular file", name)
		}
		if err := am.saveJenkinsFile(buildID, name, path, filepath.Join(destination, rel)); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	if count == 0 && !allowEmpty {
		return 0, errors.New("archiveArtifacts matched no files")
	}
	return count, nil
}

func (am *ArtifactManager) saveJenkinsFile(buildID int64, name, source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = am.store.CreateArtifact(buildID, name, destination, size, fmt.Sprintf("%x", hash.Sum(nil)), contentType)
	return err
}

type ArtifactManager struct {
	store *store.Store
	root  string
}

func NewArtifactManager(s *store.Store, root string) *ArtifactManager {
	if root == "" {
		root = "./artifacts"
	}
	os.MkdirAll(root, 0o755)
	return &ArtifactManager{store: s, root: root}
}

func (am *ArtifactManager) Save(buildID int64, name string, reader io.Reader) (*store.Artifact, error) {
	build, err := am.store.GetBuild(buildID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	safeName := filepath.Base(name)
	destPath := filepath.Join(dir, safeName)
	f, err := os.Create(destPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hash := sha256.New()
	w := io.MultiWriter(f, hash)
	size, err := io.Copy(w, reader)
	if err != nil {
		return nil, err
	}
	sha := fmt.Sprintf("%x", hash.Sum(nil))
	ct := mime.TypeByExtension(filepath.Ext(safeName))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return am.store.CreateArtifact(buildID, safeName, destPath, size, sha, ct)
}

func (am *ArtifactManager) Open(id int64) (io.ReadCloser, *store.Artifact, error) {
	a, err := am.store.GetArtifact(id)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, nil, err
	}
	_ = am.store.IncArtifactDownloads(id)
	return f, a, nil
}

func (am *ArtifactManager) ListByBuild(buildID int64) ([]*store.Artifact, error) {
	return am.store.ListArtifactsByBuild(buildID)
}

func (am *ArtifactManager) DeleteByBuild(buildID int64) error {
	arts, err := am.store.ListArtifactsByBuild(buildID)
	if err != nil {
		return err
	}
	for _, a := range arts {
		os.Remove(a.Path)
	}
	build, err := am.store.GetBuild(buildID)
	if err == nil {
		dir := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
		os.RemoveAll(dir)
	}
	return am.store.DeleteArtifactsByBuild(buildID)
}

func (am *ArtifactManager) CollectGlob(buildID int64, workspace string, patterns []string) error {
	build, err := am.store.GetBuild(buildID)
	if err != nil {
		return err
	}
	var matched []string
	for _, pattern := range patterns {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(workspace, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			matched = append(matched, m)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	dir := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, src := range matched {
		rel, err := filepath.Rel(workspace, src)
		if err != nil {
			rel = filepath.Base(src)
		}
		rel = filepath.ToSlash(rel)
		rel = strings.ReplaceAll(rel, "/", "_")
		safeName := filepath.Base(rel)
		destPath := filepath.Join(dir, safeName)
		if err := copyFile(src, destPath); err != nil {
			continue
		}
		info, _ := os.Stat(destPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		sha := fileSHA256(destPath)
		ct := mime.TypeByExtension(filepath.Ext(safeName))
		if ct == "" {
			ct = "application/octet-stream"
		}
		am.store.CreateArtifact(buildID, safeName, destPath, size, sha, ct)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil))
}
