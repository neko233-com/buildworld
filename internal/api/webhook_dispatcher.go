package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
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
	var change *webhook.PullRequestChange
	if payload.Kind == "pull_request" {
		pr := payload.PullRequest
		if pr == nil || payload.Number < 1 || (pr.Number != 0 && pr.Number != payload.Number) || !strings.EqualFold(pr.State, "open") {
			return errors.New("invalid pull request event")
		}
		// An enabled project can execute its Jenkinsfile with configured
		// credentials. Do not automatically execute code from another repository.
		if payload.Repository.ID < 1 || pr.Head.Repo == nil || pr.Base.Repo == nil || pr.Head.Repo.ID != payload.Repository.ID || pr.Base.Repo.ID != payload.Repository.ID {
			return nil
		}
		id := strconv.FormatInt(payload.Number, 10)
		branch = "refs/pull/" + id + "/head"
		commit = pr.Head.SHA
		change = &webhook.PullRequestChange{ID: id, Target: pr.Base.Ref, Branch: pr.Head.Ref, Title: pr.Title, Author: pr.User.Login}
	} else if payload.Kind == "merge_request" {
		mr := payload.GitLabMergeRequest
		if mr == nil || mr.IID < 1 || !strings.EqualFold(mr.State, "opened") {
			return errors.New("invalid merge request event")
		}
		if payload.Project.ID < 1 || mr.SourceProjectID != payload.Project.ID || mr.TargetProjectID != payload.Project.ID {
			return nil
		}
		if strings.EqualFold(strings.TrimSpace(mr.Action), "update") && (!webhook.ValidCommitID(mr.OldRevision) || strings.EqualFold(mr.OldRevision, mr.LastCommit.ID)) {
			return errors.New("merge request update requires a distinct previous commit")
		}
		id := strconv.FormatInt(mr.IID, 10)
		branch = "refs/merge-requests/" + id + "/head"
		commit = mr.LastCommit.ID
		change = &webhook.PullRequestChange{ID: id, Target: mr.TargetBranch, Branch: mr.SourceBranch, Title: mr.Title}
		// GitLab's user is the event actor, not necessarily the MR author.
		if mr.AuthorID > 0 && payload.User.ID == mr.AuthorID {
			change.Author, change.AuthorDisplayName = payload.User.Username, payload.User.Name
		}
	}
	if change != nil {
		for _, value := range []string{change.Target, change.Branch, change.Title, change.Author, change.AuthorDisplayName} {
			if len(value) > 2048 || strings.ContainsRune(value, '\x00') {
				return errors.New("invalid pull request metadata")
			}
		}
	}
	if change == nil && (strings.HasPrefix(strings.TrimSpace(payload.Ref), "refs/pull/") || strings.HasPrefix(strings.TrimSpace(payload.Ref), "refs/merge-requests/")) {
		return errors.New("PR refs require an authenticated PR event and project opt-in")
	}
	if len(payload.Ref) > 1024 {
		return errors.New("webhook ref is too long")
	}
	if branch != "" && !webhook.ValidCommitID(commit) {
		return errors.New("webhook build requires a complete immutable commit ID")
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
			if change != nil && (!p.PullRequestWebhook || !strings.EqualFold(strings.TrimSpace(p.PipelineSourceMode), "scm") || !strings.EqualFold(strings.TrimSpace(p.PipelineFormat), "jenkinsfile")) {
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
	// Never persist repository URLs or credentials in a webhook delivery. PR
	// builds additionally retain only the bounded fields needed by conditions.
	stored := webhook.WebhookPayload{Ref: payload.Ref}
	if change != nil {
		stored.Kind, stored.Ref, stored.Change = "pull_request", branch, change
	}
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
	if payload.Kind == "pull_request" && !project.PullRequestWebhook && target.BuildID == 0 {
		return h.d.Store.FinishWebhookTarget(target, "skipped", "PR webhook disabled", time.Time{})
	}
	branch, commit, _ := webhook.ParsePushEvent(payload)
	config, revision, parameters, err := h.prepareProjectBuild(ctx, project, branch, commit, nil)
	if err != nil {
		return err
	}
	if payload.Kind == "pull_request" {
		if payload.Change == nil || (branch != "refs/pull/"+payload.Change.ID+"/head" && branch != "refs/merge-requests/"+payload.Change.ID+"/head") {
			return errors.New("missing PR metadata")
		}
		parameters, err = engine.AttachJenkinsChangeRequestContext(parameters, engine.JenkinsChangeRequestContext{
			ID: payload.Change.ID, Target: payload.Change.Target, Branch: payload.Change.Branch, Title: payload.Change.Title, Author: payload.Change.Author, AuthorDisplayName: payload.Change.AuthorDisplayName,
		})
		if err != nil {
			return err
		}
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
