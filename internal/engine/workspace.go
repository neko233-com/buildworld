package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceManager manages per-build workspace directories.
type WorkspaceManager struct {
	root string
}

func NewWorkspaceManager(root string) *WorkspaceManager {
	if root == "" {
		root = "./data/workspaces"
	}
	return &WorkspaceManager{root: root}
}

// Prepare creates and returns a workspace directory for a build.
func (m *WorkspaceManager) Prepare(projectID int64, buildNumber int) (string, error) {
	dir := filepath.Join(m.root, fmt.Sprintf("project-%d-build-%d", projectID, buildNumber))
	if err := m.Clean(dir); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("clean stale workspace: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	return dir, nil
}

// Clean removes a workspace directory.
func (m *WorkspaceManager) Clean(dir string) error {
	root, err := filepath.Abs(m.root)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve workspace path: %w", err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refuse to clean path outside workspace root: %s", target)
	}
	return os.RemoveAll(target)
}

// Root returns the workspace root path.
func (m *WorkspaceManager) Root() string { return m.root }
