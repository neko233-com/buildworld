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
	"strings"
)

type WebhookPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"head_commit"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	if !h.verifyGitHubSignature(body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	h.handlePayload(w, bytes.NewReader(body))
}

func (h *WebhookHandler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.validateRequest(w, r) {
		return
	}
	if !hmac.Equal([]byte(r.Header.Get("X-Gitlab-Token")), []byte(h.secret)) {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	h.handlePayload(w, r.Body)
}

// HandleGiteaWebhook verifies Gitea's hex-encoded SHA-256 HMAC from the
// X-Gitea-Signature header.
func (h *WebhookHandler) HandleGiteaWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.validateRequest(w, r) {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	if !h.verifyGiteaSignature(body, r.Header.Get("X-Gitea-Signature")) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	h.handlePayload(w, bytes.NewReader(body))
}

func (h *WebhookHandler) handlePayload(w http.ResponseWriter, body io.Reader) {
	var payload WebhookPayload
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		http.Error(w, "Failed to parse payload", http.StatusBadRequest)
		return
	}

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

// ParsePushEvent parses a push event and returns relevant information
func ParsePushEvent(payload WebhookPayload) (branch, commit, repo string) {
	// Extract branch from ref (refs/heads/main -> main)
	branch = strings.TrimPrefix(payload.Ref, "refs/heads/")
	commit = payload.HeadCommit.ID
	repo = payload.Repository.FullName

	// Shorten commit hash
	if len(commit) > 7 {
		commit = commit[:7]
	}

	return branch, commit, repo
}
