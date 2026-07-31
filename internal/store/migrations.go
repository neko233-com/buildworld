package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type schemaMigration struct {
	version            int
	name               string
	disableForeignKeys bool
	up                 func(*sql.Tx) error
}

var currentSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		session_version INTEGER NOT NULL DEFAULT 1,
		avatar_url TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME
	)`,
	`ALTER TABLE users ADD COLUMN session_version INTEGER NOT NULL DEFAULT 1`,
	`CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		repo_url TEXT NOT NULL,
		repo_type TEXT NOT NULL,
		default_branch TEXT DEFAULT 'main',
		config TEXT NOT NULL,
		pipeline_format TEXT NOT NULL DEFAULT 'yaml',
		pipeline_source_mode TEXT NOT NULL DEFAULT 'inline',
		pipeline_scm_repo TEXT NOT NULL DEFAULT '',
		pipeline_scm_branch TEXT NOT NULL DEFAULT '',
		pipeline_scm_path TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		http_trigger_enabled BOOLEAN NOT NULL DEFAULT FALSE,
		http_trigger_token TEXT NOT NULL DEFAULT '',
		created_by INTEGER REFERENCES users(id),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS builds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id),
		number INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		trigger TEXT NOT NULL,
		branch TEXT,
		commit_sha TEXT,
		parameters TEXT,
		log TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		duration_ms INTEGER,
		UNIQUE(project_id, number)
	)`,
	`CREATE TABLE IF NOT EXISTS artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id INTEGER NOT NULL REFERENCES builds(id),
		name TEXT NOT NULL,
		path TEXT NOT NULL,
		size INTEGER,
		sha256 TEXT DEFAULT '',
		content_type TEXT DEFAULT '',
		downloads INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS workers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		address TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		labels TEXT,
		pool TEXT DEFAULT '',
		max_concurrent_builds INTEGER DEFAULT 4,
		status TEXT DEFAULT 'offline',
		last_heartbeat DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS plugins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		version TEXT NOT NULL,
		description TEXT,
		author TEXT DEFAULT '',
		enabled BOOLEAN DEFAULT TRUE,
		config TEXT,
		path TEXT DEFAULT '',
		source TEXT DEFAULT '',
		installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS env_vars (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scope TEXT NOT NULL,
		project_id INTEGER,
		name TEXT NOT NULL,
		value TEXT NOT NULL,
		is_secret BOOLEAN DEFAULT FALSE,
		description TEXT,
		UNIQUE(scope, project_id, name)
	)`,
	`CREATE TABLE IF NOT EXISTS credentials (
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
	`CREATE INDEX IF NOT EXISTS idx_credentials_type ON credentials(type)`,
	`CREATE INDEX IF NOT EXISTS idx_credentials_host ON credentials(host)`,
	`CREATE TABLE IF NOT EXISTS vcs_roots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		type TEXT NOT NULL,
		url TEXT NOT NULL,
		branch TEXT DEFAULT 'main',
		credential_id INTEGER REFERENCES credentials(id),
		poll_interval INTEGER DEFAULT 60,
		auto_checkout BOOLEAN DEFAULT TRUE,
		config TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS build_templates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		config TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`ALTER TABLE projects ADD COLUMN vcs_root_id INTEGER REFERENCES vcs_roots(id)`,
	`ALTER TABLE projects ADD COLUMN template_id INTEGER REFERENCES build_templates(id)`,
	`ALTER TABLE projects ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`,
	`ALTER TABLE builds ADD COLUMN wait_dependency_on INTEGER REFERENCES builds(id)`,
	`ALTER TABLE builds ADD COLUMN retried_from INTEGER REFERENCES builds(id)`,
	`ALTER TABLE builds ADD COLUMN pinned BOOLEAN DEFAULT FALSE`,
	`ALTER TABLE workers ADD COLUMN pool TEXT DEFAULT ''`,
	`ALTER TABLE artifacts ADD COLUMN sha256 TEXT DEFAULT ''`,
	`ALTER TABLE artifacts ADD COLUMN content_type TEXT DEFAULT ''`,
	`ALTER TABLE artifacts ADD COLUMN downloads INTEGER DEFAULT 0`,
	`CREATE TABLE IF NOT EXISTS notification_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		type TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		enabled BOOLEAN DEFAULT TRUE,
		conditions TEXT DEFAULT '{}',
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS notification_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel_id INTEGER NOT NULL REFERENCES notification_channels(id),
		build_id INTEGER REFERENCES builds(id),
		event_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		error_message TEXT,
		delivered_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_notification_events_channel ON notification_events(channel_id)`,
	`CREATE INDEX IF NOT EXISTS idx_notification_events_build ON notification_events(build_id)`,
	`CREATE TABLE IF NOT EXISTS web_notification_reads (
		user_id INTEGER PRIMARY KEY REFERENCES users(id),
		last_event_id INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS build_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id),
		date TEXT NOT NULL,
		total_builds INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		failed_count INTEGER DEFAULT 0,
		avg_duration_ms INTEGER DEFAULT 0,
		UNIQUE(project_id, date)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_build_stats_project ON build_stats(project_id)`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		username TEXT,
		action TEXT NOT NULL,
		resource_type TEXT,
		resource_id TEXT,
		detail TEXT,
		ip TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at)`,
	`CREATE TABLE IF NOT EXISTS api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		token_hash TEXT UNIQUE NOT NULL,
		token_prefix TEXT NOT NULL,
		scopes TEXT DEFAULT '[]',
		expires_at DATETIME,
		last_used_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash)`,
	`CREATE TABLE IF NOT EXISTS build_approvals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id INTEGER NOT NULL REFERENCES builds(id),
		user_id INTEGER NOT NULL REFERENCES users(id),
		username TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		comment TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		resolved_at DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_build_approvals_build ON build_approvals(build_id)`,
	`CREATE INDEX IF NOT EXISTS idx_build_approvals_status ON build_approvals(status)`,
	`CREATE TABLE IF NOT EXISTS test_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id INTEGER NOT NULL REFERENCES builds(id),
		total INTEGER DEFAULT 0,
		passed INTEGER DEFAULT 0,
		failed INTEGER DEFAULT 0,
		skipped INTEGER DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		report_xml TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_test_results_build ON test_results(build_id)`,
	`CREATE TABLE IF NOT EXISTS project_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		color TEXT NOT NULL DEFAULT 'neutral',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS build_queue_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id INTEGER NOT NULL REFERENCES builds(id),
		project_id INTEGER NOT NULL,
		project_name TEXT,
		priority INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'queued',
		trigger TEXT,
		branch TEXT,
		queued_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_build_queue_status ON build_queue_items(status, priority DESC, queued_at)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_build_queue_build ON build_queue_items(build_id)`,
	`ALTER TABLE builds ADD COLUMN approval_required BOOLEAN DEFAULT FALSE`,
	`ALTER TABLE builds ADD COLUMN approved_by INTEGER`,
	`ALTER TABLE builds ADD COLUMN approved_at DATETIME`,
	`ALTER TABLE builds ADD COLUMN timeout_sec INTEGER DEFAULT 0`,
	`ALTER TABLE builds ADD COLUMN test_result_id INTEGER`,
	`ALTER TABLE projects ADD COLUMN group_id INTEGER REFERENCES project_groups(id)`,
	`ALTER TABLE project_groups ADD COLUMN updated_at DATETIME`,
	`ALTER TABLE project_groups ADD COLUMN color TEXT NOT NULL DEFAULT 'neutral'`,
	`ALTER TABLE build_approvals ADD COLUMN resolved_by INTEGER`,
	`ALTER TABLE build_approvals ADD COLUMN resolved_by_username TEXT DEFAULT ''`,
	`ALTER TABLE workers ADD COLUMN active_builds INTEGER DEFAULT 0`,
	`ALTER TABLE projects ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT 0`,
	`ALTER TABLE projects ADD COLUMN quick_access BOOLEAN NOT NULL DEFAULT 0`,
}

var schemaMigrations = []schemaMigration{
	{
		version: 1,
		name:    "reconcile-current-schema",
		up:      reconcileCurrentSchema,
	},
	{
		version:            2,
		name:               "project-groups-single-level",
		disableForeignKeys: true,
		up:                 migrateProjectGroupsToSingleLevel,
	},
	{
		version: 3,
		name:    "binary-plugins-only",
		up:      migratePluginsToBinaryOnly,
	},
	{
		version: 4,
		name:    "user-session-version",
		up:      addUserSessionVersion,
	},
	{
		version: 5,
		name:    "remove-inert-deployment-and-project-hooks",
		up:      removeInertDeploymentAndProjectHooks,
	},
	{
		version: 6,
		name:    "remove-dead-account-ssh-keys",
		up:      removeDeadAccountSSHKeys,
	},
	{
		version: 7,
		name:    "repair-orphan-project-history-references",
		up:      repairOrphanProjectHistoryReferences,
	},
	{
		version: 8,
		name:    "project-favorite-quick-access",
		up:      addProjectFavoriteQuickAccess,
	},
	{
		version: 9,
		name:    "project-pipeline-source",
		up:      addProjectPipelineSource,
	},
	{
		version: 10,
		name:    "project-enabled",
		up:      addProjectEnabled,
	},
	{
		version: 11,
		name:    "project-display-order",
		up:      addProjectDisplayOrder,
	},
	{
		version: 12,
		name:    "project-http-trigger",
		up:      addProjectHTTPTrigger,
	},
}

func runSchemaMigrations(db *sql.DB, migrations []schemaMigration) error {
	if err := validateMigrationDefinitions(migrations); err != nil {
		return err
	}
	if err := ensureMigrationLedger(db); err != nil {
		return err
	}

	applied, err := appliedMigrationCount(db, migrations)
	if err != nil {
		return err
	}
	for _, migration := range migrations[applied:] {
		if err := applySchemaMigration(db, migration); err != nil {
			return err
		}
	}
	return nil
}

func validateMigrationDefinitions(migrations []schemaMigration) error {
	for index, migration := range migrations {
		expectedVersion := index + 1
		if migration.version != expectedVersion {
			return fmt.Errorf("schema migration declaration %d has version %d; versions must be contiguous", index, migration.version)
		}
		if strings.TrimSpace(migration.name) == "" {
			return fmt.Errorf("schema migration %d has an empty name", migration.version)
		}
		if migration.up == nil {
			return fmt.Errorf("schema migration %d (%s) has no implementation", migration.version, migration.name)
		}
	}
	return nil
}

func ensureMigrationLedger(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration ledger setup: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return rollbackMigration(tx, fmt.Errorf("create schema migration ledger: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return rollbackMigration(tx, fmt.Errorf("commit schema migration ledger setup: %w", err))
	}
	return nil
}

func appliedMigrationCount(db *sql.DB, migrations []schemaMigration) (int, error) {
	rows, err := db.Query("SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		return 0, fmt.Errorf("read schema migration ledger: %w", err)
	}
	defer rows.Close()

	applied := 0
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return 0, fmt.Errorf("scan schema migration ledger: %w", err)
		}
		expectedVersion := applied + 1
		if version != expectedVersion {
			return 0, fmt.Errorf("schema migration ledger is not contiguous: expected version %d, found %d", expectedVersion, version)
		}
		if version > len(migrations) {
			return 0, fmt.Errorf("database schema version %d is newer than this BuildWorld binary supports (%d)", version, len(migrations))
		}
		if name != migrations[version-1].name {
			return 0, fmt.Errorf("schema migration %d name mismatch: database has %q, binary expects %q", version, name, migrations[version-1].name)
		}
		applied++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate schema migration ledger: %w", err)
	}
	return applied, nil
}

func applySchemaMigration(db *sql.DB, migration schemaMigration) (returnErr error) {
	if migration.disableForeignKeys {
		var foreignKeysEnabled int
		if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
			return fmt.Errorf("inspect foreign-key mode for schema migration %d (%s): %w", migration.version, migration.name, err)
		}
		if foreignKeysEnabled != 0 {
			if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
				return fmt.Errorf("disable foreign keys for schema migration %d (%s): %w", migration.version, migration.name, err)
			}
			defer func() {
				if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
					restoreErr := fmt.Errorf("restore foreign keys after schema migration %d (%s): %w", migration.version, migration.name, err)
					if returnErr == nil {
						returnErr = restoreErr
					} else {
						returnErr = errors.Join(returnErr, restoreErr)
					}
				}
			}()
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration %d (%s): %w", migration.version, migration.name, err)
	}
	if err := migration.up(tx); err != nil {
		return rollbackMigration(tx, fmt.Errorf("apply schema migration %d (%s): %w", migration.version, migration.name, err))
	}
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
		migration.version,
		migration.name,
	); err != nil {
		return rollbackMigration(tx, fmt.Errorf("record schema migration %d (%s): %w", migration.version, migration.name, err))
	}
	if err := tx.Commit(); err != nil {
		return rollbackMigration(tx, fmt.Errorf("commit schema migration %d (%s): %w", migration.version, migration.name, err))
	}
	return nil
}

func rollbackMigration(tx *sql.Tx, migrationErr error) error {
	if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		return errors.Join(migrationErr, fmt.Errorf("rollback schema migration: %w", rollbackErr))
	}
	return migrationErr
}

func reconcileCurrentSchema(tx *sql.Tx) error {
	for _, statement := range currentSchemaStatements {
		if err := applyCurrentSchemaStatement(tx, statement); err != nil {
			preview := strings.Join(strings.Fields(statement), " ")
			if len(preview) > 80 {
				preview = preview[:80]
			}
			return fmt.Errorf("schema statement %q: %w", preview, err)
		}
	}
	return nil
}

func applyCurrentSchemaStatement(tx *sql.Tx, statement string) error {
	fields := strings.Fields(statement)
	if len(fields) >= 6 &&
		strings.EqualFold(fields[0], "ALTER") &&
		strings.EqualFold(fields[1], "TABLE") &&
		strings.EqualFold(fields[3], "ADD") &&
		strings.EqualFold(fields[4], "COLUMN") {
		exists, err := sqliteColumnExists(tx, fields[2], fields[5])
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	_, err := tx.Exec(statement)
	return err
}

type sqliteQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func sqliteColumnExists(queryer sqliteQueryer, table, column string) (bool, error) {
	if !isSQLiteIdentifier(table) || !isSQLiteIdentifier(column) {
		return false, fmt.Errorf("invalid SQLite identifier %q.%q", table, column)
	}
	rows, err := queryer.Query(`PRAGMA table_info("` + table + `")`)
	if err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan table %s schema: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table %s schema: %w", table, err)
	}
	return false, nil
}

func isSQLiteIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			char == '_' ||
			(index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func addUserSessionVersion(tx *sql.Tx) error {
	exists, err := sqliteColumnExists(tx, "users", "session_version")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE users ADD COLUMN session_version INTEGER NOT NULL DEFAULT 1`); err != nil {
		return fmt.Errorf("add users.session_version: %w", err)
	}
	return nil
}

// addProjectFavoriteQuickAccess introduces the favorite / quick_access flags
// used by the dashboard sidebar and quick-access navigation. It runs as a
// discrete migration because reconcileCurrentSchema (v1) only executes once,
// so columns appended to currentSchemaStatements after the first boot are not
// picked up by existing databases. The existence guard keeps it safe to replay.
func addProjectFavoriteQuickAccess(tx *sql.Tx) error {
	for _, column := range []string{"favorite", "quick_access"} {
		exists, err := sqliteColumnExists(tx, "projects", column)
		if err != nil {
			return fmt.Errorf("inspect projects.%s: %w", column, err)
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN ` + column + ` BOOLEAN NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add projects.%s: %w", column, err)
		}
	}
	return nil
}

// addProjectPipelineSource introduces the pipeline authoring fields: the
// source syntax (typescript / yaml / jenkinsfile) and the SCM-sourced pipeline
// definition (repository, branch, path). It mirrors Jenkins' "Pipeline script
// from SCM" so the same configuration can live in the repository rather than
// the BuildWorld database. The existence guard keeps it safe to replay.
func addProjectPipelineSource(tx *sql.Tx) error {
	columns := []struct {
		name string
		ddl  string
	}{
		{"pipeline_format", `TEXT NOT NULL DEFAULT 'yaml'`},
		{"pipeline_source_mode", `TEXT NOT NULL DEFAULT 'inline'`},
		{"pipeline_scm_repo", `TEXT NOT NULL DEFAULT ''`},
		{"pipeline_scm_branch", `TEXT NOT NULL DEFAULT ''`},
		{"pipeline_scm_path", `TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		exists, err := sqliteColumnExists(tx, "projects", column.name)
		if err != nil {
			return fmt.Errorf("inspect projects.%s: %w", column.name, err)
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN ` + column.name + ` ` + column.ddl); err != nil {
			return fmt.Errorf("add projects.%s: %w", column.name, err)
		}
	}
	return nil
}

func addProjectEnabled(tx *sql.Tx) error {
	exists, err := sqliteColumnExists(tx, "projects", "enabled")
	if err != nil {
		return fmt.Errorf("inspect projects.enabled: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE`); err != nil {
		return fmt.Errorf("add projects.enabled: %w", err)
	}
	return nil
}

// addProjectHTTPTrigger adds an opt-in, per-project external trigger. Its
// opaque URL token is a credential and intentionally never serializes in API
// responses; handlers expose a complete URL only to authenticated editors.
func addProjectHTTPTrigger(tx *sql.Tx) error {
	for _, column := range []struct {
		name string
		ddl  string
	}{
		{"http_trigger_enabled", "BOOLEAN NOT NULL DEFAULT FALSE"},
		{"http_trigger_token", "TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := sqliteColumnExists(tx, "projects", column.name)
		if err != nil {
			return fmt.Errorf("inspect projects.%s: %w", column.name, err)
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN ` + column.name + ` ` + column.ddl); err != nil {
			return fmt.Errorf("add projects.%s: %w", column.name, err)
		}
	}
	return nil
}

func addProjectDisplayOrder(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS project_display_order (
		project_id INTEGER PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
		position INTEGER NOT NULL CHECK(position >= 0)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO project_display_order (project_id, position)
		SELECT project.id, (
			SELECT COUNT(*)
			FROM projects candidate
			WHERE candidate.favorite > project.favorite
				OR (candidate.favorite = project.favorite AND candidate.quick_access > project.quick_access)
				OR (candidate.favorite = project.favorite AND candidate.quick_access = project.quick_access AND candidate.id > project.id)
		)
		FROM projects project`); err != nil {
		return err
	}
	_, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_project_display_order_position
		ON project_display_order(position)`)
	return err
}

func removeInertDeploymentAndProjectHooks(tx *sql.Tx) error {
	statements := []string{
		`DROP INDEX IF EXISTS idx_deployment_envs_project`,
		`DROP TABLE IF EXISTS deployment_envs`,
		`DROP INDEX IF EXISTS idx_git_hooks_project`,
		`DROP TABLE IF EXISTS git_hooks`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("remove inert deployment/project-hook schema: %w", err)
		}
	}
	return nil
}

// removeDeadAccountSSHKeys drops the unused per-user key registry. Repository
// authentication remains in credentials, including its ssh_key type and
// private/public key fields.
func removeDeadAccountSSHKeys(tx *sql.Tx) error {
	if _, err := tx.Exec(`DROP TABLE IF EXISTS ssh_keys`); err != nil {
		return fmt.Errorf("remove dead account SSH-key schema: %w", err)
	}
	return nil
}

// repairOrphanProjectHistoryReferences repairs rows left by older project and
// build deletion paths. Queue entries and aggregate statistics have no useful
// meaning without their owners, while notification events remain valuable
// history and therefore retain their payload with a NULL build reference.
func repairOrphanProjectHistoryReferences(tx *sql.Tx) error {
	statements := []struct {
		name  string
		query string
	}{
		{
			name: "build queue items",
			query: `DELETE FROM build_queue_items
				WHERE NOT EXISTS (SELECT 1 FROM builds WHERE builds.id=build_queue_items.build_id)
				   OR NOT EXISTS (SELECT 1 FROM projects WHERE projects.id=build_queue_items.project_id)`,
		},
		{
			name:  "build statistics",
			query: `DELETE FROM build_stats WHERE NOT EXISTS (SELECT 1 FROM projects WHERE projects.id=build_stats.project_id)`,
		},
		{
			name: "notification event build references",
			query: `UPDATE notification_events SET build_id=NULL
				WHERE build_id IS NOT NULL
				  AND NOT EXISTS (SELECT 1 FROM builds WHERE builds.id=notification_events.build_id)`,
		},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement.query); err != nil {
			return fmt.Errorf("repair orphan %s: %w", statement.name, err)
		}
	}
	return nil
}

// migrateProjectGroupsToSingleLevel removes the development-only hierarchy
// column. Rebuilding inside the migration transaction preserves group IDs and
// therefore every projects.group_id association.
func migrateProjectGroupsToSingleLevel(tx *sql.Tx) error {
	hasParentID, err := sqliteColumnExists(tx, "project_groups", "parent_id")
	if err != nil {
		return err
	}
	if !hasParentID {
		return nil
	}
	if _, err := tx.Exec("PRAGMA defer_foreign_keys=ON"); err != nil {
		return fmt.Errorf("defer project group foreign keys: %w", err)
	}

	statements := []string{
		`DROP TABLE IF EXISTS project_groups_single_level_migration`,
		`CREATE TABLE project_groups_single_level_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			color TEXT NOT NULL DEFAULT 'neutral',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO project_groups_single_level_migration (id, name, description, color, created_at, updated_at)
			SELECT id, name, description, color, created_at, updated_at FROM project_groups`,
		`DROP TABLE project_groups`,
		`ALTER TABLE project_groups_single_level_migration RENAME TO project_groups`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("rebuild project_groups without parent_id: %w", err)
		}
	}
	return nil
}

// migratePluginsToBinaryOnly removes all executable script/runtime columns.
// Rows known to represent Go binary plugins are copied with stable IDs and
// activation state; legacy JavaScript/TypeScript plugin records are discarded.
func migratePluginsToBinaryOnly(tx *sql.Tx) error {
	columns := make(map[string]bool)
	for _, column := range []string{
		"id", "name", "version", "description", "author", "enabled", "config", "path", "source", "installed_at", "updated_at",
		"script_lang", "source_script", "source_ui_script", "steps", "triggers", "ui_extensions",
	} {
		exists, err := sqliteColumnExists(tx, "plugins", column)
		if err != nil {
			return err
		}
		columns[column] = exists
	}
	hasLegacyColumns := columns["script_lang"] || columns["source_script"] || columns["source_ui_script"] || columns["steps"] || columns["triggers"] || columns["ui_extensions"]
	hasFinalColumns := true
	for _, column := range []string{"id", "name", "version", "description", "author", "enabled", "config", "path", "source", "installed_at", "updated_at"} {
		hasFinalColumns = hasFinalColumns && columns[column]
	}
	if !hasLegacyColumns && hasFinalColumns {
		return nil
	}

	valueOr := func(column, fallback string) string {
		if columns[column] {
			return column
		}
		return fallback
	}
	binaryFilter := "0"
	if columns["script_lang"] {
		binaryFilter += " OR LOWER(COALESCE(script_lang, '')) = 'go'"
	}
	installedAt := "CURRENT_TIMESTAMP"
	if columns["installed_at"] {
		installedAt = "COALESCE(installed_at, CURRENT_TIMESTAMP)"
	}
	updatedAt := installedAt
	if columns["updated_at"] {
		updatedAt = "COALESCE(updated_at, " + installedAt + ")"
	}
	enabled := "1"
	if columns["enabled"] {
		enabled = "COALESCE(enabled, 1)"
	}
	copyBinaryRows := fmt.Sprintf(`INSERT INTO plugins_binary_v1_migration
			(id, name, version, description, author, enabled, config, path, source, installed_at, updated_at)
			SELECT id, name, version, %s, %s, %s, %s, %s, %s, %s, %s
			FROM plugins WHERE %s`,
		valueOr("description", "''"),
		valueOr("author", "''"),
		enabled,
		valueOr("config", "''"),
		valueOr("path", "''"),
		valueOr("source", "''"),
		installedAt,
		updatedAt,
		binaryFilter,
	)

	statements := []string{
		`DROP TABLE IF EXISTS plugins_binary_v1_migration`,
		`CREATE TABLE plugins_binary_v1_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			version TEXT NOT NULL,
			description TEXT,
			author TEXT DEFAULT '',
			enabled BOOLEAN DEFAULT TRUE,
			config TEXT,
			path TEXT DEFAULT '',
			source TEXT DEFAULT '',
			installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		copyBinaryRows,
		`DROP TABLE plugins`,
		`ALTER TABLE plugins_binary_v1_migration RENAME TO plugins`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("rebuild binary-only plugins table: %w", err)
		}
	}
	return nil
}
