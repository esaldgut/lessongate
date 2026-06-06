package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"lessongate/internal/config"
)

func openTemp(t *testing.T) *JSONL {
	t.Helper()
	dir := t.TempDir()
	l, err := Open(filepath.Join(dir, "ledger.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestLedger_PutThenGet(t *testing.T) {
	l := openTemp(t)
	id := config.LessonID{PR: 40, Index: 1, SHA: "abc"}
	e := Entry{ID: id, Stage: config.StageEmitted, DraftPRURL: "https://x/pr/9", ModelID: "claude-opus-4-8"}
	if err := l.Put(e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := l.Get(id)
	if !ok {
		t.Fatal("entry not found after Put")
	}
	if got.Stage != config.StageEmitted || got.DraftPRURL != "https://x/pr/9" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestLedger_PutUpserts(t *testing.T) {
	l := openTemp(t)
	id := config.LessonID{PR: 40, Index: 1}
	_ = l.Put(Entry{ID: id, Stage: config.StageDiscovered})
	_ = l.Put(Entry{ID: id, Stage: config.StageDone})
	got, _ := l.Get(id)
	if got.Stage != config.StageDone {
		t.Fatalf("expected upsert to latest stage, got %v", got.Stage)
	}
}

func TestLedger_SurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	id := config.LessonID{PR: 41, Index: 2}

	l1, _ := Open(path)
	_ = l1.Put(Entry{ID: id, Stage: config.StageGated})
	_ = l1.Close()

	// Reopen — simulating a fresh run after a crash. State must persist.
	l2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer l2.Close()
	got, ok := l2.Get(id)
	if !ok || got.Stage != config.StageGated {
		t.Fatalf("state did not survive reopen: ok=%v got=%+v", ok, got)
	}
}

func TestLedger_FailsClosedOnCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	// A torn/garbage line must NOT silently parse as empty (which would trigger a
	// full backfill). Open must error.
	if err := os.WriteFile(path, []byte("{not valid json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open must fail closed on a corrupt ledger, not silently reset")
	}
}

func TestLedger_CursorIsMaxMergedAt(t *testing.T) {
	l := openTemp(t)
	t1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	_ = l.Put(Entry{ID: config.LessonID{PR: 1, Index: 1}, Stage: config.StageDone, UpdatedAt: t1})
	_ = l.Put(Entry{ID: config.LessonID{PR: 2, Index: 1}, Stage: config.StageDone, UpdatedAt: t2})
	cur, err := l.Cursor()
	if err != nil {
		t.Fatalf("Cursor: %v", err)
	}
	if !cur.Equal(t2) {
		t.Fatalf("cursor should be the max UpdatedAt (%v), got %v", t2, cur)
	}
}

func TestLedger_LockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")

	l1, _ := Open(path)
	defer l1.Close()
	if err := l1.Lock(); err != nil {
		t.Fatalf("first Lock should succeed: %v", err)
	}
	defer l1.Unlock()

	l2, _ := Open(path)
	defer l2.Close()
	if err := l2.Lock(); err == nil {
		t.Fatal("second concurrent Lock must fail (anti duplicate-PR)")
		_ = l2.Unlock()
	}
}

func TestLedger_AtomicWriteLeavesNoTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	l, _ := Open(path)
	defer l.Close()
	_ = l.Put(Entry{ID: config.LessonID{PR: 1, Index: 1}, Stage: config.StageDone})

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("atomic write left a .tmp file behind: %s", e.Name())
		}
	}
}
