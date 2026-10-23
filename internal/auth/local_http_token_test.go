package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func localTokenDatabase(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "local.db")
	db, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := SetupDefaultAdmin(db); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func TestLocalHTTPTokenIsPrivatePersistentAndScoped(t *testing.T) {
	db, databasePath := localTokenDatabase(t)
	path, generated, err := EnsureLocalHTTPToken(db, databasePath)
	if err != nil || !generated {
		t.Fatalf("first provisioning: generated=%v err=%v", generated, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	principal, valid := ResolveAPIToken(db, strings.TrimSpace(string(body)))
	if !valid || principal.Role != "admin" || len(principal.Scopes) != 1 {
		t.Fatal("missing scoped current administrator")
	}
	if _, ok := principal.Scopes[ScopeBuildTrigger]; !ok {
		t.Fatal("build trigger scope missing")
	}
	if _, ok := principal.Scopes[ScopeSystemUpdate]; ok {
		t.Fatal("unrequested system update scope")
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("token file is not private")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.New(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	secondPath, secondGenerated, err := EnsureLocalHTTPToken(reopened, databasePath)
	if err != nil || secondGenerated || secondPath != path {
		t.Fatalf("restart provisioning: generated=%v err=%v", secondGenerated, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(body) {
		t.Fatal("restart rotated the token")
	}
	tokens, _ := reopened.ListAPITokens(principal.UserID)
	if len(tokens) != 1 {
		t.Fatalf("token count %d, want one", len(tokens))
	}
}

func TestLocalHTTPTokenDoesNotUndoRevocationOrDeletedOwner(t *testing.T) {
	for _, removeOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "revoked", true: "deleted-owner"}[removeOwner], func(t *testing.T) {
			db, databasePath := localTokenDatabase(t)
			path, _, err := EnsureLocalHTTPToken(db, databasePath)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(path)
			principal, _ := ResolveAPIToken(db, strings.TrimSpace(string(body)))
			if removeOwner {
				err = db.DeleteUser(principal.UserID)
			} else {
				err = db.DeleteAPIToken(principal.TokenID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, generated, err := EnsureLocalHTTPToken(db, databasePath); err == nil || generated {
				t.Fatal("revoked authority was restored")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(body) {
				t.Fatal("revoked token file overwritten")
			}
		})
	}
}

func TestLocalHTTPTokenRejectsUnsafeOrCorruptFiles(t *testing.T) {
	for name, body := range map[string]string{"empty": "", "shape": "invalid", "encoding": "bw_" + strings.Repeat("x", 64), "oversize": strings.Repeat("a", 256)} {
		t.Run(name, func(t *testing.T) {
			db, databasePath := localTokenDatabase(t)
			path, _ := LocalHTTPTokenPath(databasePath)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, generated, err := EnsureLocalHTTPToken(db, databasePath); err == nil || generated {
				t.Fatal("unsafe file accepted")
			}
			after, _ := os.ReadFile(path)
			if string(after) != body {
				t.Fatal("corrupt evidence overwritten")
			}
		})
	}
	db, databasePath := localTokenDatabase(t)
	path, _ := LocalHTTPTokenPath(databasePath)
	target := filepath.Join(filepath.Dir(path), "other-secret")
	_ = os.WriteFile(target, []byte("unrelated"), 0o600)
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink privilege unavailable")
		}
		t.Fatal(err)
	}
	if _, _, err := EnsureLocalHTTPToken(db, databasePath); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestLocalHTTPTokenHonorsOwnerDemotion(t *testing.T) {
	db, databasePath := localTokenDatabase(t)
	path, _, err := EnsureLocalHTTPToken(db, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := db.GetUserByUsername("root")
	if err := db.UpdateUserRole(root.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, generated, err := EnsureLocalHTTPToken(db, databasePath); err == nil || generated {
		t.Fatal("owner demotion was undone")
	}
	body, _ := os.ReadFile(path)
	principal, valid := ResolveAPIToken(db, strings.TrimSpace(string(body)))
	if !valid || principal.Role != "viewer" {
		t.Fatal("current role was not enforced")
	}
}

func TestLocalHTTPTokenConcurrentProvisioningCreatesOneCredential(t *testing.T) {
	db, databasePath := localTokenDatabase(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, _ = EnsureLocalHTTPToken(db, databasePath) }()
	}
	wg.Wait()
	if _, generated, err := EnsureLocalHTTPToken(db, databasePath); err != nil || generated {
		t.Fatalf("final token invalid: %v", err)
	}
	root, _ := db.GetUserByUsername("root")
	tokens, _ := db.ListAPITokens(root.ID)
	if len(tokens) != 1 {
		t.Fatalf("concurrent token count=%d", len(tokens))
	}
	for _, invalid := range []string{"", ":memory:", "file:memory.db"} {
		if _, err := LocalHTTPTokenPath(invalid); err == nil {
			t.Fatal("nonpersistent path accepted")
		}
	}
}
