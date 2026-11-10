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

func TestPullRequestWebhookEventsAreAuthenticatedAndFiltered(t *testing.T) {
	body := []byte(`{"action":"opened","number":42,"repository":{"id":7,"full_name":"team/repo"},"pull_request":{"number":42,"state":"open","head":{"sha":"0123456789abcdef0123456789abcdef01234567","repo":{"id":7}},"base":{"repo":{"id":7}}}}`)
	for _, tc := range []struct {
		name, provider, event, action string
		wantReceived                  bool
	}{
		{"github open", "github", "pull_request", "opened", true},
		{"github update", "github", "pull_request", "synchronize", true},
		{"github closed", "github", "pull_request", "closed", false},
		{"gitea update", "gitea", "pull_request_sync", "synchronized", true},
		{"gitea review", "gitea", "pull_request_review", "opened", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewWebhookHandler("secret")
			got := false
			handler.OnPush(func(payload WebhookPayload) error {
				got = payload.Kind == "pull_request" && payload.Number == 42 && payload.PullRequest != nil
				return nil
			})
			data := bytes.Replace(body, []byte(`"action":"opened"`), []byte(`"action":"`+tc.action+`"`), 1)
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
			if tc.provider == "github" {
				req.Header.Set("X-GitHub-Event", tc.event)
				req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(handler.computeHMAC(data)))
			} else {
				req.Header.Set("X-Gitea-Event", tc.event)
				req.Header.Set("X-Gitea-Signature", hex.EncodeToString(handler.computeHMAC(data)))
			}
			response := httptest.NewRecorder()
			if tc.provider == "github" {
				handler.HandleGitHubWebhook(response, req)
			} else {
				handler.HandleGiteaWebhook(response, req)
			}
			if response.Code != http.StatusOK || got != tc.wantReceived {
				t.Fatalf("status=%d received=%v", response.Code, got)
			}
		})
	}
	handler := NewWebhookHandler("secret")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	response := httptest.NewRecorder()
	handler.HandleGitHubWebhook(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid signature status=%d", response.Code)
	}
}

func TestGitLabMergeRequestWebhookFiltersNonCodeUpdates(t *testing.T) {
	for _, tc := range []struct {
		action, oldrev string
		want           bool
	}{
		{"open", "", true}, {"reopen", "", true},
		{"update", "0123456789abcdef0123456789abcdef01234567", true},
		{"update", "", false}, {"merge", "", false}, {"approved", "", false},
	} {
		handler := NewWebhookHandler("secret")
		received := false
		handler.OnPush(func(payload WebhookPayload) error {
			received = payload.Kind == "merge_request" && payload.GitLabMergeRequest != nil && payload.GitLabMergeRequest.IID == 42
			return nil
		})
		body := `{"object_kind":"merge_request","project":{"id":7},"object_attributes":{"iid":42,"action":"` + tc.action + `","oldrev":"` + tc.oldrev + `"}}`
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
		req.Header.Set("X-Gitlab-Token", "secret")
		response := httptest.NewRecorder()
		handler.HandleGitLabWebhook(response, req)
		if response.Code != http.StatusOK || received != tc.want {
			t.Fatalf("action=%s oldrev=%q status=%d received=%v", tc.action, tc.oldrev, response.Code, received)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"object_kind":"merge_request","object_attributes":{"action":"open"}}`))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "wrong")
	response := httptest.NewRecorder()
	NewWebhookHandler("secret").HandleGitLabWebhook(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status=%d", response.Code)
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

func TestGitLabDeliveryIDAndNonPushFiltering(t *testing.T) {
	handler := NewWebhookHandler("secret")
	var deliveries []string
	handler.OnPush(func(payload WebhookPayload) error {
		deliveries = append(deliveries, payload.DeliveryID)
		return nil
	})
	for _, tc := range []struct{ event, webhookID, idempotency string }{
		{"Push Hook", "new-id", "old-id"},
		{"Tag Push Hook", "", "old-id"},
		{"Issue Hook", "ignored", ""},
	} {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"ref":"refs/heads/main"}`))
		req.Header.Set("X-Gitlab-Token", "secret")
		req.Header.Set("X-Gitlab-Event", tc.event)
		req.Header.Set("webhook-id", tc.webhookID)
		req.Header.Set("Idempotency-Key", tc.idempotency)
		response := httptest.NewRecorder()
		handler.HandleGitLabWebhook(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("event %s status %d", tc.event, response.Code)
		}
	}
	if len(deliveries) != 2 || deliveries[0] != "new-id" || deliveries[1] != "old-id" {
		t.Fatalf("deliveries = %v", deliveries)
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
			ID         int64  `json:"id"`
			FullName   string `json:"full_name"`
			CloneURL   string `json:"clone_url"`
			GitHTTPURL string `json:"git_http_url"`
			GitSSHURL  string `json:"git_ssh_url"`
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

	if commit != "abc123def456789" {
		t.Errorf("commit = %s, want full commit", commit)
	}

	if repo != "user/repo" {
		t.Errorf("repo = %s, want user/repo", repo)
	}
}

func TestParsePushEventUsesAuthoritativeAfterAndIgnoresDeletion(t *testing.T) {
	p := WebhookPayload{Ref: "refs/heads/main", After: "0123456789abcdef0123456789abcdef01234567"}
	p.HeadCommit.ID = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	branch, commit, _ := ParsePushEvent(p)
	if branch != "main" || commit != p.After || !ValidCommitID(commit) {
		t.Fatalf("branch=%q commit=%q", branch, commit)
	}
	p.After = "0000000000000000000000000000000000000000"
	if branch, _, _ := ParsePushEvent(p); branch != "" {
		t.Fatalf("deleted ref triggered branch %q", branch)
	}
	for _, value := range []string{"", "abcdef0", "g123456789abcdef0123456789abcdef01234567", p.After} {
		if ValidCommitID(value) {
			t.Fatalf("accepted invalid commit %q", value)
		}
	}
	if !ValidCommitID("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef") {
		t.Fatal("rejected SHA-256 commit")
	}
}

func TestMatchesRepositorySupportsGitLabAndExplicitMirrorOrigin(t *testing.T) {
	var payload WebhookPayload
	payload.Project.GitHTTPURL = "https://git.example/owner/repo.git"
	if !MatchesRepository("https://git.example/owner/repo", payload) {
		t.Fatal("GitLab project HTTP clone URL did not match")
	}
	payload = WebhookPayload{}
	payload.Repository.CloneURL = "https://github.com/org/repo.git"
	if !MatchesRepository("https://mirror.example/https://github.com/org/repo.git", payload) {
		t.Fatal("configured mirror did not match its explicit origin")
	}
	for _, repository := range []string{
		"https://mirror.example/https://github.com/other/repo.git",
		"https://github.com/org%2Frepo.git",
		"https://other.example/org/repo.git",
	} {
		if MatchesRepository(repository, payload) {
			t.Fatalf("matched different repository %q", repository)
		}
	}
}
