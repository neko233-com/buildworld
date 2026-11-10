package store

import (
	"path/filepath"
	"testing"
)

func TestProjectPullRequestWebhookOptInPersistsAndIsScoped(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("pr", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateProject("other", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if project.PullRequestWebhook {
		t.Fatal("PR webhook must default to disabled")
	}
	if err := db.SetProjectPullRequestWebhook(project.ID, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.GetProjectByName(project.Name)
	if err != nil || !loaded.PullRequestWebhook {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	projects, err := db.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range projects {
		if item.PullRequestWebhook != (item.ID == project.ID) {
			t.Fatalf("incorrect project setting: %+v", item)
		}
	}
	if err := db.SetProjectPullRequestWebhook(project.ID, false); err != nil {
		t.Fatal(err)
	}
	loaded, err = db.GetProject(project.ID)
	if err != nil || loaded.PullRequestWebhook {
		t.Fatalf("disabled=%+v err=%v", loaded, err)
	}
	if err := db.SetProjectPullRequestWebhook(other.ID+1000, true); err == nil {
		t.Fatal("nonexistent project accepted")
	}
}
