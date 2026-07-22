package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestSchemaMigrationsCreateFreshDatabase(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	if got, want := readMigrationLedger(t, data.db), []string{
		"1:reconcile-current-schema",
		"2:project-groups-single-level",
		"3:binary-plugins-only",
		"4:user-session-version",
		"5:remove-inert-deployment-and-project-hooks",
		"6:remove-dead-account-ssh-keys",
		"7:repair-orphan-project-history-references",
		"8:project-favorite-quick-access",
		"9:project-pipeline-source",
		"10:project-enabled",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("migration ledger = %v, want %v", got, want)
	}
	for _, column := range []string{"vcs_root_id", "template_id", "tags", "group_id"} {
		if !databaseColumnExists(t, data.db, "projects", column) {
			t.Fatalf("fresh projects table missing %q", column)
		}
	}
	for _, column := range []string{"color", "updated_at"} {
		if !databaseColumnExists(t, data.db, "project_groups", column) {
			t.Fatalf("fresh project_groups table missing %q", column)
		}
	}
	if databaseColumnExists(t, data.db, "project_groups", "parent_id") {
		t.Fatal("fresh project_groups table contains development parent_id")
	}
	if !databaseColumnExists(t, data.db, "users", "session_version") {
		t.Fatal("fresh users table missing session_version")
	}
	for _, table := range []string{"deployment_envs", "git_hooks", "ssh_keys"} {
		if databaseTableExists(t, data.db, table) {
			t.Fatalf("fresh database contains removed table %q", table)
		}
	}
	for _, column := range []string{"id", "name", "version", "description", "author", "enabled", "config", "path", "source", "installed_at", "updated_at"} {
		if !databaseColumnExists(t, data.db, "plugins", column) {
			t.Fatalf("fresh plugins table missing %q", column)
		}
	}
	for _, column := range []string{"script_lang", "source_script", "source_ui_script", "steps", "triggers", "ui_extensions"} {
		if databaseColumnExists(t, data.db, "plugins", column) {
			t.Fatalf("fresh plugins table contains legacy column %q", column)
		}
	}
}

func TestUserSessionVersionMigrationUpgradesVersionedDatabase(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "session-version.db"))
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer'
	)`); err != nil {
		t.Fatal(err)
	}
	if err := ensureMigrationLedger(db); err != nil {
		t.Fatal(err)
	}
	for _, entry := range schemaMigrations[:3] {
		if _, err := db.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", entry.version, entry.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ('existing', 'existing@example.test', 'hash', 'admin')`); err != nil {
		t.Fatal(err)
	}
	createProjectHistoryRepairFixtureTables(t, db)

	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatal(err)
	}
	var sessionVersion int64
	if err := db.QueryRow("SELECT session_version FROM users WHERE username = 'existing'").Scan(&sessionVersion); err != nil {
		t.Fatal(err)
	}
	if sessionVersion != 1 {
		t.Fatalf("migrated session_version = %d, want 1", sessionVersion)
	}
	if got := readMigrationLedger(t, db); !reflect.DeepEqual(got, []string{
		"1:reconcile-current-schema",
		"2:project-groups-single-level",
		"3:binary-plugins-only",
		"4:user-session-version",
		"5:remove-inert-deployment-and-project-hooks",
		"6:remove-dead-account-ssh-keys",
		"7:repair-orphan-project-history-references",
		"8:project-favorite-quick-access",
		"9:project-pipeline-source",
		"10:project-enabled",
	}) {
		t.Fatalf("migration ledger = %v", got)
	}
}

func TestRemovedFeatureSchemaMigrationDropsTablesAndIndexes(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "removed-features.db"))
	defer db.Close()
	statements := []string{
		`CREATE TABLE deployment_envs (id INTEGER PRIMARY KEY, project_id INTEGER NOT NULL)`,
		`CREATE INDEX idx_deployment_envs_project ON deployment_envs(project_id)`,
		`CREATE TABLE git_hooks (id INTEGER PRIMARY KEY, project_id INTEGER NOT NULL)`,
		`CREATE INDEX idx_git_hooks_project ON git_hooks(project_id)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	createProjectHistoryRepairFixtureTables(t, db)
	if err := ensureMigrationLedger(db); err != nil {
		t.Fatal(err)
	}
	for _, entry := range schemaMigrations[:4] {
		if _, err := db.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", entry.version, entry.name); err != nil {
			t.Fatal(err)
		}
	}

	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatal(err)
	}
	for _, object := range []struct{ kind, name string }{
		{kind: "table", name: "deployment_envs"},
		{kind: "index", name: "idx_deployment_envs_project"},
		{kind: "table", name: "git_hooks"},
		{kind: "index", name: "idx_git_hooks_project"},
	} {
		if databaseObjectExists(t, db, object.kind, object.name) {
			t.Fatalf("removed %s %q still exists", object.kind, object.name)
		}
	}
	if got := readMigrationLedger(t, db); got[len(got)-1] != "10:project-enabled" {
		t.Fatalf("migration ledger = %v", got)
	}
}

func TestSSHKeysSchemaMigrationDropsDeadTableAndPreservesRepositoryCredentials(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "remove-ssh-keys.db"))
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT NOT NULL)`,
		`CREATE TABLE ssh_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			public_key TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			type TEXT NOT NULL,
			host TEXT NOT NULL DEFAULT '',
			username TEXT,
			password TEXT,
			private_key TEXT,
			public_key TEXT,
			token TEXT,
			description TEXT,
			is_secret BOOLEAN DEFAULT TRUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO users (id, username) VALUES (7, 'owner')`,
		`INSERT INTO ssh_keys (id, user_id, name, public_key, fingerprint)
			VALUES (8, 7, 'dead-account-key', 'ssh-ed25519 dead', 'dead-fingerprint')`,
		`INSERT INTO credentials (id, name, type, host, private_key, public_key, is_secret)
			VALUES (9, 'active-repository-key', 'ssh_key', 'git.example.test', 'private-value', 'ssh-ed25519 active', 1)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	createProjectHistoryRepairFixtureTables(t, db)
	if err := ensureMigrationLedger(db); err != nil {
		t.Fatal(err)
	}
	for _, entry := range schemaMigrations[:5] {
		if _, err := db.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", entry.version, entry.name); err != nil {
			t.Fatal(err)
		}
	}

	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatal(err)
	}
	if databaseTableExists(t, db, "ssh_keys") {
		t.Fatal("dead ssh_keys table remains after migration")
	}
	var credentialType, privateKey, publicKey string
	if err := db.QueryRow("SELECT type, private_key, public_key FROM credentials WHERE id=9").Scan(&credentialType, &privateKey, &publicKey); err != nil {
		t.Fatal(err)
	}
	if credentialType != "ssh_key" || privateKey != "private-value" || publicKey != "ssh-ed25519 active" {
		t.Fatalf("repository credential changed: type=%q private=%q public=%q", credentialType, privateKey, publicKey)
	}
	if got := readMigrationLedger(t, db); got[len(got)-1] != "10:project-enabled" {
		t.Fatalf("migration ledger = %v", got)
	}
}

func TestPluginSchemaMigrationPreservesBinaryPluginsOnly(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "binary-plugins.db"))
	defer db.Close()
	statements := []string{
		`CREATE TABLE plugins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			version TEXT NOT NULL,
			description TEXT,
			author TEXT DEFAULT '',
			enabled BOOLEAN DEFAULT TRUE,
			config TEXT,
			path TEXT DEFAULT '',
			source TEXT DEFAULT 'builtin',
			steps TEXT DEFAULT '[]',
			triggers TEXT DEFAULT '[]',
			ui_extensions TEXT DEFAULT '[]',
			script_lang TEXT DEFAULT 'js',
			source_script TEXT DEFAULT '',
			source_ui_script TEXT DEFAULT '',
			installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO plugins (id, name, version, description, author, enabled, config, path, source, script_lang)
			VALUES (12, 'binary', '2.1.0', 'binary plugin', 'acme', 0, '{"safe":true}', '/plugins/binary', 'https://github.com/acme/binary', 'go')`,
		`INSERT INTO plugins (id, name, version, source, script_lang, source_script)
			VALUES (13, 'legacy-script', '1.0.0', 'upload', 'js', 'registerStep("bad", () => {})')`,
		`INSERT INTO plugins (id, name, version, source, script_lang, source_script)
			VALUES (14, 'legacy-github-script', '1.0.0', 'https://github.com/acme/legacy-script', 'ts', 'registerStep("bad", () => {})')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"script_lang", "source_script", "source_ui_script", "steps", "triggers", "ui_extensions"} {
		if databaseColumnExists(t, db, "plugins", column) {
			t.Fatalf("legacy plugin column %q remains", column)
		}
	}
	var id int64
	var enabled bool
	var source, path string
	if err := db.QueryRow("SELECT id, enabled, source, path FROM plugins WHERE name='binary'").Scan(&id, &enabled, &source, &path); err != nil {
		t.Fatal(err)
	}
	if id != 12 || enabled || source != "https://github.com/acme/binary" || path != "/plugins/binary" {
		t.Fatalf("migrated binary plugin = id %d, enabled %t, source %q, path %q", id, enabled, source, path)
	}
	var legacyCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM plugins WHERE name IN ('legacy-script', 'legacy-github-script')").Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatal("legacy script plugin record survived migration")
	}
}

func TestSchemaMigrationsUpgradeUnversionedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "upgrade.db")
	db := openMigrationTestDatabase(t, dbPath)
	legacySchema := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			repo_url TEXT NOT NULL,
			repo_type TEXT NOT NULL,
			default_branch TEXT DEFAULT 'main',
			config TEXT NOT NULL,
			created_by INTEGER REFERENCES users(id),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO users (id, username, email, password_hash) VALUES (7, 'legacy', 'legacy@example.test', 'hash')`,
		`INSERT INTO projects (id, name, description, repo_url, repo_type, config, created_by) VALUES (11, 'legacy project', '', '', 'git', '{}', 7)`,
	}
	for _, statement := range legacySchema {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.GetProject(11)
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "legacy project" || project.CreatedBy != 7 {
		t.Fatalf("legacy project changed during migration: %#v", project)
	}
	if project.Tags == nil || len(project.Tags) != 0 {
		t.Fatalf("migrated project tags = %#v, want empty non-nil list", project.Tags)
	}
	if got := readMigrationLedger(t, data.db); len(got) != len(schemaMigrations) {
		t.Fatalf("migration ledger = %v, want %d entries", got, len(schemaMigrations))
	}
}

func TestSchemaMigrationsDoNotRepeatAppliedSteps(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "repeat.db"))
	defer db.Close()
	testMigrations := []schemaMigration{{
		version: 1,
		name:    "insert-once",
		up: func(tx *sql.Tx) error {
			if _, err := tx.Exec("CREATE TABLE repeat_guard (value TEXT NOT NULL UNIQUE)"); err != nil {
				return err
			}
			_, err := tx.Exec("INSERT INTO repeat_guard (value) VALUES ('once')")
			return err
		},
	}}
	if err := runSchemaMigrations(db, testMigrations); err != nil {
		t.Fatal(err)
	}
	if err := runSchemaMigrations(db, testMigrations); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM repeat_guard").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("repeat guard rows = %d, want 1", count)
	}
	if got := readMigrationLedger(t, db); !reflect.DeepEqual(got, []string{"1:insert-once"}) {
		t.Fatalf("migration ledger after repeat = %v", got)
	}
}

func TestProjectGroupSchemaMigrationPreservesForeignKeysWhenEnabled(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "foreign-keys.db"))
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE project_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			parent_id INTEGER REFERENCES project_groups(id),
			color TEXT NOT NULL DEFAULT 'neutral',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			repo_url TEXT NOT NULL,
			repo_type TEXT NOT NULL,
			default_branch TEXT DEFAULT 'main',
			config TEXT NOT NULL,
			group_id INTEGER REFERENCES project_groups(id),
			created_by INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO project_groups (id, name, color) VALUES (41, 'root', 'blue')`,
		`INSERT INTO project_groups (id, name, parent_id, color) VALUES (42, 'child', 41, 'mint')`,
		`INSERT INTO projects (id, name, description, repo_url, repo_type, config, group_id, created_by)
			VALUES (43, 'linked', '', '', 'git', '{}', 42, 0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatal(err)
	}
	var foreignKeysEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		t.Fatal(err)
	}
	if foreignKeysEnabled != 1 {
		t.Fatal("migration did not restore foreign-key enforcement")
	}
	if databaseColumnExists(t, db, "project_groups", "parent_id") {
		t.Fatal("parent_id remains after migration with foreign keys enabled")
	}
	var groupID int64
	if err := db.QueryRow("SELECT group_id FROM projects WHERE id=43").Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	if groupID != 42 {
		t.Fatalf("project group id = %d, want 42", groupID)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("project group migration left a foreign-key violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaMigrationFailureRollsBackWithoutLedgerEntry(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "rollback.db"))
	defer db.Close()
	failure := errors.New("forced migration failure")
	testMigrations := []schemaMigration{
		{
			version: 1,
			name:    "stable",
			up: func(tx *sql.Tx) error {
				_, err := tx.Exec("CREATE TABLE stable_data (value TEXT NOT NULL)")
				return err
			},
		},
		{
			version: 2,
			name:    "fails",
			up: func(tx *sql.Tx) error {
				if _, err := tx.Exec("INSERT INTO stable_data (value) VALUES ('must roll back')"); err != nil {
					return err
				}
				if _, err := tx.Exec("CREATE TABLE must_roll_back (id INTEGER PRIMARY KEY)"); err != nil {
					return err
				}
				return failure
			},
		},
	}
	err := runSchemaMigrations(db, testMigrations)
	if !errors.Is(err, failure) {
		t.Fatalf("migration error = %v, want forced failure", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM stable_data").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed migration left %d data rows", count)
	}
	if databaseTableExists(t, db, "must_roll_back") {
		t.Fatal("failed migration left its DDL behind")
	}
	if got := readMigrationLedger(t, db); !reflect.DeepEqual(got, []string{"1:stable"}) {
		t.Fatalf("failed migration ledger = %v, want only version 1", got)
	}
}

func openMigrationTestDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func readMigrationLedger(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var entries []string
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, fmt.Sprintf("%d:%s", version, name))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func databaseColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	exists, err := sqliteColumnExists(db, table, column)
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

func databaseTableExists(t *testing.T, db *sql.DB, table string) bool {
	return databaseObjectExists(t, db, "table", table)
}

func databaseObjectExists(t *testing.T, db *sql.DB, kind, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?", kind, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count == 1
}
