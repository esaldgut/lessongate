// Package ledger is the durable state of the pipeline. It is NOT a scalar
// high-water mark: it records, per LessonID, the stage reached, the draft PR URL
// (if any), a content hash (for idempotency + churn detection), and the model
// ID it was processed under. This lets a crash mid-run resume idempotently and
// lets emissions be re-audited after a model bump (threats C3, N2).
//
// Writes are atomic (tmp + fsync + rename); a parse error fails closed and never
// resets the cursor to zero. A flock guards against concurrent runs opening
// duplicate PRs.
//
// Built in Fase 2. This file declares the contract.
package ledger

import (
	"time"

	"lessongate/internal/config"
)

// Entry is the persisted record for one lesson.
type Entry struct {
	ID          config.LessonID `json:"id"`
	Stage       config.Stage    `json:"stage"`
	DraftPRURL  string          `json:"draft_pr_url,omitzero"`
	ContentHash string          `json:"content_hash,omitzero"`
	ModelID     string          `json:"model_id,omitzero"`
	UpdatedAt   time.Time       `json:"updated_at,omitzero"`
}

// Ledger is the append-and-resume store plus the single-run lock.
type Ledger interface {
	// Lock acquires the single-run flock; returns an error if another instance holds it.
	Lock() error
	Unlock() error

	// Get returns the entry for an ID and whether it exists.
	Get(id config.LessonID) (Entry, bool)
	// Put atomically upserts an entry.
	Put(e Entry) error

	// Cursor returns the merge timestamp high-water mark for the watch query,
	// derived from processed entries (not a separately-stored scalar).
	Cursor() (time.Time, error)
}
