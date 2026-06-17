package git

import (
	"fmt"
	"os"
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

func (c *Client) CloneWithSSH(url, dest, username, password string) error {
	// Create credential helper for password authentication
	credHelper, err := createSSHCredentialHelper(username, password)
	if err != nil {
		return fmt.Errorf("create credential helper: %w", err)
	}
	defer os.Remove(credHelper)

	cmd := exec.Command("git", "clone", url, dest)
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

func CreateSSHKeyPair(keyPath string) error {
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh-keygen failed: %w, output: %s", err, output)
	}
	return nil
}

func GetPublicKey(keyPath string) (string, error) {
	pubKeyPath := keyPath + ".pub"
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	return string(data), nil
}

func GetFingerprint(pubKeyPath string) (string, error) {
	cmd := exec.Command("ssh-keygen", "-lf", pubKeyPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("get fingerprint failed: %w, output: %s", err, output)
	}
	return string(output), nil
}
