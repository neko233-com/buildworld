package cli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/glebarez/go-sqlite"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/spf13/cobra"
)

// inspectMigrationLedgerCmd only reads the ledger. The update helper uses the
// incoming binary so this works even when the installed CLI predates the check.
var inspectMigrationLedgerCmd = &cobra.Command{
	Use:    "inspect-migration-ledger",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := resolvedConfigPath(configFile)
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		fingerprint, err := migrationLedgerFingerprint(cfg.Database.Path)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), fingerprint)
		return err
	},
}

// migrationLedgerFingerprint refuses relative paths: a background service's
// working directory may differ from the detached updater's working directory.
func migrationLedgerFingerprint(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("database.path must be absolute for a safe system update")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect update database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("update database is not a regular file")
	}
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&_pragma=busy_timeout%285000%29"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return "", fmt.Errorf("open update database read-only: %w", err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		return "", fmt.Errorf("read update migration ledger: %w", err)
	}
	defer rows.Close()
	hash := sha256.New()
	version := 0
	for rows.Next() {
		var next int
		var name string
		if err := rows.Scan(&next, &name); err != nil {
			return "", fmt.Errorf("scan update migration ledger: %w", err)
		}
		if next != version+1 || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\x00\n") {
			return "", fmt.Errorf("invalid update migration ledger at version %d", next)
		}
		_, _ = fmt.Fprintf(hash, "%d\x00%s\n", next, name)
		version = next
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate update migration ledger: %w", err)
	}
	if version == 0 {
		return "", fmt.Errorf("update migration ledger is empty")
	}
	return "v1:" + hex.EncodeToString(hash.Sum(nil)), nil
}
