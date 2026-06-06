//go:build unix

package config

import "syscall"

// DisableCoreDumps sets RLIMIT_CORE to 0 so that if the process crashes while a
// raw diff or unsanitized lesson is in memory, that content cannot be written
// to a core file on disk (a real NDA leak vector — see threat model C4).
//
// Best-effort: a failure here is logged by the caller, not fatal, since the
// in-memory-only handling of sensitive content is the primary control.
func DisableCoreDumps() error {
	return syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{Cur: 0, Max: 0})
}
