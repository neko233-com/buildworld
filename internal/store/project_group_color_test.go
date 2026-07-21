package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestProjectGroupColorDefaultsValidatesAndPersists(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	plain, err := data.CreateProjectGroup("plain", "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Color != ProjectGroupColorNeutral {
		t.Fatalf("default color = %q, want %q", plain.Color, ProjectGroupColorNeutral)
	}

	for _, color := range []string{"neutral", "blue", "cyan", "mint", "green", "yellow", "orange", "pink", "purple"} {
		if !IsValidProjectGroupColor(color) {
			t.Fatalf("palette color %q rejected", color)
		}
	}
	for _, color := range []string{"", "BLUE", " blue", "red", "#007aff"} {
		if IsValidProjectGroupColor(color) {
			t.Fatalf("unsupported color %q accepted", color)
		}
	}

	colored, err := data.CreateProjectGroupWithColor("game servers", "", "blue")
	if err != nil {
		t.Fatal(err)
	}
	if colored.Color != "blue" {
		t.Fatalf("created color = %q, want blue", colored.Color)
	}
	if _, err := data.CreateProjectGroupWithColor("invalid", "", "red"); !errors.Is(err, ErrInvalidProjectGroupColor) {
		t.Fatalf("invalid create error = %v, want ErrInvalidProjectGroupColor", err)
	}

	if err := data.UpdateProjectGroupWithColor(colored.ID, colored.Name, colored.Description, "purple"); err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateProjectGroup(colored.ID, colored.Name, "preserve color"); err != nil {
		t.Fatal(err)
	}
	updated, err := data.GetProjectGroup(colored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Color != "purple" {
		t.Fatalf("updated color = %q, want purple", updated.Color)
	}
	if err := data.UpdateProjectGroupWithColor(colored.ID, colored.Name, colored.Description, "red"); !errors.Is(err, ErrInvalidProjectGroupColor) {
		t.Fatalf("invalid update error = %v, want ErrInvalidProjectGroupColor", err)
	}

	groups, err := data.ListProjectGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Color == "" || groups[1].Color == "" {
		t.Fatalf("listed groups = %#v", groups)
	}
}

func TestProjectGroupSchemaMigrationDropsDevelopmentParentColumnAndPreservesLinks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-group-schema.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := []string{
		`CREATE TABLE project_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			parent_id INTEGER REFERENCES project_groups(id),
			color TEXT NOT NULL DEFAULT 'neutral',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			repo_url TEXT NOT NULL,
			repo_type TEXT NOT NULL,
			default_branch TEXT DEFAULT 'main',
			config TEXT NOT NULL,
			group_id INTEGER REFERENCES project_groups(id),
			created_by INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO project_groups (id, name, color) VALUES (10, 'development root', 'blue')`,
		`INSERT INTO project_groups (id, name, parent_id, color) VALUES (20, 'game servers', 10, 'mint')`,
		`INSERT INTO projects (id, name, description, repo_url, repo_type, config, group_id, created_by) VALUES (30, 'linked project', '', '', 'git', '{}', 20, 0)`,
	}
	for _, statement := range legacySchema {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if projectGroupColumnExists(t, migrated, "parent_id") {
		migrated.Close()
		t.Fatal("parent_id column still exists after schema migration")
	}
	migratedGroup, err := migrated.GetProjectGroupByName("game servers")
	if err != nil {
		migrated.Close()
		t.Fatal(err)
	}
	if migratedGroup.ID != 20 || migratedGroup.Color != "mint" {
		migrated.Close()
		t.Fatalf("migrated group = %#v, want ID 20 and mint", migratedGroup)
	}
	migratedProject, err := migrated.GetProject(30)
	if err != nil {
		migrated.Close()
		t.Fatal(err)
	}
	if migratedProject.GroupID == nil || *migratedProject.GroupID != 20 {
		migrated.Close()
		t.Fatalf("migrated project group = %#v, want 20", migratedProject.GroupID)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dbPath)
	if err != nil {
		t.Fatalf("idempotent reopen failed: %v", err)
	}
	defer reopened.Close()
	if projectGroupColumnExists(t, reopened, "parent_id") {
		t.Fatal("parent_id column reappeared after idempotent reopen")
	}
}

func projectGroupColumnExists(t *testing.T, data *Store, column string) bool {
	t.Helper()
	rows, err := data.db.Query("PRAGMA table_info(project_groups)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
