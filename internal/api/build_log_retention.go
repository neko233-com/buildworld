package api

import (
	"context"
	"log"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

// StartBuildLogRetentionCleanup runs the durable log cleanup without blocking
// server startup. It runs once immediately and then periodically so a long-
// lived server does not depend on a restart to enforce the retention policy.
func StartBuildLogRetentionCleanup(ctx context.Context, data *store.Store) {
	if data == nil {
		return
	}
	go func() {
		run := func() {
			days, err := data.GetBuildLogRetentionDays()
			if err != nil {
				log.Printf("Warning: failed to load build log retention: %v", err)
				return
			}
			purged, err := data.PurgeExpiredBuildLogs(days, time.Now())
			if err != nil {
				log.Printf("Warning: failed to purge expired build logs: %v", err)
				return
			}
			if purged > 0 {
				log.Printf("Purged %d expired build log(s) older than %d day(s)", purged, days)
			}
		}

		run()
		ticker := time.NewTicker(store.BuildLogRetentionSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
