package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/webhook"
)

// WebhookDispatcher restores durable webhook targets and resolves them without
// keeping the provider's HTTP request open. Builds use the ordinary queue.
type WebhookDispatcher struct {
	h      *handlers
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWebhookDispatcher starts up to 16 independent target resolvers. Leases
// from a crashed controller expire naturally, even if another controller is
// concurrently using the same database.
func NewWebhookDispatcher(d Deps) (*WebhookDispatcher, error) {
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher := &WebhookDispatcher{h: &handlers{d: d}, cancel: cancel}
	for i := 0; i < 16; i++ {
		dispatcher.wg.Add(1)
		go dispatcher.run(ctx)
	}
	return dispatcher, nil
}

// Stop waits for source resolution to observe cancellation before queue shutdown.
func (d *WebhookDispatcher) Stop(ctx context.Context) error {
	d.cancel()
	done := make(chan struct{})
	go func() { d.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func validDeliveryID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.:", c)) {
			return false
		}
	}
	return true
}

func (h *handlers) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	targets, err := h.d.Store.ListWebhookTargets()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

// receiveWebhook persists a snapshot of matching enabled projects before the
// provider receives an acknowledgement. No network clone runs on the HTTP path.
func (h *handlers) receiveWebhook(payload webhook.WebhookPayload) error {
	if !validDeliveryID(payload.DeliveryID) {
		return errors.New("missing or invalid provider delivery ID")
	}
	branch, commit, _ := webhook.ParsePushEvent(payload)
	if len(payload.Ref) > 1024 {
		return errors.New("webhook ref is too long")
	}
	if branch != "" && !webhook.ValidCommitID(commit) {
		return errors.New("webhook push requires a complete immutable commit ID")
	}
	projects, err := h.d.Store.ListProjects()
	if err != nil {
		return err
	}
	ids := make([]int64, 0)
	if branch != "" {
		for _, p := range projects {
			if !p.Enabled {
				continue
			}
			repository := strings.TrimSpace(p.RepoURL)
			if strings.EqualFold(strings.TrimSpace(p.PipelineSourceMode), "scm") {
				repository = strings.TrimSpace(p.PipelineSCMRepo)
			}
			if webhook.MatchesRepository(repository, payload) {
				ids = append(ids, p.ID)
			}
		}
	}
	// No repository URL, commit message or sender is needed after matching;
	// persist only the immutable source identity, never embedded URL credentials.
	stored := webhook.WebhookPayload{Ref: payload.Ref}
	if branch != "" {
		stored.After = commit
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return h.d.Store.RecordWebhookDelivery(payload.Provider, payload.DeliveryID, payload.BodyHash, string(encoded), ids)
}

func (d *WebhookDispatcher) run(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		target, err := d.h.d.Store.ClaimWebhookTarget(time.Now(), 2*time.Minute)
		if err == nil {
			if err := d.h.processWebhookTarget(ctx, target); err != nil {
				// Do not persist git errors: remote URLs can contain credentials.
				log.Printf("Webhook delivery %d project %d failed; retry scheduled", target.DeliveryID, target.ProjectID)
				delay := time.Second << min(target.Attempt-1, 6)
				if finishErr := d.h.d.Store.FinishWebhookTarget(target, "pending", "target processing failed", time.Now().Add(delay)); finishErr != nil {
					log.Printf("Webhook retry state: %v", finishErr)
				}
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Webhook target claim failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *handlers) processWebhookTarget(ctx context.Context, target *store.WebhookTarget) error {
	project, err := h.d.Store.GetProject(target.ProjectID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !project.Enabled && target.BuildID == 0 {
		return h.d.Store.FinishWebhookTarget(target, "skipped", "project unavailable", time.Time{})
	}
	if err != nil {
		return err
	}
	var payload webhook.WebhookPayload
	if err := json.Unmarshal([]byte(target.Payload), &payload); err != nil {
		return err
	}
	branch, commit, _ := webhook.ParsePushEvent(payload)
	config, revision, parameters, err := h.prepareProjectBuild(ctx, project, branch, commit, nil)
	if err != nil {
		return err
	}
	hold, err := h.approvalHold(config)
	if err != nil {
		return err
	}
	buildID := target.BuildID
	if buildID == 0 {
		buildID, err = h.d.Store.CreateWebhookTargetBuild(target, branch, revision, parameters, hold)
		if err != nil {
			return err
		}
	}
	build, err := h.d.Store.GetBuild(buildID)
	if errors.Is(err, sql.ErrNoRows) && target.BuildID != 0 {
		return h.d.Store.FinishWebhookTarget(target, "skipped", "build no longer exists", time.Time{})
	}
	if err != nil {
		return err
	}
	if build.ApprovalRequired {
		if !hold {
			return errors.New("approval policy changed after build creation")
		}
		if h.d.Approval == nil {
			return errors.New("approval service unavailable")
		}
		_, err := h.d.Store.GetBuildApprovalByBuild(buildID)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err = h.d.Approval.RequestIfRequired(buildID, 0, config); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	if h.d.Runner != nil {
		if err := h.d.Runner.Enqueue(buildID); err != nil {
			return fmt.Errorf("enqueue build: %w", err)
		}
	}
	return h.d.Store.FinishWebhookTarget(target, "done", "", time.Time{})
}
