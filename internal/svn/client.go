package svn

import (
	"fmt"
	"os/exec"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Checkout(url, dest string) error {
	cmd := exec.Command("svn", "checkout", url, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("svn checkout failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Update(repoPath string) error {
	cmd := exec.Command("svn", "update", repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("svn update failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Commit(repoPath, message string) error {
	cmd := exec.Command("svn", "commit", "-m", message, repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("svn commit failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Info(repoPath string) (string, error) {
	cmd := exec.Command("svn", "info", repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("svn info failed: %w, output: %s", err, output)
	}
	return string(output), nil
}

func (c *Client) Status(repoPath string) (string, error) {
	cmd := exec.Command("svn", "status", repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("svn status failed: %w, output: %s", err, output)
	}
	return string(output), nil
}
