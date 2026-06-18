package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookHandlerGitHub(t *testing.T) {
	handler := NewWebhookHandler("")
	
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
	w := httptest.NewRecorder()
	
	handler.HandleGitHubWebhook(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", w.Code)
	}
	
	if !received {
		t.Error("Push handler was not called")
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
	handler := NewWebhookHandler("")
	
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
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
