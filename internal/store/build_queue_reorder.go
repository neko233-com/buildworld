package store

import (
	"database/sql"
	"errors"
	"fmt"
)

var ErrBuildQueueItemNotMovable = errors.New("build queue item is not movable")

// QueuePositionResolver converts the current zero-based position and queue
// length into a target zero-based position. Keeping the ordering rule outside
// the store lets new queue operations be added without duplicating transaction
// and normalization logic.
type QueuePositionResolver func(current, total int) int

// ReorderQueuedBuildQueueItem atomically moves one waiting build and then
// normalizes all movable priorities. Running and approval-gated builds are
// intentionally excluded: they are visible queue states, but cannot be moved.
func (s *Store) ReorderQueuedBuildQueueItem(id int64, resolve QueuePositionResolver) (int, error) {
	if resolve == nil {
		return 0, fmt.Errorf("queue position resolver is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT q.id
		FROM build_queue_items q
		JOIN builds b ON b.id = q.build_id
		WHERE q.status = 'queued' AND b.status = 'pending'
		ORDER BY q.priority DESC, q.queued_at ASC, q.id ASC`)
	if err != nil {
		return 0, err
	}
	var ordered []int64
	for rows.Next() {
		var queueID int64
		if err := rows.Scan(&queueID); err != nil {
			rows.Close()
			return 0, err
		}
		ordered = append(ordered, queueID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	current := -1
	for index, queueID := range ordered {
		if queueID == id {
			current = index
			break
		}
	}
	if current < 0 {
		return 0, ErrBuildQueueItemNotMovable
	}

	target := resolve(current, len(ordered))
	if target < 0 {
		target = 0
	}
	if target >= len(ordered) {
		target = len(ordered) - 1
	}
	if target != current {
		item := ordered[current]
		ordered = append(ordered[:current], ordered[current+1:]...)
		ordered = append(ordered, 0)
		copy(ordered[target+1:], ordered[target:])
		ordered[target] = item
	}

	// Positive, gap-free priorities keep future default-priority builds at the
	// tail and make the persisted order deterministic even when timestamps tie.
	for index, queueID := range ordered {
		priority := int64(len(ordered) - index)
		result, err := tx.Exec("UPDATE build_queue_items SET priority = ? WHERE id = ?", priority, queueID)
		if err != nil {
			return 0, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if affected != 1 {
			return 0, sql.ErrNoRows
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return target + 1, nil
}
