package engine

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/ws"
)

const (
	durableBuildLogBatchBytes    = 64 * 1024
	durableBuildLogFlushInterval = 250 * time.Millisecond
	durableBuildLogRetryMaxDelay = 5 * time.Second
	liveBuildLogLineMaxBytes     = 128 * 1024
)

type durableBuildLogBatch struct {
	buffer     strings.Builder
	lastFlush  time.Time
	timer      *time.Timer
	retryDelay time.Duration
	terminal   bool
}

func boundLiveBuildLogLine(line string) string {
	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "\uFFFD")
	}
	if len(line) <= liveBuildLogLineMaxBytes {
		return line
	}
	start := len(line) - liveBuildLogLineMaxBytes
	for start < len(line) && !utf8.RuneStart(line[start]) {
		start++
	}
	return store.BuildLogOversizedMarker + line[start:]
}

// appendDurableBuildLog keeps the live WebSocket path independent from
// SQLite. Fast output is coalesced into bounded writes; low-volume output is
// persisted immediately or within the short flush interval.
func (r *BuildRunner) appendDurableBuildLog(buildID int64, entry string) error {
	if r.store == nil && r.durableLogAppend == nil {
		return nil
	}
	now := time.Now()
	r.durableLogMu.Lock()

	batch := r.durableLogs[buildID]
	if batch == nil {
		batch = &durableBuildLogBatch{}
		r.durableLogs[buildID] = batch
	}
	_, _ = batch.buffer.WriteString(entry)
	if batch.retryDelay == 0 &&
		(batch.lastFlush.IsZero() ||
			batch.buffer.Len() >= durableBuildLogBatchBytes ||
			now.Sub(batch.lastFlush) >= durableBuildLogFlushInterval) {
		err := r.flushDurableBuildLogLocked(buildID, batch, false)
		r.durableLogMu.Unlock()
		r.reportDurableBuildLogError(buildID, err)
		return err
	}
	if batch.timer == nil {
		delay := durableBuildLogFlushInterval - now.Sub(batch.lastFlush)
		if delay < 0 {
			delay = 0
		}
		r.scheduleDurableBuildLogFlushLocked(buildID, batch, delay)
	}
	r.durableLogMu.Unlock()
	return nil
}

// flushDurableBuildLog serializes timer, size-triggered, and terminal flushes.
// Terminal callers remove the state only after its tail was handed to SQLite.
func (r *BuildRunner) flushDurableBuildLog(buildID int64, terminal bool) error {
	r.durableLogMu.Lock()
	batch := r.durableLogs[buildID]
	if batch == nil {
		r.durableLogMu.Unlock()
		return nil
	}
	err := r.flushDurableBuildLogLocked(buildID, batch, terminal)
	r.durableLogMu.Unlock()
	r.reportDurableBuildLogError(buildID, err)
	return err
}

func (r *BuildRunner) flushDurableBuildLogLocked(buildID int64, batch *durableBuildLogBatch, terminal bool) error {
	if batch.timer != nil {
		batch.timer.Stop()
		batch.timer = nil
	}
	batch.terminal = batch.terminal || terminal
	if batch.buffer.Len() > 0 {
		content := batch.buffer.String()
		if err := r.persistDurableBuildLog(buildID, content); err != nil {
			if batch.retryDelay == 0 {
				batch.retryDelay = durableBuildLogFlushInterval
			} else {
				batch.retryDelay *= 2
				if batch.retryDelay > durableBuildLogRetryMaxDelay {
					batch.retryDelay = durableBuildLogRetryMaxDelay
				}
			}
			r.scheduleDurableBuildLogFlushLocked(buildID, batch, batch.retryDelay)
			return fmt.Errorf("persist build log: %w; retained batch will retry in %s", err, batch.retryDelay)
		}
		// Reset only after SQLite accepted the complete batch. A transient
		// database error therefore cannot silently discard live output.
		batch.buffer.Reset()
		batch.lastFlush = time.Now()
		batch.retryDelay = 0
	}
	if batch.terminal {
		delete(r.durableLogs, buildID)
	}
	return nil
}

func (r *BuildRunner) scheduleDurableBuildLogFlushLocked(buildID int64, batch *durableBuildLogBatch, delay time.Duration) {
	batch.timer = time.AfterFunc(delay, func() {
		_ = r.flushDurableBuildLog(buildID, false)
	})
}

func (r *BuildRunner) persistDurableBuildLog(buildID int64, content string) error {
	if r.durableLogAppend != nil {
		return r.durableLogAppend(buildID, content)
	}
	return r.store.AppendBuildLog(buildID, content)
}

func (r *BuildRunner) reportDurableBuildLogError(buildID int64, err error) {
	if err == nil {
		return
	}
	safeError := r.maskBuildText(buildID, err.Error())
	if r.durableLogErrorReporter != nil {
		r.durableLogErrorReporter(buildID, err)
	} else {
		log.Printf("build %d durable log persistence failed: %s", buildID, safeError)
	}
	if r.hub != nil || r.liveLogBroadcast != nil {
		ts := time.Now().Format("15:04:05")
		payload := map[string]interface{}{
			"timestamp": ts,
			"stage":     r.maskBuildText(buildID, "buildworld"),
			"line":      "Durable log persistence failed; retained output will retry: " + safeError,
			"level":     "error",
		}
		if r.liveLogBroadcast != nil {
			r.liveLogBroadcast(buildID, payload)
		} else {
			ws.BroadcastBuildLog(r.hub, fmt.Sprintf("%d", buildID), payload)
		}
	}
}
