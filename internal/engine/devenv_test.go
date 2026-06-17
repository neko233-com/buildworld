package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDevEnvManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devenv-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	
	m := NewDevEnvManager(tmpDir)
	
	// Test Get
	jdk := m.Get("jdk")
	if jdk == nil {
		t.Fatal("Get(jdk) returned nil")
	}
	if jdk.Name != "JDK" {
		t.Errorf("Name = %s, want JDK", jdk.Name)
	}
	if jdk.Version != "21" {
		t.Errorf("Version = %s, want 21", jdk.Version)
	}
	
	// Test GetAll
	all := m.GetAll()
	if len(all) != 5 {
		t.Errorf("GetAll() length = %d, want 5", len(all))
	}
	
	// Test non-existent
	if m.Get("nonexistent") != nil {
		t.Error("Get(nonexistent) should return nil")
	}
}

func TestDevEnvManagerIsInstalled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devenv-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	
	m := NewDevEnvManager(tmpDir)
	
	// Test with system binary (git should be available)
	if !m.IsInstalled("git") {
		t.Skip("git not available, skipping test")
	}
	
	// Test with non-existent
	if m.IsInstalled("nonexistent") {
		t.Error("IsInstalled(nonexistent) should return false")
	}
}

func TestDevEnvManagerSetup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devenv-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	
	m := NewDevEnvManager(tmpDir)
	
	// Test setup with auto-detect
	err = m.Setup("jdk")
	if err != nil {
		t.Fatalf("Setup(jdk) error = %v", err)
	}
	
	jdk := m.Get("jdk")
	if jdk == nil {
		t.Fatal("Get(jdk) returned nil after setup")
	}
	
	// Test non-existent environment
	err = m.Setup("nonexistent")
	if err == nil {
		t.Error("Setup(nonexistent) should return error")
	}
}

func TestDevEnvManagerGetEnvVars(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devenv-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	
	m := NewDevEnvManager(tmpDir)
	
	vars := m.GetEnvVars("jdk")
	if vars == nil {
		t.Fatal("GetEnvVars(jdk) returned nil")
	}
	
	if _, ok := vars["JAVA_HOME"]; !ok {
		t.Error("GetEnvVars(jdk) should include JAVA_HOME")
	}
	
	// Test non-existent
	vars = m.GetEnvVars("nonexistent")
	if vars != nil {
		t.Error("GetEnvVars(nonexistent) should return nil")
	}
}

func TestDevEnvManagerBinaryPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devenv-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	
	m := NewDevEnvManager(tmpDir)
	
	jdk := m.Get("jdk")
	if jdk == nil {
		t.Fatal("Get(jdk) returned nil")
	}
	
	// Test that binary path is constructed correctly
	installBase := filepath.Join(tmpDir, "env")
	expectedPath := filepath.Join(installBase, "jdk-21", "bin", "JDK")
	if runtime.GOOS == "windows" {
		expectedPath += ".exe"
	}
	
	actualPath := m.getBinaryPath(jdk)
	if actualPath != expectedPath {
		t.Errorf("getBinaryPath() = %s, want %s", actualPath, expectedPath)
	}
}
