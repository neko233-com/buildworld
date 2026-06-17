package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld233/internal/config"
)

func newTestRouter() http.Handler {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 6050},
	}
	return NewRouter(cfg)
}

func TestHealthEndpoint(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["status"] != "ok" {
		t.Errorf("status = %s, want ok", resp["status"])
	}
}

func TestHealthEndpointMethodNotAllowed(t *testing.T) {
	router := newTestRouter()

	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		req := httptest.NewRequest(method, "/api/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: status = %d, want 405", method, w.Code)
		}
	}
}

func TestVersionEndpoint(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["version"] != "0.1.0" {
		t.Errorf("version = %s, want 0.1.0", resp["version"])
	}
}

func TestVersionEndpointMethodNotAllowed(t *testing.T) {
	router := newTestRouter()

	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		req := httptest.NewRequest(method, "/api/version", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: status = %d, want 405", method, w.Code)
		}
	}
}

func TestCORSHeaders(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	allow := w.Header().Get("Allow")
	if allow == "" && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("OPTIONS request returned status %d", w.Code)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("X-Request-ID header not set")
	}
}

func TestRequestIDPreservedWhenProvided(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Header.Set("X-Request-ID", "my-custom-id-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	if requestID != "my-custom-id-123" {
		t.Errorf("X-Request-ID = %s, want my-custom-id-123", requestID)
	}
}

func Test404Handling(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestRoot404(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestContentTypeJSON(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestVersionContentTypeJSON(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestHealthResponseIsJSON(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

func TestVersionResponseIsJSON(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

func TestMultipleRequestsConsistency(t *testing.T) {
	router := newTestRouter()

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i, w.Code)
		}

		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)

		if resp["status"] != "ok" {
			t.Errorf("request %d: status = %s, want ok", i, resp["status"])
		}
	}
}
