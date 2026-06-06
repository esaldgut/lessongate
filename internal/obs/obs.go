// Package obs provides structured logging and counters for lessongate.
//
// The cardinal rule: NDA content never reaches a log sink. Loggers in this
// package only ever emit IDs, hashes, counts, durations, and stage outcomes —
// never raw lesson text, diffs, prompts, or tokens. A redacting handler is the
// last line of defense: even if a caller mistakenly passes a sensitive string,
// the handler scrubs known structural secret shapes before the record is written.
package obs

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"sync/atomic"
)

// structuralSecrets matches *classes* of private identifiers (not literals), so
// the redactor catches novel secrets the literal deny-list can't enumerate.
// Mirrors the gate's structural patterns (see internal/gate). Kept here so the
// logging layer is self-contained and can never be bypassed by import order.
var structuralSecrets = regexp.MustCompile(
	`(?i)` +
		`\b\d{12}\b` + // AWS account IDs (12 digits)
		`|arn:aws:[^\s"']+` + // ARNs
		`|AKIA[0-9A-Z]{16}` + // AWS access key IDs
		`|ghp_[0-9A-Za-z]{36}` + // GitHub PAT (classic)
		`|github_pat_[0-9A-Za-z_]{22,}` + // GitHub PAT (fine-grained)
		`|xox[baprs]-[0-9A-Za-z-]+` + // Slack tokens
		`|sk-ant-[0-9A-Za-z-]+`, // Anthropic API keys
)

const redacted = "[REDACTED]"

// redactingHandler wraps another slog.Handler and scrubs structural secrets
// from every string attribute value before delegating.
type redactingHandler struct {
	inner slog.Handler
}

func (h redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = structuralSecrets.ReplaceAllString(r.Message, redacted)
	scrubbed := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		scrubbed.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, scrubbed)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	for i := range attrs {
		attrs[i] = redactAttr(attrs[i])
	}
	return redactingHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		a.Value = slog.StringValue(structuralSecrets.ReplaceAllString(a.Value.String(), redacted))
	}
	return a
}

// NewLogger returns a slog.Logger whose every record passes through the
// redacting handler. Use this everywhere; never construct a raw slog logger.
func NewLogger(level slog.Level) *slog.Logger {
	base := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return slog.New(redactingHandler{inner: base})
}

// Counters holds the cheap atomic tallies the agent reports per run. No content,
// only magnitudes — safe to log and to surface in `status`.
type Counters struct {
	PRsScanned   atomic.Int64
	PRsPrefilter atomic.Int64 // dropped by prefilter (docs-only, dep-bump, chore(lessons))
	LessonsSeen  atomic.Int64
	Extracted    atomic.Int64
	GateRejected atomic.Int64
	Emitted      atomic.Int64
	Skipped      atomic.Int64 // idempotency: PR/fingerprint already present
}
