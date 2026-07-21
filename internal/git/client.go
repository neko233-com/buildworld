package git

import (
	"context"
	"fmt"
	"os"

	"github.com/neko233-com/buildworld/internal/processtree"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Clone(url, dest string) error {
	return c.CloneContext(context.Background(), url, dest)
}

func (c *Client) CloneContext(ctx context.Context, url, dest string) error {
	cmd := processtree.CommandContext(ctx, "git", "clone", url, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w, output: %s", err, output)
	}
	return nil
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
	cmd.Env = append(os.Environ(),
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w, output: %s", err, output)
	}
	return nil
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
