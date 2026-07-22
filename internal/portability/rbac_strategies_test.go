package portability

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/store"
)

func issueTestToken(t *testing.T, database *store.Store, userID int64, name string, scopes []string) {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	plain := "bw_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateAPIToken(userID, name, hex.EncodeToString(sum[:]), plain[:11], string(scopesJSON), nil); err != nil {
		t.Fatal(err)
	}
}

func TestRBACUsersAndTokensRoundTrip(t *testing.T) {
	source := openPortabilityStore(t, "rbac-source")
	aliceHash, err := auth.HashPassword("alice-original")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := source.CreateUser("alice", "alice@example.test", aliceHash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	bobHash, err := auth.HashPassword("bob-original")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.CreateUser("bob", "bob@example.test", bobHash, "user"); err != nil {
		t.Fatal(err)
	}
	issueTestToken(t, source, alice.ID, "ci-bot", []string{"build:trigger"})

	sourceRegistry, err := NewDefaultRegistry(source)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := sourceRegistry.Export(context.Background(), ExportOptions{
		Sections:       []string{"users", "api_tokens"},
		IncludeSecrets: true,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if _, ok := bundle.Sections["users"]; !ok {
		t.Fatal("users section missing from bundle")
	}
	if _, ok := bundle.Sections["api_tokens"]; !ok {
		t.Fatal("api_tokens section missing from bundle")
	}

	target := openPortabilityStore(t, "rbac-target")
	targetRegistry, err := NewDefaultRegistry(target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := targetRegistry.Import(context.Background(), bundle, ImportOptions{Mode: "overwrite"})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	importedAlice, err := target.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("imported alice: %v", err)
	}
	if importedAlice.Role != "admin" {
		t.Fatalf("alice role = %q, want admin", importedAlice.Role)
	}
	importedBob, err := target.GetUserByUsername("bob")
	if err != nil {
		t.Fatalf("imported bob: %v", err)
	}
	if importedBob.Role != "user" {
		t.Fatalf("bob role = %q, want user", importedBob.Role)
	}

	// The original password must NOT survive the migration: imported users get
	// a reset password so credentials never travel inside a bundle.
	if importedAlice.PasswordHash == aliceHash {
		t.Fatal("imported user kept the original password hash; secrets leaked")
	}
	if auth.CheckPassword(importedAlice.PasswordHash, "alice-original") {
		t.Fatal("imported user is still authenticable with the original password")
	}

	// API token grant must be re-issued with a fresh secret.
	tokens, err := target.ListAPITokens(importedAlice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Name != "ci-bot" {
		t.Fatalf("alice token = %#v, want exactly ci-bot", tokens)
	}
	var scopes []string
	if err := json.Unmarshal([]byte(tokens[0].Scopes), &scopes); err != nil || len(scopes) != 1 || scopes[0] != "build:trigger" {
		t.Fatalf("alice token scopes = %q, want [build:trigger]", tokens[0].Scopes)
	}
	var issued []IssuedSecret
	for _, sec := range result.Sections {
		if sec.Key == "api_tokens" {
			issued = sec.Issued
		}
	}
	if len(issued) != 1 || issued[0].Token == "" {
		t.Fatalf("expected exactly one re-issued token secret, got %#v", issued)
	}

	// A second import in skip mode must not create duplicates.
	second, err := targetRegistry.Import(context.Background(), bundle, ImportOptions{Mode: "skip"})
	if err != nil {
		t.Fatalf("second Import(skip) error = %v", err)
	}
	for _, section := range second.Sections {
		if section.Count > 0 && section.Skipped != section.Count {
			t.Fatalf("second import section = %#v, want all existing records skipped", section)
		}
	}
	tokensAfter, err := target.ListAPITokens(importedAlice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokensAfter) != 1 {
		t.Fatalf("token count after skip re-import = %d, want 1", len(tokensAfter))
	}
}

func TestRBACAPITokenSkipsUnknownOwner(t *testing.T) {
	source := openPortabilityStore(t, "rbac-orphan-source")
	orphan := []apiTokenRecord{{Username: "ghost", Name: "lost", Scopes: []string{"build:trigger"}}}
	data, err := json.Marshal(orphan)
	if err != nil {
		t.Fatal(err)
	}
	s := apiTokenStrategy{store: source}
	res, err := s.Import(context.Background(), data, ImportOptions{Mode: "overwrite"})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if res.Count != 1 || res.Skipped != 1 {
		t.Fatalf("orphan token result = %#v, want count 1 skipped 1", res)
	}
}
