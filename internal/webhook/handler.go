package webhook

import (
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

func (h *WebhookHandler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify signature
	if h.secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			http.Error(w, "Missing signature", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusInternalServerError)
			return
		}

		if !h.verifySignature(body, sig) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}

		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	// Parse payload
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Failed to parse payload", http.StatusBadRequest)
		return
	}

	// Handle push event
	if h.onPush != nil {
		if err := h.onPush(payload); err != nil {
			log.Printf("Failed to handle push: %v", err)
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *WebhookHandler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify token
	if h.secret != "" {
		token := r.Header.Get("X-Gitlab-Token")
		if token != h.secret {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
	}

	// Parse payload
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Failed to parse payload", http.StatusBadRequest)
		return
	}

	// Handle push event
	if h.onPush != nil {
		if err := h.onPush(payload); err != nil {
			log.Printf("Failed to handle push: %v", err)
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *WebhookHandler) verifySignature(payload []byte, signature string) bool {
	expectedSignature := "sha256=" + hex.EncodeToString(h.computeHMAC(payload))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
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
