package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestProjectEnabledDefaultsTrueAndRoundTripsThroughProjectReads(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "project-enabled.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	project, err := data.CreateProject("enabled-job", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !project.Enabled {
		t.Fatal("new project enabled = false, want true")
	}
	if err := data.SetProjectEnabled(project.ID, false); err != nil {
		t.Fatal(err)
	}

	byID, err := data.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	byName, err := data.GetProjectByName(project.Name)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := data.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := data.ListProjectSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if byID.Enabled || byName.Enabled || len(projects) != 1 || projects[0].Enabled || len(summaries) != 1 || summaries[0].Enabled {
		t.Fatalf("disabled state did not round trip: byID=%t byName=%t projects=%#v summaries=%#v", byID.Enabled, byName.Enabled, projects, summaries)
	}
	if err := data.SetProjectEnabled(project.ID+1000, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing project error = %v, want sql.ErrNoRows", err)
	}
}

func TestProjectEnabledMigrationPreservesExistingRowsAsEnabled(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "project-enabled-migration.db"))
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name) VALUES (1, 'legacy')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := addProjectEnabled(tx); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := db.QueryRow(`SELECT enabled FROM projects WHERE id=1`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("legacy project enabled = false, want true")
	}
}
