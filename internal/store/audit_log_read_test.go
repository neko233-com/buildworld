package store

import (
	"path/filepath"
	"testing"
)

func TestListAuditLogsHandlesEventsWithoutActorOrResource(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	empty, err := data.ListAuditLogs(50, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty audit log = %+v, %v", empty, err)
	}
	if err := data.CreateAuditLog(42, "operator", "update", "project", "7", "updated", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec("INSERT INTO audit_logs (action, username, resource_type) VALUES ('login-attempt', 'operator', 'authentication'), ('system-event', NULL, NULL)"); err != nil {
		t.Fatal(err)
	}
	logs, err := data.ListAuditLogs(50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 || logs[0].Action != "system-event" || logs[0].UserID != 0 || logs[0].Username != "" ||
		logs[1].Action != "login-attempt" || logs[1].Username != "operator" || logs[1].ResourceID != "" ||
		logs[2].UserID != 42 || logs[2].ResourceID != "7" || logs[2].CreatedAt.IsZero() {
		for _, log := range logs {
			t.Logf("%+v", *log)
		}
		t.Fatalf("unexpected audit log entries, count = %d", len(logs))
	}
}
