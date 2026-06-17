package svn

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSVNClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestSVNCheckout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "svn-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Check if svn is available
	if _, err := exec.LookPath("svn"); err != nil {
		t.Skip("svn not available, skipping test")
	}

	// Create a test repository
	repoDir := filepath.Join(tmpDir, "repo")
	os.MkdirAll(repoDir, 0755)

	cmd := exec.Command("svnadmin", "create", repoDir)
	if err := cmd.Run(); err != nil {
		t.Skip("svnadmin not available, skipping test")
	}

	client := NewClient()
	checkoutDir := filepath.Join(tmpDir, "checkout")
	err = client.Checkout("file:///"+repoDir, checkoutDir)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	if _, err := os.Stat(checkoutDir); os.IsNotExist(err) {
		t.Error("Checkout did not create directory")
	}
}

func TestSVNUpdate(t *testing.T) {
	client := NewClient()
	err := client.Update("/nonexistent")
	if err == nil {
		t.Error("Update() should return error for nonexistent path")
	}
}

func TestSVNInfo(t *testing.T) {
	client := NewClient()
	_, err := client.Info("/nonexistent")
	if err == nil {
		t.Error("Info() should return error for nonexistent path")
	}
}

func TestSVNStatus(t *testing.T) {
	client := NewClient()
	_, err := client.Status("/nonexistent")
	if err == nil {
		t.Error("Status() should return error for nonexistent path")
	}
}
