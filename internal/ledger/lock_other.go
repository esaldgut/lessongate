//go:build !unix

package ledger

// Lock is a no-op on non-unix platforms (lessongate targets darwin/arm64).
// Without flock the single-run guarantee is weaker; documented as such.
func (l *JSONL) Lock() error { return nil }

// Unlock is the no-op counterpart.
func (l *JSONL) Unlock() error { return nil }
