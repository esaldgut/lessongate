//go:build unix

package ledger

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrLocked is returned when another instance already holds the run lock.
var ErrLocked = errors.New("ledger: another lessongate instance is running (lock held)")

// Lock acquires an exclusive, non-blocking flock on a sidecar lockfile. This is
// the anti-duplicate-PR control (threat C3): two concurrent runs cannot both
// process the same PRs and open competing draft PRs.
func (l *JSONL) Lock() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lockFile != nil {
		return nil // already held by this instance
	}
	f, err := os.OpenFile(l.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("ledger: open lockfile: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return ErrLocked
	}
	l.lockFile = f
	return nil
}

// Unlock releases the run lock if held.
func (l *JSONL) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lockFile == nil {
		return nil
	}
	err := syscall.Flock(int(l.lockFile.Fd()), syscall.LOCK_UN)
	cerr := l.lockFile.Close()
	l.lockFile = nil
	if err != nil {
		return fmt.Errorf("ledger: unlock: %w", err)
	}
	return cerr
}
