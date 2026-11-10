package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type WebhookPayload struct {
	Provider   string `json:"-"`
	DeliveryID string `json:"-"`
	BodyHash   string `json:"-"`
	Kind       string `json:"kind,omitempty"`
	Action     string `json:"action,omitempty"`
	Number     int64  `json:"number,omitempty"`
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		ID         int64  `json:"id"`
		FullName   string `json:"full_name"`
		CloneURL   string `json:"clone_url"`
		GitHTTPURL string `json:"git_http_url"`
		GitSSHURL  string `json:"git_ssh_url"`
	} `json:"repository"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
		GitHTTPURL        string `json:"git_http_url"`
		GitSSHURL         string `json:"git_ssh_url"`
	} `json:"project"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"head_commit"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
	PullRequest *PullRequestPayload `json:"pull_request,omitempty"`
	Change      *PullRequestChange  `json:"change,omitempty"`
}

// PullRequestPayload contains only provider fields used to authorize and pin a PR build.
type PullRequestPayload struct {
	Number int64  `json:"number"`
	State  string `json:"state"`
	Title  string `json:"title"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo *struct {
			ID int64 `json:"id"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo *struct {
			ID int64 `json:"id"`
		} `json:"repo"`
	} `json:"base"`
}

// PullRequestChange is the bounded, credential-free metadata persisted for a build.
type PullRequestChange struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Branch string `json:"branch"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

// MatchesRepository accepts the clone identities emitted by supported push providers.
func MatchesRepository(repository string, payload WebhookPayload) bool {
	if repository == "" {
		return false
	}
	configured := []string{repository}
	if outer, err := url.Parse(repository); err == nil && (outer.Scheme == "https" || outer.Scheme == "http") {
		// Some Git mirrors use https://mirror/https://origin/path. Match the
		// explicit embedded origin as well as the configured mirror URL.
		inner := strings.TrimPrefix(outer.EscapedPath(), "/")
		if parsed, err := url.Parse(inner); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" {
			configured = append(configured, inner)
		}
	}
	for _, candidate := range []string{payload.Repository.CloneURL, payload.Repository.FullName, payload.Repository.GitHTTPURL, payload.Repository.GitSSHURL, payload.Project.GitHTTPURL, payload.Project.GitSSHURL, payload.Project.PathWithNamespace} {
		if candidate == "" {
			continue
		}
		for _, value := range configured {
			if repositoryIdentity(value) == repositoryIdentity(candidate) {
				return true
			}
		}
	}
	return false
}

func repositoryIdentity(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return raw
	}
	path := strings.TrimSuffix(strings.TrimSuffix(parsed.EscapedPath(), "/"), ".git")
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + path
}

type WebhookHandler struct {
	secret string
	onPush func(payload WebhookPayload) error
}

func NewWebhookHandler(secret string) *WebhookHandler {
	return &WebhookHandler{
		secret: secret,
	}
}

func (h *WebhookHandler) OnPush(handler func(payload WebhookPayload) error) {
	h.onPush = handler
}

func (h *WebhookHandler) validateRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if strings.TrimSpace(h.secret) == "" {
		http.Error(w, "Webhook secret not configured", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (h *WebhookHandler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.validateRequest(w, r) {
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	if !h.verifyGitHubSignature(body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	h.handlePayload(w, bytes.NewReader(body), "github", r.Header.Get("X-GitHub-Delivery"), r.Header.Get("X-GitHub-Event"), body)
}

func (h *WebhookHandler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.validateRequest(w, r) {
		return
	}
	if !hmac.Equal([]byte(r.Header.Get("X-Gitlab-Token")), []byte(h.secret)) {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	deliveryID := r.Header.Get("webhook-id")
	if deliveryID == "" {
		deliveryID = r.Header.Get("Idempotency-Key")
	}
	h.handlePayload(w, bytes.NewReader(body), "gitlab", deliveryID, r.Header.Get("X-Gitlab-Event"), body)
}

// HandleGiteaWebhook verifies Gitea's hex-encoded SHA-256 HMAC from the
// X-Gitea-Signature header.
func (h *WebhookHandler) HandleGiteaWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.validateRequest(w, r) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	if !h.verifyGiteaSignature(body, r.Header.Get("X-Gitea-Signature")) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	h.handlePayload(w, bytes.NewReader(body), "gitea", r.Header.Get("X-Gitea-Delivery"), r.Header.Get("X-Gitea-Event"), body)
}

func (h *WebhookHandler) handlePayload(w http.ResponseWriter, body io.Reader, provider, deliveryID, event string, raw []byte) {
	// Providers send other event types to the same endpoint. PR events are
	// identified by the verified provider header, not by a body-supplied field.
	event = strings.ToLower(strings.TrimSpace(event))
	prEvent := (provider == "github" && event == "pull_request") || (provider == "gitea" && (event == "pull_request" || event == "pull_request_sync"))
	if !prEvent && event != "" && event != "push" && !(provider == "gitlab" && (event == "push hook" || event == "tag push hook")) {
		w.WriteHeader(http.StatusOK)
		return
	}
	var payload WebhookPayload
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		http.Error(w, "Failed to parse payload", http.StatusBadRequest)
		return
	}
	payload.Kind = "push"
	if prEvent {
		payload.Kind = "pull_request"
		action := strings.ToLower(strings.TrimSpace(payload.Action))
		if action != "opened" && action != "reopened" && action != "synchronize" && !(provider == "gitea" && action == "synchronized") {
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	hash := sha256.Sum256(raw)
	payload.Provider, payload.DeliveryID, payload.BodyHash = provider, deliveryID, hex.EncodeToString(hash[:])

	if h.onPush != nil {
		if err := h.onPush(payload); err != nil {
			log.Printf("Failed to handle push: %v", err)
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *WebhookHandler) verifyGitHubSignature(payload []byte, signature string) bool {
	if signature == "" {
		return false
	}
	expectedSignature := "sha256=" + hex.EncodeToString(h.computeHMAC(payload))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (h *WebhookHandler) verifyGiteaSignature(payload []byte, signature string) bool {
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(provided) == 0 {
		return false
	}
	return hmac.Equal(provided, h.computeHMAC(payload))
}

func (h *WebhookHandler) computeHMAC(payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(payload)
	return mac.Sum(nil)
}

// ParsePushEvent returns the full immutable ref target. A deleted or non-push
// ref has an empty branch so callers do not accidentally build the latest tip.
func ParsePushEvent(payload WebhookPayload) (branch, commit, repo string) {
	ref := strings.TrimSpace(payload.Ref)
	if ref == "" || (payload.After != "" && strings.Trim(payload.After, "0") == "") {
		return "", "", payload.Repository.FullName
	}
	branch = strings.TrimPrefix(ref, "refs/heads/")
	commit = payload.After
	if commit == "" {
		commit = payload.HeadCommit.ID
	}
	repo = payload.Repository.FullName
	return branch, commit, repo
}

// ValidCommitID accepts complete SHA-1 and SHA-256 object IDs only.
func ValidCommitID(commit string) bool {
	if len(commit) != 40 && len(commit) != 64 {
		return false
	}
	for _, c := range commit {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return strings.Trim(commit, "0") != ""
}
