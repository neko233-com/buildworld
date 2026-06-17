package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepo(t *testing.T) (repoDir string, cleanup func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}

	repoDir = tmpDir + "/test-repo"
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "init", "--bare", repoDir).Run(); err != nil {
		t.Fatalf("failed to init bare repo: %v", err)
	}

	cleanup = func() { os.RemoveAll(tmpDir) }
	return
}

func TestGitClone(t *testing.T) {
	repoDir, cleanup := initTestRepo(t)
	defer cleanup()

	client := NewClient()
	cloneDir := filepath.Join(filepath.Dir(repoDir), "clone")

	err := client.Clone("file://"+repoDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	if _, err := os.Stat(cloneDir + "/.git"); os.IsNotExist(err) {
		t.Error("Clone did not create .git directory")
	}
}

func TestGitCloneToExistingDir(t *testing.T) {
	repoDir, cleanup := initTestRepo(t)
	defer cleanup()

	client := NewClient()
	tmpDir := filepath.Dir(repoDir)
	existingDir := tmpDir + "/existing"
	os.MkdirAll(existingDir, 0755)

	file := existingDir + "/placeholder.txt"
	os.WriteFile(file, []byte("data"), 0644)

	err := client.Clone("file://"+repoDir, existingDir)
	if err == nil {
		t.Fatal("Clone() to non-empty directory expected error, got nil")
	}
}

func TestGitPullNonRepo(t *testing.T) {
	client := NewClient()
	tmpDir, err := os.MkdirTemp("", "git-pull-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = client.Pull(tmpDir)
	if err == nil {
		t.Fatal("Pull() on non-repo directory expected error, got nil")
	}
}

func TestGitCheckoutNonExistentBranch(t *testing.T) {
	repoDir, cleanup := initTestRepo(t)
	defer cleanup()

	cloneDir := filepath.Join(filepath.Dir(repoDir), "clone")
	client := NewClient()

	if err := client.Clone("file://"+repoDir, cloneDir); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	err := client.Checkout(cloneDir, "nonexistent-branch-xyz")
	if err == nil {
		t.Fatal("Checkout() nonexistent branch expected error, got nil")
	}
}

func TestClientInitialization(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}
