package store

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoginAttemptReservationSurvivesRestartAndSerializesConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.db")
	database, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		handle := database
		if i%2 == 1 {
			handle = second
		}
		go func(handle *Store) {
			defer wg.Done()
			allowed, err := handle.ReserveLoginAttempt("Target", "192.0.2.1")
			if err != nil {
				t.Error(err)
			} else if allowed {
				accepted.Add(1)
			}
		}(handle)
	}
	wg.Wait()
	if got := accepted.Load(); got != loginAttemptAccountLimit {
		t.Fatalf("accepted %d attempts, want %d", got, loginAttemptAccountLimit)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if allowed, err := database.ReserveLoginAttempt("target", "198.51.100.2"); err != nil || allowed {
		t.Fatalf("restart or a different source bypassed account limit: allowed=%v err=%v", allowed, err)
	}
	if _, err := database.DB().Exec(`UPDATE audit_logs SET created_at = datetime('now', '-6 minutes') WHERE action = 'login-attempt'`); err != nil {
		t.Fatal(err)
	}
	if allowed, err := database.ReserveLoginAttempt("target", "198.51.100.2"); err != nil || !allowed {
		t.Fatalf("expired window did not recover: allowed=%v err=%v", allowed, err)
	}
}
