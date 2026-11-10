package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neko233-com/buildworld/internal/processtree"
)

type Client struct{}

var nonInteractiveEnvironmentDefaults = []string{
	"GIT_TERMINAL_PROMPT=0",
	"GCM_INTERACTIVE=Never",
	"GIT_HTTP_LOW_SPEED_LIMIT=1",
	"GIT_HTTP_LOW_SPEED_TIME=60",
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Clone(url, dest string) error {
	return c.CloneContext(context.Background(), url, dest)
}

func (c *Client) CloneContext(ctx context.Context, url, dest string) error {
	cmd := processtree.CommandContext(ctx, "git", "clone", url, dest)
	cmd.Env = NonInteractiveEnvironment(cmd.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w, output: %s", err, output)
	}
	return nil
}

// PrepareExistingWorkspaceContext initializes a nonempty workspace or verifies
// that its existing repository is rooted there and points at the expected SCM.
func (c *Client) PrepareExistingWorkspaceContext(ctx context.Context, repoPath, url, revision string) error {
	gitPath := filepath.Join(repoPath, ".git")
	info, err := os.Lstat(gitPath)
	if os.IsNotExist(err) {
		format := "sha1"
		if len(revision) == 64 {
			format = "sha256"
		}
		for _, args := range [][]string{
			{"init", "--object-format=" + format},
			{"config", "--local", "--", "remote.origin.url", url},
			{"config", "--local", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"},
			{"fetch", "--tags", "origin"},
		} {
			if _, err := workspaceGitCommand(ctx, repoPath, args...); err != nil {
				return fmt.Errorf("initialize SCM workspace: %w", err)
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("SCM workspace .git must be a directory")
	}
	top, err := workspaceGitCommand(ctx, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("resolve SCM workspace: %w", err)
	}
	workspaceInfo, err := os.Stat(repoPath)
	if err != nil {
		return err
	}
	topInfo, err := os.Stat(strings.TrimSpace(top))
	if err != nil || !os.SameFile(workspaceInfo, topInfo) {
		return fmt.Errorf("SCM repository must be rooted in the build workspace")
	}
	origin, err := workspaceGitCommand(ctx, repoPath, "config", "--local", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(origin) != url {
		return fmt.Errorf("SCM workspace origin differs from the configured repository")
	}
	return nil
}

func workspaceGitCommand(ctx context.Context, repoPath string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", repoPath}, args...)
	cmd := processtree.CommandContext(ctx, "git", commandArgs...)
	cmd.Env = NonInteractiveEnvironment(cmd.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, output)
	}
	return string(output), nil
}

// ForceCheckoutRevisionContext restores tracked files to the selected SCM
// revision while retaining unrelated untracked workspace files.
func (c *Client) ForceCheckoutRevisionContext(ctx context.Context, repoPath, branch, revision string) error {
	if revision == "" || strings.HasPrefix(revision, "-") {
		return fmt.Errorf("revision is required")
	}
	if strings.HasPrefix(branch, "refs/tags/") {
		_, err := workspaceGitCommand(ctx, repoPath, "checkout", "--force", "--detach", revision, "--")
		return err
	}
	branch = strings.TrimPrefix(branch, "refs/heads/")
	if branch == "" || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("branch is required")
	}
	_, err := workspaceGitCommand(ctx, repoPath, "checkout", "--force", "-B", branch, revision, "--")
	return err
}

func (c *Client) CloneWithSSH(url, dest, username, password string) error {
	return c.CloneWithSSHContext(context.Background(), url, dest, username, password)
}

func (c *Client) CloneWithSSHContext(ctx context.Context, url, dest, username, password string) error {
	// Create credential helper for password authentication
	credHelper, err := createSSHCredentialHelper(username, password)
	if err != nil {
		return fmt.Errorf("create credential helper: %w", err)
	}
	defer os.Remove(credHelper)

	cmd := processtree.CommandContext(ctx, "git", "clone", url, dest)
	cmd.Env = append(NonInteractiveEnvironment(os.Environ()),
		"GIT_ASKPASS="+credHelper,
		"GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Pull(repoPath string) error {
	return c.PullContext(context.Background(), repoPath)
}

func (c *Client) PullContext(ctx context.Context, repoPath string) error {
	cmd := processtree.CommandContext(ctx, "git", "-C", repoPath, "pull")
	cmd.Env = NonInteractiveEnvironment(cmd.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Checkout(repoPath, branch string) error {
	return c.CheckoutContext(context.Background(), repoPath, branch)
}

func (c *Client) CheckoutContext(ctx context.Context, repoPath, branch string) error {
	cmd := processtree.CommandContext(ctx, "git", "-C", repoPath, "checkout", branch)
	cmd.Env = NonInteractiveEnvironment(cmd.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w, output: %s", err, output)
	}
	return nil
}

// CheckoutBranchAtRevisionContext pins a disposable workspace while retaining its branch name.
func (c *Client) CheckoutBranchAtRevisionContext(ctx context.Context, repoPath, branch, revision string) error {
	branch = strings.TrimPrefix(branch, "refs/heads/")
	if branch == "" || strings.HasPrefix(branch, "-") || revision == "" || strings.HasPrefix(revision, "-") {
		return fmt.Errorf("branch and revision are required")
	}
	cmd := processtree.CommandContext(ctx, "git", "-C", repoPath, "checkout", "-B", branch, revision)
	cmd.Env = NonInteractiveEnvironment(cmd.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout pinned branch failed: %w, output: %s", err, output)
	}
	return nil
}

// CheckoutDetachedAtRevisionContext checks out a tag build's pinned commit.
func (c *Client) CheckoutDetachedAtRevisionContext(ctx context.Context, repoPath, revision string) error {
	if revision == "" || strings.HasPrefix(revision, "-") {
		return fmt.Errorf("revision is required")
	}
	cmd := processtree.CommandContext(ctx, "git", "-C", repoPath, "checkout", "--detach", revision, "--")
	cmd.Env = NonInteractiveEnvironment(cmd.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout pinned tag failed: %w, output: %s", err, output)
	}
	return nil
}

// NonInteractiveEnvironment keeps CI Git commands from waiting forever for a
// terminal/keychain prompt and aborts an HTTP transfer that makes no progress.
// Safety values replace inherited values so server launch environment cannot
// accidentally re-enable an interactive prompt.
func NonInteractiveEnvironment(base []string) []string {
	result := make([]string, 0, len(base)+len(nonInteractiveEnvironmentDefaults))
	for _, entry := range base {
		current, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		safetyKey := false
		for _, fallback := range nonInteractiveEnvironmentDefaults {
			key, _, _ := strings.Cut(fallback, "=")
			if current == key || (runtime.GOOS == "windows" && strings.EqualFold(current, key)) {
				safetyKey = true
				break
			}
		}
		if !safetyKey {
			result = append(result, entry)
		}
	}
	for _, fallback := range nonInteractiveEnvironmentDefaults {
		result = append(result, fallback)
	}
	return result
}

func createSSHCredentialHelper(username, password string) (string, error) {
	tmpFile, err := os.CreateTemp("", "git-cred-*.sh")
	if err != nil {
		return "", err
	}

	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
	Username*) echo "%s" ;;
	Password*) echo "%s" ;;
esac
`, username, password)

	if err := os.WriteFile(tmpFile.Name(), []byte(script), 0700); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}
