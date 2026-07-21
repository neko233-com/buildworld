package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRepositoryStoresRejectUnsupportedTypes(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "git-only.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.CreateProject("svn-project", "", "", "svn", "main", `{}`, 0, nil, nil); !errors.Is(err, ErrUnsupportedRepositoryType) {
		t.Fatalf("CreateProject() error = %v, want ErrUnsupportedRepositoryType", err)
	}
	if _, err := database.CreateVCSRoot("hg-root", "hg", "", "main", nil, 0, false, `{}`); !errors.Is(err, ErrUnsupportedRepositoryType) {
		t.Fatalf("CreateVCSRoot() error = %v, want ErrUnsupportedRepositoryType", err)
	}
	if _, err := database.CreateCredential("svn-credential", CredentialType("svn"), "", "", "", "", "", "", "", true); !errors.Is(err, ErrUnsupportedCredentialType) {
		t.Fatalf("CreateCredential() error = %v, want ErrUnsupportedCredentialType", err)
	}
}

func TestStoreStartupNormalizesOnlyEmptyRepositoryTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "normalize-empty.db")
	database, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	validConfig := `{"stages":[{"name":"Build","steps":[]}]}`
	validProject, err := database.CreateProject(
		"valid-git",
		"must remain unchanged",
		"ssh://git@example.test/game.git",
		RepositoryTypeGit,
		"release",
		validConfig,
		0,
		nil,
		nil,
		[]string{"server"},
	)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.db.Exec(
		`INSERT INTO projects (name, description, repo_url, repo_type, default_branch, config, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"empty-project", "", "https://example.test/empty.git", "", "main", `{}`, 0,
	); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.db.Exec(
		`INSERT INTO vcs_roots (name, type, url, branch, config) VALUES (?, ?, ?, ?, ?)`,
		"empty-root", "", "https://example.test/empty.git", "main", `{}`,
	); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	emptyProject, err := database.GetProjectByName("empty-project")
	if err != nil {
		t.Fatal(err)
	}
	if emptyProject.RepoType != RepositoryTypeGit {
		t.Fatalf("empty project repo type = %q, want git", emptyProject.RepoType)
	}
	roots, err := database.ListVCSRoots()
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].Type != RepositoryTypeGit {
		t.Fatalf("normalized roots = %+v, want one Git root", roots)
	}

	after, err := database.GetProject(validProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.RepoType != RepositoryTypeGit ||
		after.RepoURL != validProject.RepoURL ||
		after.DefaultBranch != validProject.DefaultBranch ||
		after.Config != validProject.Config ||
		after.Description != validProject.Description {
		t.Fatalf("valid Git project changed during normalization: before=%+v after=%+v", validProject, after)
	}
}
