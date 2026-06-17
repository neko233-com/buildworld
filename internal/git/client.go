package git

import (
	"fmt"
	"os/exec"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Clone(url, dest string) error {
	cmd := exec.Command("git", "clone", url, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Pull(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Checkout(repoPath, branch string) error {
	cmd := exec.Command("git", "-C", repoPath, "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w, output: %s", err, output)
	}
	return nil
}
