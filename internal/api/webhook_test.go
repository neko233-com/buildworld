package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/webhook"
)

func TestWebhookPushCreatesMatchedProjectBuild(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	project, err := database.CreateProject(
		"webhook-project",
		"",
		"https://github.com/example/project.git",
		"git",
		"main",
		"jobs:\n  build:\n    steps:\n      - run: echo ok\n",
		0,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	payload := webhook.WebhookPayload{Ref: "refs/heads/release"}
	payload.Repository.CloneURL = project.RepoURL
	payload.HeadCommit.ID = "0123456789abcdef0123456789abcdef01234567"
	payload.HeadCommit.Message = "deploy release"

	handler := &handlers{d: Deps{Store: database}}
	if err := handler.triggerBuildByWebhook(payload); err != nil {
		t.Fatal(err)
	}

	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 {
		t.Fatalf("build count = %d, want 1", len(builds))
	}
	build := builds[0]
	if build.Trigger != "webhook" || build.Branch != "release" || build.CommitSHA != payload.HeadCommit.ID {
		t.Fatalf("build = %+v, want normal webhook metadata", build)
	}
}

func TestWebhookPinsParametersConditionsAndWorkspaceToPushedRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGit := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "--initial-branch=main", repo)
	runGit("-C", repo, "config", "user.email", "test@example.invalid")
	runGit("-C", repo, "config", "user.name", "BuildWorld Test")
	writePipeline := func(defaultValue, marker string) {
		t.Helper()
		source := `pipeline { agent any parameters { booleanParam(name: 'RUN', defaultValue: ` + defaultValue + `) } stages { stage('verify') { when { expression { params.RUN } } steps { sh 'test "$(cat marker)" = ` + marker + `; echo verified-` + marker + `' } } } }`
		if err := os.WriteFile(filepath.Join(repo, "Jenkinsfile"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "marker"), []byte(marker), 0o600); err != nil {
			t.Fatal(err)
		}
		runGit("-C", repo, "add", "Jenkinsfile", "marker")
		runGit("-C", repo, "commit", "-m", marker)
	}
	writePipeline("true", "first")
	first := runGit("-C", repo, "rev-parse", "HEAD")
	writePipeline("false", "second")
	runGit("-C", repo, "checkout", "-b", "feature")
	writePipeline("true", "feature")
	feature := runGit("-C", repo, "rev-parse", "HEAD")
	runGit("-C", repo, "checkout", "main")
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("pinned-webhook", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repo, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	payload := webhook.WebhookPayload{Ref: "refs/heads/main", After: first}
	payload.Repository.CloneURL = repo
	h := &handlers{d: Deps{Store: database}}
	loadedProject, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, selected, defaults, err := h.prepareProjectBuild(context.Background(), loadedProject, "feature", "", nil)
	if err != nil || selected != feature || !strings.Contains(defaults, `"RUN":true`) {
		t.Fatalf("selected branch revision=%q defaults=%q error=%v", selected, defaults, err)
	}
	if err := h.triggerBuildByWebhook(payload); err != nil {
		t.Fatal(err)
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 {
		t.Fatalf("builds=%v error=%v", builds, err)
	}
	build := builds[0]
	if build.CommitSHA != first || !strings.Contains(build.Parameters, `"RUN":true`) {
		t.Fatalf("incorrect revision or parameter defaults: %+v", build)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		build, err = database.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "success" || build.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "success" || build.CommitSHA != first || !strings.Contains(build.Log, "verified-first") || strings.Contains(build.Log, "verified-second") {
		t.Fatalf("wrong executable build: %+v", build)
	}
}

func TestWebhookBuildUsesDeclaredParameterDefaults(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	source := `parameters:
  - {name: ENTRY_ONLY, type: boolean, default: true}
jobs:
  test:
    steps:
      - {name: ok, run: echo ok}
`
	project, err := database.CreateProject("defaults", "", "https://example.invalid/repo", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := webhook.WebhookPayload{Ref: "refs/heads/main"}
	payload.Repository.CloneURL = project.RepoURL
	payload.After = "0123456789abcdef0123456789abcdef01234567"
	h := &handlers{d: Deps{Store: database}}
	if err := h.triggerBuildByWebhook(payload); err != nil {
		t.Fatal(err)
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 {
		t.Fatalf("builds=%v err=%v", builds, err)
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(builds[0].Parameters), &params); err != nil {
		t.Fatal(err)
	}
	if params["ENTRY_ONLY"] != true {
		t.Fatalf("defaults: %#v", params)
	}
}

func TestWebhookNeverBuildsLatestForMissingCommitOrDeletedRef(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "webhook-pinning.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("pinning", "", "https://example.invalid/repo", "git", "main", "jobs:\n  test:\n    steps:\n      - run: echo ok\n", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{d: Deps{Store: database}}
	for _, tc := range []struct {
		name, ref, after string
		wantError        bool
	}{
		{"missing commit", "refs/heads/main", "", true},
		{"short commit", "refs/heads/main", "1234567", true},
		{"branch deleted", "refs/heads/main", "0000000000000000000000000000000000000000", false},
		{"non push event", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := webhook.WebhookPayload{Ref: tc.ref, After: tc.after}
			p.Repository.CloneURL = project.RepoURL
			if err := h.triggerBuildByWebhook(p); (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%t", err, tc.wantError)
			}
		})
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 0 {
		t.Fatalf("unexpected builds=%v error=%v", builds, err)
	}
}

func webhookSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookEndpointsFailClosedAndTriggerOnlyWithCorrectSecret(t *testing.T) {
	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"example/project","clone_url":"https://example.test/project.git"},"head_commit":{"id":"0123456789abcdef0123456789abcdef01234567"}}`)
	providers := []struct {
		name      string
		path      string
		setHeader func(*http.Request, string)
	}{
		{
			name: "github",
			path: "/api/webhooks/github",
			setHeader: func(request *http.Request, secret string) {
				request.Header.Set("X-Hub-Signature-256", "sha256="+webhookSignature(secret, payload))
			},
		},
		{
			name: "gitlab",
			path: "/api/webhooks/gitlab",
			setHeader: func(request *http.Request, secret string) {
				request.Header.Set("X-Gitlab-Token", secret)
			},
		},
		{
			name: "gitea",
			path: "/api/webhooks/gitea",
			setHeader: func(request *http.Request, secret string) {
				request.Header.Set("X-Gitea-Signature", webhookSignature(secret, payload))
			},
		},
	}
	scenarios := []struct {
		name             string
		configuredSecret string
		requestSecret    string
		wantStatus       int
		wantBuilds       int
	}{
		{name: "empty-secret", wantStatus: http.StatusServiceUnavailable},
		{name: "wrong-secret", configuredSecret: "expected-secret", requestSecret: "wrong-secret", wantStatus: http.StatusUnauthorized},
		{name: "correct-secret", configuredSecret: "expected-secret", requestSecret: "expected-secret", wantStatus: http.StatusOK, wantBuilds: 1},
	}

	for _, provider := range providers {
		for _, scenario := range scenarios {
			t.Run(provider.name+"/"+scenario.name, func(t *testing.T) {
				database, err := store.New(filepath.Join(t.TempDir(), "webhook-endpoint.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.Close() })
				project, err := database.CreateProject(
					"webhook-endpoint-project",
					"",
					"https://example.test/project.git",
					"git",
					"main",
					"jobs:\n  build:\n    steps:\n      - run: echo ok\n",
					0,
					nil,
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				router := NewRouter(Deps{
					Cfg:   &config.Config{Automation: config.AutomationConfig{GitHubWebhookSecret: scenario.configuredSecret}},
					Store: database,
				})
				request := httptest.NewRequest(http.MethodPost, provider.path, bytes.NewReader(payload))
				provider.setHeader(request, scenario.requestSecret)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				if recorder.Code != scenario.wantStatus {
					t.Fatalf("status = %d, want %d; body=%s", recorder.Code, scenario.wantStatus, recorder.Body.String())
				}
				builds, err := database.ListBuildsByProject(project.ID)
				if err != nil {
					t.Fatal(err)
				}
				if len(builds) != scenario.wantBuilds {
					t.Fatalf("build count = %d, want %d", len(builds), scenario.wantBuilds)
				}
			})
		}
	}
}
