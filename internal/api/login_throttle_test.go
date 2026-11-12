package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestLoginThrottlesAcrossSourcesAndIgnoresForgedForwarding(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "throttle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateUser("target", "target@example.test", hash, "admin"); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: auth.NewJWT("fixture-secret")})
	login := func(username, password, peer, forwarded string) *httptest.ResponseRecorder {
		t.Helper()
		body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = peer + ":1234"
		req.Header.Set("X-Forwarded-For", forwarded)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for i := 0; i < 10; i++ {
		response := login("target", "wrong", fmt.Sprintf("192.0.2.%d", i+1), "203.0.113.1")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d", i, response.Code)
		}
	}
	if response := login("target", "correct-password", "198.51.100.1", "203.0.113.2"); response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("account limit bypassed: %d", response.Code)
	}
	for i := 0; i < 60; i++ {
		response := login(fmt.Sprintf("absent-%d", i), "wrong", "203.0.113.9", fmt.Sprintf("198.51.100.%d", i))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("source attempt %d returned %d", i, response.Code)
		}
	}
	if response := login("another", "wrong", "203.0.113.9", "different-client-claims"); response.Code != http.StatusTooManyRequests {
		t.Fatalf("forged forwarding bypassed source limit: %d", response.Code)
	}
}
