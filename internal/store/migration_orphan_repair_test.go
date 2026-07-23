package store

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectHistoryReferenceMigrationRepairsProductionOrphansWithForeignKeysEnabled(t *testing.T) {
	db := openMigrationTestDatabase(t, filepath.Join(t.TempDir(), "orphan-project-history.db"))
	defer db.Close()
	createProjectHistoryRepairFixtureTables(t, db)
	statements := []string{
		`INSERT INTO projects (id) VALUES (1)`,
		`INSERT INTO builds (id, project_id) VALUES (10, 1)`,
		`INSERT INTO build_queue_items (id, build_id, project_id) VALUES (100, 10, 1)`,
		`INSERT INTO build_queue_items (id, build_id, project_id) VALUES (101, 99, 1)`,
		`INSERT INTO build_queue_items (id, build_id, project_id) VALUES (102, 10, 99)`,
		`INSERT INTO build_stats (id, project_id) VALUES (200, 1)`,
		`INSERT INTO build_stats (id, project_id) VALUES (201, 99)`,
		`INSERT INTO notification_events (id, build_id, payload) VALUES (300, 10, 'valid history')`,
		`INSERT INTO notification_events (id, build_id, payload) VALUES (301, 99, 'orphan history')`,
		`INSERT INTO notification_events (id, build_id, payload) VALUES (302, NULL, 'existing detached history')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := ensureMigrationLedger(db); err != nil {
		t.Fatal(err)
	}
	for _, migration := range schemaMigrations[:6] {
		if _, err := db.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", migration.version, migration.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	if got := foreignKeyViolationCount(t, db); got < 3 {
		t.Fatalf("fixture foreign-key violations = %d, want at least 3", got)
	}

	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatal(err)
	}
	if got, want := readMigrationLedger(t, db), []string{
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
		"11:project-display-order",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("migration ledger = %v, want %v", got, want)
	}
	assertIntegerIDs(t, db, "SELECT id FROM build_queue_items ORDER BY id", []int64{100})
	assertIntegerIDs(t, db, "SELECT id FROM build_stats ORDER BY id", []int64{200})

	var validBuildID sql.NullInt64
	if err := db.QueryRow("SELECT build_id FROM notification_events WHERE id=300").Scan(&validBuildID); err != nil {
		t.Fatal(err)
	}
	if !validBuildID.Valid || validBuildID.Int64 != 10 {
		t.Fatalf("valid notification build_id = %v", validBuildID)
	}
	var orphanBuildID sql.NullInt64
	var orphanPayload string
	if err := db.QueryRow("SELECT build_id, payload FROM notification_events WHERE id=301").Scan(&orphanBuildID, &orphanPayload); err != nil {
		t.Fatal(err)
	}
	if orphanBuildID.Valid || orphanPayload != "orphan history" {
		t.Fatalf("orphan notification build=%v payload=%q", orphanBuildID, orphanPayload)
	}
	var notificationCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM notification_events").Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != 3 {
		t.Fatalf("notification count = %d, want 3", notificationCount)
	}
	if got := foreignKeyViolationCount(t, db); got != 0 {
		t.Fatalf("foreign-key violations after migration = %d", got)
	}
	var foreignKeysEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		t.Fatal(err)
	}
	if foreignKeysEnabled != 1 {
		t.Fatal("migration disabled foreign-key enforcement")
	}
	if err := runSchemaMigrations(db, schemaMigrations); err != nil {
		t.Fatalf("repeated migration run: %v", err)
	}
	if got := foreignKeyViolationCount(t, db); got != 0 {
		t.Fatalf("foreign-key violations after repeated run = %d", got)
	}
}

func createProjectHistoryRepairFixtureTables(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS projects (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS builds (
			id INTEGER PRIMARY KEY,
			project_id INTEGER NOT NULL REFERENCES projects(id)
		)`,
		`CREATE TABLE IF NOT EXISTS build_queue_items (
			id INTEGER PRIMARY KEY,
			build_id INTEGER NOT NULL REFERENCES builds(id),
			project_id INTEGER NOT NULL REFERENCES projects(id)
		)`,
		`CREATE TABLE IF NOT EXISTS build_stats (
			id INTEGER PRIMARY KEY,
			project_id INTEGER NOT NULL REFERENCES projects(id)
		)`,
		`CREATE TABLE IF NOT EXISTS notification_events (
			id INTEGER PRIMARY KEY,
			build_id INTEGER REFERENCES builds(id),
			payload TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func foreignKeyViolationCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertIntegerIDs(t *testing.T, db *sql.DB, query string, want []int64) {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}
