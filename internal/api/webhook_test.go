package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/config"
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
	payload.HeadCommit.ID = "0123456789abcdef"
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
	if build.Trigger != "webhook" || build.Branch != "release" || build.CommitSHA != "0123456" {
		t.Fatalf("build = %+v, want normal webhook metadata", build)
	}
}

func webhookSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookEndpointsFailClosedAndTriggerOnlyWithCorrectSecret(t *testing.T) {
	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"example/project","clone_url":"https://example.test/project.git"},"head_commit":{"id":"0123456789abcdef"}}`)
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
