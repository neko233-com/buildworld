package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestMaskCredentialSecretsPreservesEmptyFields(t *testing.T) {
	credential := &store.Credential{Password: "password", PrivateKey: "", Token: "token"}
	maskCredentialSecrets(credential)

	if credential.Password != "********" || credential.Token != "********" {
		t.Fatalf("non-empty secrets were not masked: %+v", credential)
	}
	if credential.PrivateKey != "" {
		t.Fatalf("empty private key = %q, want empty", credential.PrivateKey)
	}
}

func newTestRouter() http.Handler {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 6050},
	}
	return NewRouter(Deps{Cfg: cfg})
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

func TestJSONResponsesSupportGzipCompression(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if encoding := w.Header().Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", encoding)
	}
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]string
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("compressed response is not valid JSON: %v", err)
	}
	if response["status"] != "ok" {
		t.Fatalf("status = %q, want ok", response["status"])
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
	if !strings.Contains(strings.ToLower(ct), "charset=utf-8") {
		t.Errorf("Content-Type = %s, want UTF-8 charset", ct)
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
