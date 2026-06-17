package git

import (
	"os"
	"os/exec"
	"testing"
)

func TestGitClone(t *testing.T) {
	client := NewClient()

	// Create a test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := tmpDir + "/test-repo"
	cloneDir := tmpDir + "/clone"

	// Init bare repo
	os.MkdirAll(repoDir, 0755)
	if err := exec.Command("git", "init", "--bare", repoDir).Run(); err != nil {
		t.Fatalf("failed to init bare repo: %v", err)
	}

	// Clone
	err = client.Clone("file://"+repoDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	// Verify clone exists
	_, err = os.Stat(cloneDir + "/.git")
	if os.IsNotExist(err) {
		t.Error("Clone did not create .git directory")
	}
}
