//go:build !unix

package config

// DisableCoreDumps is a no-op on non-unix platforms. lessongate's supported
// target is darwin/arm64; this stub keeps the build green elsewhere.
func DisableCoreDumps() error { return nil }
