package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestIdempotentBuildConcurrentAcrossDatabaseConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "builds.db")
	first, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	user, err := first.CreateUser("builder", "builder@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := first.CreateProject("demo", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(chan int64, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		database := first
		if i%2 != 0 {
			database = second
		}
		go func(database *Store) {
			defer wg.Done()
			build, _, err := database.CreateIdempotentBuild(project.ID, user.ID, "request-key-hash", "request-fingerprint", "main", "", "{}", false, nil)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- build.ID
		}(database)
	}
	wg.Wait()
	close(ids)
	var original int64
	count := 0
	for id := range ids {
		if original == 0 {
			original = id
		}
		if id != original {
			t.Fatalf("duplicate build ids: %d and %d", original, id)
		}
		count++
	}
	if count != 20 {
		t.Fatalf("received %d receipts, want 20", count)
	}
	builds, err := first.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 {
		t.Fatalf("build count=%d err=%v", len(builds), err)
	}
	if _, err := first.FindIdempotentBuild("request-key-hash", "changed", user.ID); !errors.Is(err, ErrBuildIdempotencyConflict) {
		t.Fatalf("changed parameters accepted: %v", err)
	}
	if _, err := first.FindIdempotentBuild("request-key-hash", "request-fingerprint", user.ID+1); !errors.Is(err, ErrBuildIdempotencyConflict) {
		t.Fatalf("other actor accepted: %v", err)
	}
}

func TestIdempotentBuildDoesNotLeaveReceiptWhenBuildInsertFails(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "builds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, err := database.CreateUser("builder", "builder@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("demo", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB().Exec(`CREATE TRIGGER fail_build BEFORE INSERT ON builds BEGIN SELECT RAISE(FAIL, 'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.CreateIdempotentBuild(project.ID, user.ID, "stable-hash", "fingerprint", "main", "", "{}", false, nil); err == nil {
		t.Fatal("build insertion unexpectedly succeeded")
	}
	if build, err := database.FindIdempotentBuild("stable-hash", "fingerprint", user.ID); err != nil || build != nil {
		t.Fatalf("failed transaction left a receipt: build=%v err=%v", build, err)
	}
	if _, err := database.DB().Exec("DROP TRIGGER fail_build"); err != nil {
		t.Fatal(err)
	}
	if build, created, err := database.CreateIdempotentBuild(project.ID, user.ID, "stable-hash", "fingerprint", "main", "", "{}", false, nil); err != nil || !created || build == nil {
		t.Fatalf("retry after rolled back transaction failed: build=%v created=%v err=%v", build, created, err)
	}
}

func TestIdempotentBuildRollsBackWhenNotificationAuditFails(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "builds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, err := database.CreateUser("builder", "builder@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("demo", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.DB().Exec(`CREATE TRIGGER fail_policy BEFORE INSERT ON audit_logs WHEN NEW.action = 'apply_notification_policy' BEGIN SELECT RAISE(FAIL, 'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, _, err = database.CreateIdempotentBuild(project.ID, user.ID, "policy-key", "fingerprint", "main", "", "{}", false, &IdempotentBuildAudit{Username: "builder", Detail: "in_app_only: testing"})
	if err == nil {
		t.Fatal("audit failure unexpectedly created a build")
	}
	if build, err := database.FindIdempotentBuild("policy-key", "fingerprint", user.ID); err != nil || build != nil {
		t.Fatalf("receipt survived rollback: %v %v", build, err)
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 0 {
		t.Fatalf("build survived rollback: %d %v", len(builds), err)
	}
}
