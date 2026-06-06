package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lessongate/internal/config"
)

// JSONL is an append-only, crash-resumable ledger backed by a JSON-lines file.
// The in-memory map holds the latest Entry per LessonID (last line wins on
// load). Each Put appends a line and fsyncs; a parse error on load fails closed
// (never silently resets the cursor). A flock guards single-run exclusivity.
type JSONL struct {
	path string
	mu   sync.Mutex
	mem  map[string]Entry

	lockFile *os.File // held while the run lock is acquired
}

// keyOf builds the map key for an entry's identity.
func keyOf(id config.LessonID) string {
	return fmt.Sprintf("%d/%d", id.PR, id.Index)
}

// Open loads (or creates) the ledger at path. It fails closed if any existing
// line is unparseable — a corrupt ledger must never read as "empty".
func Open(path string) (*JSONL, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("ledger: mkdir: %w", err)
	}
	l := &JSONL{path: path, mem: map[string]Entry{}}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil // fresh ledger
		}
		return nil, fmt.Errorf("ledger: open: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			// FAIL CLOSED — do not silently drop a torn line.
			return nil, fmt.Errorf("ledger: corrupt entry at line %d: %w", line, err)
		}
		l.mem[keyOf(e.ID)] = e
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ledger: scan: %w", err)
	}
	return l, nil
}

// Get returns the latest entry for an ID.
func (l *JSONL) Get(id config.LessonID) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.mem[keyOf(id)]
	return e, ok
}

// Put upserts an entry: updates the in-memory map and appends a durable line.
func (l *JSONL) Put(e Entry) error {
	if e.UpdatedAt.IsZero() {
		// Caller may set UpdatedAt explicitly (e.g. to a PR mergedAt); otherwise
		// stamp it now so the cursor advances. Time is acquired at the boundary,
		// not in pure logic, so it stays testable.
		e.UpdatedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("ledger: marshal: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("ledger: open for append: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("ledger: append: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("ledger: fsync: %w", err)
	}

	l.mem[keyOf(e.ID)] = e
	return nil
}

// Cursor returns the maximum UpdatedAt across all entries — the high-water mark
// for the watch query. Derived from entries, not stored separately.
func (l *JSONL) Cursor() (time.Time, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var max time.Time
	for _, e := range l.mem {
		if e.UpdatedAt.After(max) {
			max = e.UpdatedAt
		}
	}
	return max, nil
}

// Close releases the lock (if held). The append file is opened per-Put, so
// there is no long-lived data handle to flush here.
func (l *JSONL) Close() error {
	return l.Unlock()
}

// Compile-time check that JSONL satisfies the Ledger interface.
var _ Ledger = (*JSONL)(nil)
