package engine

import (
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestTriggerCheckerSkipsDisabledProject(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "disabled-trigger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("disabled-auto", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectEnabled(project.ID, false); err != nil {
		t.Fatal(err)
	}
	project, err = data.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}

	NewTriggerChecker(data, nil, nil).triggerProjectBuild(project, "schedule", "main", "", nil)
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 0 {
		t.Fatalf("disabled automatic trigger created %d builds", len(builds))
	}
}
