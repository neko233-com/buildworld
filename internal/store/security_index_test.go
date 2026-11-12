package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityAuditQueriesUseBoundedIndexes(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "security.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, tc := range []struct{ query, want string }{
		{`EXPLAIN QUERY PLAN SELECT resource_id FROM audit_logs WHERE action = 'build-idempotency' AND resource_type = 'build-request' AND username = 'key'`, "idx_audit_request_lookup"},
		{`EXPLAIN QUERY PLAN SELECT COUNT(*) FROM audit_logs WHERE action = 'login-attempt' AND resource_type = 'login-throttle' AND ip = 'source' AND created_at >= datetime('now', '-5 minutes')`, "idx_audit_login_source"},
		{`EXPLAIN QUERY PLAN SELECT 1 FROM audit_logs WHERE user_id = 1 AND action = 'totp-consumed' AND resource_type = 'login' AND resource_id = 'counter'`, "idx_audit_totp_consumed"},
	} {
		rows, err := database.DB().Query(tc.query)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plan.String(), tc.want) {
			t.Fatalf("query plan %q does not use %s", plan.String(), tc.want)
		}
	}
}
