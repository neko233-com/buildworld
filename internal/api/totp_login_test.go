package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestTOTPRequiredLoginAndReplayProtection(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "totp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := database.CreateUser("target", "target@example.test", hash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Auth: config.AuthConfig{TOTPRequired: true, TOTPSecrets: map[string]string{"target": "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"}}}
	jwt := auth.NewJWT("fixture-secret")
	router := NewRouter(Deps{Cfg: cfg, Store: database, JWT: jwt})
	oldToken, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	me := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := me(oldToken); response.Code != http.StatusUnauthorized {
		t.Fatalf("pre-MFA session remained valid: %d", response.Code)
	}
	login := func(code string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"target","password":"correct-password","totp_code":%q}`, code)))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := login(""); response.Code != http.StatusUnauthorized {
		t.Fatalf("login without TOTP returned %d", response.Code)
	}
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, []byte("12345678901234567890"))
	_, _ = mac.Write(message[:])
	digest := mac.Sum(nil)
	offset := int(digest[len(digest)-1] & 15)
	code := fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1_000_000)
	response := login(code)
	if response.Code != http.StatusOK {
		t.Fatalf("valid TOTP returned %d: %s", response.Code, response.Body.String())
	}
	var session loginResp
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || me(session.Token).Code != http.StatusOK {
		t.Fatalf("MFA session unusable: %v", err)
	}
	if response := login(code); response.Code != http.StatusUnauthorized {
		t.Fatalf("replayed TOTP returned %d", response.Code)
	}
	cfg.Auth.TOTPSecrets["target"] = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	if response := me(session.Token); response.Code != http.StatusUnauthorized {
		t.Fatalf("session remained valid after TOTP rotation: %d", response.Code)
	}
	delete(cfg.Auth.TOTPSecrets, "target")
	if response := me(session.Token); response.Code != http.StatusUnauthorized {
		t.Fatalf("session remained valid after TOTP removal: %d", response.Code)
	}
}
