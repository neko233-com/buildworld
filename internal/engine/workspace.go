package engine

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	return dir, nil
}

// Clean removes a workspace directory.
func (m *WorkspaceManager) Clean(dir string) error {
	return os.RemoveAll(dir)
}

// Root returns the workspace root path.
func (m *WorkspaceManager) Root() string { return m.root }
