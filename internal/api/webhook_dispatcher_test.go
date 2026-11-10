package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/webhook"
)

func TestWebhookDeliveryFanoutReplayAndRestart(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const repo = "https://example.invalid/team/repo.git"
	const source = "jobs:\n  build:\n    steps:\n      - run: echo ok\n"
	first, err := db.CreateProject("first", "", repo, "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CreateProject("second", "", repo, "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{d: Deps{Store: db}}
	payload := webhook.WebhookPayload{Provider: "github", DeliveryID: "delivery-1", BodyHash: "body-hash", Ref: "refs/heads/main", After: "0123456789abcdef0123456789abcdef01234567"}
	payload.Repository.CloneURL = repo
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- h.receiveWebhook(payload) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	changed := payload
	changed.BodyHash = "another-body"
	if err := h.receiveWebhook(changed); !errors.Is(err, store.ErrWebhookDeliveryConflict) {
		t.Fatalf("collision error = %v", err)
	}
	// Simulate a crash after the first target's build was committed, but before
	// its delivery was marked complete or enqueued.
	target, err := db.ClaimWebhookTarget(time.Now().Add(-time.Minute), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	buildID, err := db.CreateWebhookTargetBuild(target, "main", payload.After, "{}", false)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewWebhookDispatcher(Deps{Store: db})
	if err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Stop(context.Background())
	deadline := time.Now().Add(10 * time.Second)
	for {
		statuses, err := db.ListWebhookTargets()
		if err != nil {
			t.Fatal(err)
		}
		if len(statuses) == 2 && statuses[0].State == "done" && statuses[1].State == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("targets not completed: %+v", statuses)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, project := range []*store.Project{first, second} {
		builds, err := db.ListBuildsByProject(project.ID)
		if err != nil || len(builds) != 1 {
			t.Fatalf("project %d builds=%v err=%v", project.ID, builds, err)
		}
		if project.ID == target.ProjectID && builds[0].ID != buildID {
			t.Fatal("recovery created a duplicate build")
		}
	}
	if err := h.receiveWebhook(payload); err != nil {
		t.Fatal(err)
	}
	statuses, err := db.ListWebhookTargets()
	if err != nil || len(statuses) != 2 {
		t.Fatalf("replayed targets=%v err=%v", statuses, err)
	}
}

func TestWebhookDeliveryPreservesApprovalHold(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const source = "approval:\n  strategy: single\njobs:\n  build:\n    steps:\n      - run: echo ok\n"
	project, err := db.CreateProject("approval", "", "https://example.invalid/a.git", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{d: Deps{Store: db, Approval: engine.NewApprovalService(db)}}
	payload := webhook.WebhookPayload{Provider: "gitea", DeliveryID: "delivery-2", BodyHash: "hash", Ref: "refs/heads/main", After: "0123456789abcdef0123456789abcdef01234567"}
	payload.Repository.CloneURL = project.RepoURL
	if err := h.receiveWebhook(payload); err != nil {
		t.Fatal(err)
	}
	target, err := db.ClaimWebhookTarget(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.processWebhookTarget(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	builds, err := db.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 {
		t.Fatalf("builds=%v err=%v", builds, err)
	}
	build := builds[0]
	if !build.ApprovalRequired || build.Status != "pending_approval" {
		t.Fatalf("approval hold missing: %+v", build)
	}
	approval, err := db.GetBuildApprovalByBuild(build.ID)
	if err != nil || approval.Status != "pending" {
		t.Fatal(fmt.Sprintf("approval=%+v err=%v", approval, err))
	}
}
