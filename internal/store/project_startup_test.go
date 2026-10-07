package store

import (
	"path/filepath"
	"testing"
)

func TestStartupBuildPersistenceAndActiveDeduplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.db")
	data, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject("startup", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if project.BuildOnStartup {
		t.Fatal("startup enabled by default")
	}
	if build, err := data.CreateStartupBuild(project.ID, "main", "", false); err != nil || build != nil {
		t.Fatalf("default build=%v err=%v", build, err)
	}
	if err := data.SetProjectBuildOnStartup(project.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	byID, _ := data.GetProject(project.ID)
	byName, _ := data.GetProjectByName(project.Name)
	listed, _ := data.ListProjects()
	if !byID.BuildOnStartup || !byName.BuildOnStartup || !listed[0].BuildOnStartup {
		t.Fatal("startup flag not persisted")
	}
	build, err := data.CreateStartupBuild(project.ID, "main", `{"env":"dev"}`, false)
	if err != nil || build == nil {
		t.Fatalf("build=%v err=%v", build, err)
	}
	if build.Trigger != "startup" || build.Number != 1 || build.Parameters != `{"env":"dev"}` {
		t.Fatalf("build=%+v", build)
	}
	for _, status := range []string{"pending", "running", "pending_approval"} {
		if _, err := data.db.Exec(`UPDATE builds SET status=? WHERE id=?`, status, build.ID); err != nil {
			t.Fatal(err)
		}
		if extra, err := data.CreateStartupBuild(project.ID, "main", "", false); err != nil || extra != nil {
			t.Fatalf("%s duplicate=%v err=%v", status, extra, err)
		}
	}
	if err := data.FinishBuild(build.ID, "cancelled", 0); err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectEnabled(project.ID, false); err != nil {
		t.Fatal(err)
	}
	if extra, err := data.CreateStartupBuild(project.ID, "main", "", false); err != nil || extra != nil {
		t.Fatalf("disabled build=%v err=%v", extra, err)
	}
	if err := data.SetProjectEnabled(project.ID, true); err != nil {
		t.Fatal(err)
	}
	if next, err := data.CreateStartupBuild(project.ID, "main", "", false); err != nil || next == nil || next.Number != 2 {
		t.Fatalf("next=%v err=%v", next, err)
	}
}

func TestStartupMigrationKeepsLegacyProjectsOptedOut(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "legacy.db"))
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO projects VALUES (1, 'legacy')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := addProjectBuildOnStartup(tx); err != nil {
		t.Fatal(err)
	}
	if err := addProjectBuildOnStartup(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := db.QueryRow(`SELECT build_on_startup FROM projects WHERE id=1`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("legacy startup enabled")
	}
}
