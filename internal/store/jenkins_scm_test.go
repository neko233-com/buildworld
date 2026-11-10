package store

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestJenkinsSCMCheckoutHistoryIsScopedAndImmutable(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("history", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CreateBuild(project.ID, 1, "manual", "main", "a", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := JenkinsSCMCheckout{BuildID: first.ID, ProjectID: project.ID, BuildNumber: 1, RepositoryHash: "repo-a", Branch: "main", Revision: "a", HistoryKnown: true, ChangesJSON: `[]`}
	if err := db.SaveJenkinsSCMCheckout(record); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveJenkinsSCMCheckout(record); err != nil {
		t.Fatal(err)
	}
	record.Revision = "replacement"
	if err := db.SaveJenkinsSCMCheckout(record); err == nil {
		t.Fatal("conflicting snapshot was accepted")
	}
	got, err := db.JenkinsSCMCheckoutForBuild(first.ID)
	if err != nil || got == nil || got.Revision != "a" {
		t.Fatalf("checkout=%+v err=%v", got, err)
	}
	for _, tc := range []struct {
		project int64
		number  int
		repo    string
		branch  string
		want    string
	}{{project.ID, 2, "repo-a", "main", "a"}, {project.ID, 1, "repo-a", "main", ""}, {project.ID, 2, "repo-b", "main", ""}, {project.ID, 2, "repo-a", "feature", ""}, {project.ID + 1, 2, "repo-a", "main", ""}} {
		got, _, err := db.PreviousJenkinsSCMRevision(tc.project, tc.number, tc.repo, tc.branch)
		if err != nil || got != tc.want {
			t.Fatalf("case=%+v got=%q err=%v", tc, got, err)
		}
	}
	if err := db.DeleteProject(project.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := db.JenkinsSCMCheckoutForBuild(first.ID); err != nil || got != nil {
		t.Fatalf("checkout survived project deletion: %+v err=%v", got, err)
	}
}

func TestJenkinsSCMHistoryDoesNotInventLegacyCheckout(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("legacy", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateBuild(project.ID, 1, "manual", "", "old", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	revision, known, err := db.PreviousJenkinsSCMRevision(project.ID, 2, "repo", "main")
	if err != nil || known || revision != "" {
		t.Fatalf("legacy history revision=%q known=%v err=%v", revision, known, err)
	}
	if revision, known, err = db.PreviousJenkinsSCMRevision(project.ID, 1, "repo", "main"); err != nil || !known || revision != "" {
		t.Fatalf("first build revision=%q known=%v err=%v", revision, known, err)
	}
}

func TestJenkinsSCMCheckoutConcurrentSaveAndPrune(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("prune", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var buildIDs []int64
	for number := 1; number <= 2; number++ {
		build, err := db.CreateBuild(project.ID, number, "manual", "main", "sha", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		buildIDs = append(buildIDs, build.ID)
		record := JenkinsSCMCheckout{BuildID: build.ID, ProjectID: project.ID, BuildNumber: number, RepositoryHash: "repo", Branch: "main", Revision: "sha", HistoryKnown: true, ChangesJSON: `[]`}
		var wg sync.WaitGroup
		errors := make(chan error, 16)
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errors <- db.SaveJenkinsSCMCheckout(record) }()
		}
		wg.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := db.FinishBuild(build.ID, "success", 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.PruneCompletedBuilds(project.ID, 1); err != nil {
		t.Fatal(err)
	}
	if old, err := db.JenkinsSCMCheckoutForBuild(buildIDs[0]); err != nil || old != nil {
		t.Fatalf("pruned checkout=%+v err=%v", old, err)
	}
	if latest, err := db.JenkinsSCMCheckoutForBuild(buildIDs[1]); err != nil || latest == nil {
		t.Fatalf("latest checkout=%+v err=%v", latest, err)
	}
}
