package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/webhook"
)

func TestPullRequestWebhookProjectConfiguration(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}
	invalid := httptest.NewRecorder()
	handler.createProject(invalid, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"bad","repo_type":"git","config":"jobs:\n  build:\n    steps:\n      - run: echo ok\n","pull_request_webhook":true}`)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("inline PR setting status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	created := httptest.NewRecorder()
	body := `{"name":"good","repo_type":"git","pipeline_format":"jenkinsfile","pipeline_source_mode":"scm","pipeline_scm_repo":"https://example.invalid/repo.git","pipeline_scm_path":"Jenkinsfile","pull_request_webhook":true}`
	handler.createProject(created, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var project store.Project
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil || !project.PullRequestWebhook {
		t.Fatalf("PR setting response=%s err=%v", created.Body.String(), err)
	}
	// Older clients that omit the new field must not silently disable it.
	update := `{"name":"good","repo_type":"git","pipeline_format":"jenkinsfile","pipeline_source_mode":"scm","pipeline_scm_repo":"https://example.invalid/repo.git","pipeline_scm_path":"Jenkinsfile"}`
	updated := httptest.NewRecorder()
	handler.updateProject(updated, requestWithRouteID(http.MethodPut, "/", strings.NewReader(update), project.ID))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"pull_request_webhook":true`) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	invalidUpdate := httptest.NewRecorder()
	handler.updateProject(invalidUpdate, requestWithRouteID(http.MethodPut, "/", strings.NewReader(`{"name":"good","repo_type":"git","config":"jobs:\n  build:\n    steps:\n      - run: echo ok\n"}`), project.ID))
	if invalidUpdate.Code != http.StatusBadRequest {
		t.Fatalf("invalid source change status=%d body=%s", invalidUpdate.Code, invalidUpdate.Body.String())
	}
}

func TestPullRequestWebhookPinsBuildAndRespectsProjectPolicy(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	origin, repository := filepath.Join(root, "origin.git"), filepath.Join(root, "repository")
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--bare", origin)
	git("init", repository)
	git("-C", repository, "config", "user.email", "test@example.invalid")
	git("-C", repository, "config", "user.name", "Test")
	jenkinsfile := `pipeline { agent any stages { stage('pr') { when { allOf { branch 'PR-42'; changeRequest id: '42', target: 'main', branch: 'feature', author: 'alice', title: 'Improve service' } } steps { sh 'test "$CHANGE_TARGET" = main; test "$CHANGE_BRANCH" = feature; test "$CHANGE_AUTHOR" = alice; test "$(cat marker)" = old; echo TRUSTED_PR_OK' } } } }`
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(jenkinsfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "marker"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("-C", repository, "add", "Jenkinsfile", "marker")
	git("-C", repository, "commit", "-m", "original")
	git("-C", repository, "branch", "-M", "main")
	git("-C", repository, "remote", "add", "origin", origin)
	git("-C", repository, "push", "origin", "main")
	git("--git-dir", origin, "symbolic-ref", "HEAD", "refs/heads/main")
	git("-C", repository, "checkout", "-b", "feature")
	git("-C", repository, "commit", "--allow-empty", "-m", "PR head")
	first := git("-C", repository, "rev-parse", "HEAD")
	git("-C", repository, "push", "origin", "HEAD:refs/pull/42/head")
	if err := os.WriteFile(filepath.Join(repository, "marker"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("-C", repository, "add", "marker")
	git("-C", repository, "commit", "-m", "move PR head")
	git("-C", repository, "push", "origin", "HEAD:refs/pull/42/head")
	originPath := filepath.ToSlash(origin)
	if !strings.HasPrefix(originPath, "/") {
		originPath = "/" + originPath
	}
	originURL := (&url.URL{Scheme: "file", Path: originPath}).String()
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("pr-webhook", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", originURL, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: database}}
	spoof := webhook.WebhookPayload{Kind: "push", Provider: "github", DeliveryID: "spoof", BodyHash: "hash", Ref: "refs/pull/42/head", After: first}
	spoof.Repository.CloneURL = originURL
	if err := handler.receiveWebhook(spoof); err == nil {
		t.Fatal("push event bypassed the PR opt-in")
	}
	provider := webhook.NewWebhookHandler("fixture-secret")
	provider.OnPush(handler.receiveWebhook)
	send := func(delivery string, headRepoID int64, gitea bool) {
		t.Helper()
		action := "opened"
		if gitea {
			action = "synchronized"
		}
		body, err := json.Marshal(map[string]interface{}{
			"action": action, "number": 42,
			"repository": map[string]interface{}{"id": 7, "clone_url": originURL},
			"pull_request": map[string]interface{}{"number": 42, "state": "open", "title": "Improve service", "user": map[string]interface{}{"login": "alice"},
				"head": map[string]interface{}{"ref": "feature", "sha": first, "repo": map[string]interface{}{"id": headRepoID}},
				"base": map[string]interface{}{"ref": "main", "repo": map[string]interface{}{"id": 7}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, []byte("fixture-secret"))
		_, _ = mac.Write(body)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		response := httptest.NewRecorder()
		if gitea {
			req.Header.Set("X-Gitea-Event", "pull_request_sync")
			req.Header.Set("X-Gitea-Delivery", delivery)
			req.Header.Set("X-Gitea-Signature", hex.EncodeToString(mac.Sum(nil)))
			provider.HandleGiteaWebhook(response, req)
		} else {
			req.Header.Set("X-GitHub-Event", "pull_request")
			req.Header.Set("X-GitHub-Delivery", delivery)
			req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			provider.HandleGitHubWebhook(response, req)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("webhook status=%d body=%s", response.Code, response.Body.String())
		}
	}
	send("disabled", 7, false)
	if err := database.SetProjectPullRequestWebhook(project.ID, true); err != nil {
		t.Fatal(err)
	}
	send("fork", 8, false)
	send("same-repo", 7, false)
	send("same-repo", 7, false)
	target, err := database.ClaimWebhookTarget(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.processWebhookTarget(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 || builds[0].CommitSHA != first || builds[0].Branch != "refs/pull/42/head" {
		t.Fatalf("unexpected builds: %+v err=%v", builds, err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(builds[0].ID)
	var build *store.Build
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		build, err = database.GetBuild(builds[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "success" || build.Status == "failed" {
			break
		}
	}
	if build == nil || build.Status != "success" || !strings.Contains(build.Log, "TRUSTED_PR_OK") {
		t.Fatalf("pinned PR did not execute matching conditions: %s", fmt.Sprint(build))
	}
	send("gitea-sync", 7, true)
	target, err = database.ClaimWebhookTarget(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.processWebhookTarget(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	builds, err = database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 2 || builds[0].CommitSHA != first || builds[0].Branch != "refs/pull/42/head" {
		t.Fatalf("Gitea sync did not create a pinned PR build: %+v err=%v", builds, err)
	}
	send("disable-before-dispatch", 7, true)
	if err := database.SetProjectPullRequestWebhook(project.ID, false); err != nil {
		t.Fatal(err)
	}
	target, err = database.ClaimWebhookTarget(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.processWebhookTarget(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	builds, err = database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 2 {
		t.Fatalf("disabled PR target created another build: %+v err=%v", builds, err)
	}
	missingMetadata, err := database.CreateBuild(project.ID, 3, "webhook", "refs/pull/42/head", first, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner.Run(missingMetadata.ID)
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		missingMetadata, err = database.GetBuild(missingMetadata.ID)
		if err != nil {
			t.Fatal(err)
		}
		if missingMetadata.Status == "success" || missingMetadata.Status == "failed" {
			break
		}
	}
	if missingMetadata.Status != "failed" || !strings.Contains(missingMetadata.Log, "authenticated PR webhook metadata is unavailable") || strings.Contains(missingMetadata.Log, "TRUSTED_PR_OK") {
		t.Fatalf("missing metadata did not fail closed: %+v", missingMetadata)
	}
}
