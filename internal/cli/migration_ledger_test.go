package cli

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationLedgerFingerprint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "带空格 # 数据库.sqlite")
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO schema_migrations VALUES (1, 'one'), (2, 'two')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	first, err := migrationLedgerFingerprint(path)
	if err != nil || len(first) != len("v1:")+64 {
		t.Fatalf("fingerprint = %q, error = %v", first, err)
	}
	second, err := migrationLedgerFingerprint(path)
	if err != nil || second != first {
		t.Fatalf("repeated fingerprint = %q, error = %v", second, err)
	}
	db, err = sql.Open("sqlite", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations VALUES (3, 'three')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := migrationLedgerFingerprint(path)
	if err != nil || third == first {
		t.Fatalf("changed fingerprint = %q, error = %v", third, err)
	}
	if _, err := migrationLedgerFingerprint("relative.db"); err == nil {
		t.Fatal("relative database path accepted")
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err := migrationLedgerFingerprint(missing); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only inspection created a database")
	}
}
