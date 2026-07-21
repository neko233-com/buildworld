package webhook

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookHandlerGitHub(t *testing.T) {
	handler := NewWebhookHandler("github-secret")

	received := false
	handler.OnPush(func(payload WebhookPayload) error {
		received = true
		if payload.Repository.FullName != "user/repo" {
			t.Errorf("Repository = %s, want user/repo", payload.Repository.FullName)
		}
		return nil
	})

	body := `{
		"ref": "refs/heads/main",
		"repository": {
			"full_name": "user/repo",
			"clone_url": "https://github.com/user/repo.git"
		},
		"head_commit": {
			"id": "abc123def456",
			"message": "Initial commit"
		},
		"sender": {
			"login": "testuser"
		}
	}`

	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(handler.computeHMAC([]byte(body))))
	w := httptest.NewRecorder()

	handler.HandleGitHubWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", w.Code)
	}

	if !received {
		t.Error("Push handler was not called")
	}
}

func TestWebhookHandlerGitLabSignedPush(t *testing.T) {
	handler := NewWebhookHandler("gitlab-secret")
	received := false
	handler.OnPush(func(payload WebhookPayload) error {
		received = payload.Repository.FullName == "owner/repo"
		return nil
	})
	body := `{"ref":"refs/heads/main","repository":{"full_name":"owner/repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")
	w := httptest.NewRecorder()

	handler.HandleGitLabWebhook(w, req)

	if w.Code != http.StatusOK || !received {
		t.Fatalf("signed GitLab push = status %d received %t, want 200/true", w.Code, received)
	}
}

func TestWebhookProvidersRequireConfiguredMatchingSecret(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"owner/repo"}}`)
	providers := []struct {
		name      string
		handle    func(*WebhookHandler, http.ResponseWriter, *http.Request)
		setSecret func(*http.Request, string)
	}{
		{
			name:   "github",
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleGitHubWebhook(w, r) },
			setSecret: func(r *http.Request, secret string) {
				signature := NewWebhookHandler(secret).computeHMAC(body)
				r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(signature))
			},
		},
		{
			name:   "gitlab",
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleGitLabWebhook(w, r) },
			setSecret: func(r *http.Request, secret string) {
				r.Header.Set("X-Gitlab-Token", secret)
			},
		},
		{
			name:   "gitea",
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleGiteaWebhook(w, r) },
			setSecret: func(r *http.Request, secret string) {
				signature := NewWebhookHandler(secret).computeHMAC(body)
				r.Header.Set("X-Gitea-Signature", hex.EncodeToString(signature))
			},
		},
	}

	for _, provider := range providers {
		t.Run(provider.name+"/empty-secret", func(t *testing.T) {
			handler := NewWebhookHandler("")
			called := false
			handler.OnPush(func(WebhookPayload) error { called = true; return nil })
			req := httptest.NewRequest(http.MethodPost, "/webhook/"+provider.name, bytes.NewReader(body))
			provider.setSecret(req, "")
			recorder := httptest.NewRecorder()
			provider.handle(handler, recorder, req)
			if recorder.Code != http.StatusServiceUnavailable || called {
				t.Fatalf("status/callback = %d/%t, want 503/false", recorder.Code, called)
			}
		})

		t.Run(provider.name+"/wrong-secret", func(t *testing.T) {
			handler := NewWebhookHandler("expected-secret")
			called := false
			handler.OnPush(func(WebhookPayload) error { called = true; return nil })
			req := httptest.NewRequest(http.MethodPost, "/webhook/"+provider.name, bytes.NewReader(body))
			provider.setSecret(req, "wrong-secret")
			recorder := httptest.NewRecorder()
			provider.handle(handler, recorder, req)
			if recorder.Code != http.StatusUnauthorized || called {
				t.Fatalf("status/callback = %d/%t, want 401/false", recorder.Code, called)
			}
		})

		t.Run(provider.name+"/correct-secret", func(t *testing.T) {
			handler := NewWebhookHandler("expected-secret")
			called := false
			handler.OnPush(func(WebhookPayload) error { called = true; return nil })
			req := httptest.NewRequest(http.MethodPost, "/webhook/"+provider.name, bytes.NewReader(body))
			provider.setSecret(req, "expected-secret")
			recorder := httptest.NewRecorder()
			provider.handle(handler, recorder, req)
			if recorder.Code != http.StatusOK || !called {
				t.Fatalf("status/callback = %d/%t, want 200/true", recorder.Code, called)
			}
		})
	}
}

func TestWebhookHandlerMethodNotAllowed(t *testing.T) {
	handler := NewWebhookHandler("")

	req := httptest.NewRequest("GET", "/webhook/github", nil)
	w := httptest.NewRecorder()

	handler.HandleGitHubWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want 405", w.Code)
	}
}

func TestWebhookHandlerInvalidPayload(t *testing.T) {
	handler := NewWebhookHandler("invalid-payload-secret")

	body := []byte("invalid")
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(handler.computeHMAC(body)))
	w := httptest.NewRecorder()

	handler.HandleGitHubWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", w.Code)
	}
}

func TestParsePushEvent(t *testing.T) {
	payload := WebhookPayload{
		Ref: "refs/heads/main",
		Repository: struct {
			FullName string `json:"full_name"`
			CloneURL string `json:"clone_url"`
		}{
			FullName: "user/repo",
			CloneURL: "https://github.com/user/repo.git",
		},
		HeadCommit: struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		}{
			ID:      "abc123def456789",
			Message: "Initial commit",
		},
	}

	branch, commit, repo := ParsePushEvent(payload)

	if branch != "main" {
		t.Errorf("branch = %s, want main", branch)
	}

	if commit != "abc123d" {
		t.Errorf("commit = %s, want abc123d", commit)
	}

	if repo != "user/repo" {
		t.Errorf("repo = %s, want user/repo", repo)
	}
}
