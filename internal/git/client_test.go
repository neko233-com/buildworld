package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNonInteractiveEnvironmentAddsAndEnforcesSafetyDefaults(t *testing.T) {
	env := NonInteractiveEnvironment([]string{
		"PATH=/usr/bin",
		"GIT_HTTP_LOW_SPEED_TIME=120",
	})
	for _, expected := range []string{
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
		"GIT_HTTP_LOW_SPEED_LIMIT=1",
		"GIT_HTTP_LOW_SPEED_TIME=60",
	} {
		if !slices.Contains(env, expected) {
			t.Fatalf("Git environment missing %q: %#v", expected, env)
		}
	}
	if slices.Contains(env, "GIT_HTTP_LOW_SPEED_TIME=120") {
		t.Fatalf("Git environment retained unsafe inherited timeout: %#v", env)
	}
}

func TestGitClone(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping test")
	}

	// Create a test repository
	repoDir := tmpDir + "/test-repo"
	os.MkdirAll(repoDir, 0755)

	cmd := exec.Command("git", "init", "--bare", repoDir)
	if err := cmd.Run(); err != nil {
		t.Skip("git init failed, skipping test")
	}

	client := NewClient()
	cloneDir := tmpDir + "/clone"
	err = client.Clone("file:///"+repoDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	if _, err := os.Stat(cloneDir + "/.git"); os.IsNotExist(err) {
		t.Error("Clone did not create .git directory")
	}
}

func TestGitPull(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test repository
	repoDir := tmpDir + "/test-repo"
	os.MkdirAll(repoDir, 0755)

	cmd := exec.Command("git", "init", "--bare", repoDir)
	if err := cmd.Run(); err != nil {
		t.Skip("git init failed, skipping test")
	}

	client := NewClient()
	cloneDir := tmpDir + "/clone"
	err = client.Clone("file:///"+repoDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	// Create a commit so we have something to pull
	cmd = exec.Command("git", "-C", cloneDir, "config", "user.email", "test@test.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", cloneDir, "config", "user.name", "Test")
	cmd.Run()
	cmd = exec.Command("git", "-C", cloneDir, "commit", "--allow-empty", "-m", "init")
	cmd.Run()
	cmd = exec.Command("git", "-C", cloneDir, "push", "origin", "HEAD")
	cmd.Run()

	err = client.Pull(cloneDir)
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
}

func TestGitCheckout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test repository
	repoDir := tmpDir + "/test-repo"
	os.MkdirAll(repoDir, 0755)

	cmd := exec.Command("git", "init", "--bare", repoDir)
	if err := cmd.Run(); err != nil {
		t.Skip("git init failed, skipping test")
	}

	client := NewClient()
	cloneDir := tmpDir + "/clone"
	err = client.Clone("file:///"+repoDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	// Create a commit
	cmd = exec.Command("git", "-C", cloneDir, "config", "user.email", "test@test.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", cloneDir, "config", "user.name", "Test")
	cmd.Run()
	cmd = exec.Command("git", "-C", cloneDir, "commit", "--allow-empty", "-m", "init")
	cmd.Run()

	// Checkout current branch (whatever it is)
	cmd = exec.Command("git", "-C", cloneDir, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Skip("Could not determine current branch")
	}
	currentBranch := string(output[:len(output)-1]) // Remove newline

	err = client.Checkout(cloneDir, currentBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}
}

func TestCreateSSHCredentialHelper(t *testing.T) {
	helper, err := createSSHCredentialHelper("testuser", "testpass")
	if err != nil {
		t.Fatalf("createSSHCredentialHelper() error = %v", err)
	}
	defer os.Remove(helper)

	// Verify the helper script exists
	if _, err := os.Stat(helper); os.IsNotExist(err) {
		t.Error("Credential helper script not created")
	}
}

func TestGitCloneToExistingDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test repository
	repoDir := tmpDir + "/test-repo"
	os.MkdirAll(repoDir, 0755)

	cmd := exec.Command("git", "init", "--bare", repoDir)
	if err := cmd.Run(); err != nil {
		t.Skip("git init failed, skipping test")
	}

	client := NewClient()
	existingDir := tmpDir + "/existing"
	os.MkdirAll(existingDir, 0755)

	file := existingDir + "/placeholder.txt"
	os.WriteFile(file, []byte("data"), 0644)

	err = client.Clone("file:///"+repoDir, existingDir)
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
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test repository
	repoDir := tmpDir + "/test-repo"
	os.MkdirAll(repoDir, 0755)

	cmd := exec.Command("git", "init", "--bare", repoDir)
	if err := cmd.Run(); err != nil {
		t.Skip("git init failed, skipping test")
	}

	cloneDir := filepath.Join(tmpDir, "clone")
	client := NewClient()

	if err := client.Clone("file:///"+repoDir, cloneDir); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	err = client.Checkout(cloneDir, "nonexistent-branch-xyz")
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
