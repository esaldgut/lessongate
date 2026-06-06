// Package pipeline wires the six stages into the end-to-end flow:
// WATCH → PREFILTER → (per lesson) REDACT → EXTRACT → GATE → RECONCILE → EMIT.
//
// Every dependency is an interface so the orchestrator is fully testable with
// fakes and so the order/decisions (skip, reject, dedup, dry-run, cap) are
// verified without any network call. The cmd layer constructs the real
// implementations and passes them in.
package pipeline

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared"

	"lessongate/internal/config"
	"lessongate/internal/extract"
	"lessongate/internal/gate"
	"lessongate/internal/prefilter"
	"lessongate/internal/redact"
	"lessongate/internal/reconcile"
	"lessongate/internal/watch"
)

// LessonSource reads the curated lesson bodies associated with a merged PR.
type LessonSource interface {
	ForPR(ctx context.Context, pr int) ([]string, error)
}

// Deps is the set of collaborators the pipeline orchestrates.
type Deps struct {
	Watcher    watch.Watcher
	Lessons    LessonSource
	Redactor   redact.Redactor
	Extractor  extractor
	Gate       gate.Deterministic
	Verifier   gate.Verifier
	Reconciler reconcile.Reconciler
	Emitter    emitter
}

// extractor / emitter are the minimal method sets the pipeline needs, declared
// locally so the real extract.Extractor / emit.Emitter satisfy them structurally
// and tests can supply fakes.
type extractor interface {
	Extract(ctx context.Context, redactedLesson string) (extract.Candidate, error)
}

type emitter interface {
	EmitDraft(ctx context.Context, id config.LessonID, skillMD, body, fingerprint string) (string, error)
}

// Options tune one run.
type Options struct {
	MaxPRs int
	Since  time.Time
	DryRun bool
	// Once processes the lesson source exactly once under a synthetic PR identity,
	// independent of the watch/prefilter. Deterministic and cheap — for smoke
	// tests and for re-processing a fixed lesson set without a live watch target.
	Once bool
	// Report, if set, accumulates a sanitized audit of every rejection (threat
	// N4) so a candidates:0 run is never silent about why.
	Report *Report
}

// Report is the sanitized audit of a run: counts and per-lesson rejection
// reasons (kinds only, never payloads).
type Report struct {
	ExtractErrors int
	Rejections    []Rejection
}

// Rejection records why one lesson did not result in a draft PR.
type Rejection struct {
	Lesson string // the lesson ID string
	Reason string // a kind: "extract-error" | "non-generalizable" | "gate-blocked" | "duplicate" | "emit-error"
	Detail string // short, already-sanitized (e.g. an API error class)
}

func (r *Report) add(id config.LessonID, reason Reason, detail string) {
	if r == nil {
		return
	}
	if reason == ReasonExtractError {
		r.ExtractErrors++
	}
	r.Rejections = append(r.Rejections, Rejection{Lesson: id.String(), Reason: reason, Detail: detail})
}

// Run executes the pipeline and returns the number of generalizable candidates
// found (in dry-run, candidates that WOULD be emitted; otherwise candidates
// actually emitted). Errors from a single lesson are skipped, not fatal, so one
// bad lesson can't halt the whole run.
func Run(ctx context.Context, d Deps, opt Options) (int, error) {
	candidates := 0

	// Once mode: process the lesson source a single time under a synthetic PR
	// identity, bypassing watch + prefilter. Deterministic smoke path.
	if opt.Once {
		bodies, err := d.Lessons.ForPR(ctx, 0)
		if err != nil {
			return 0, err
		}
		for i, body := range bodies {
			id := config.LessonID{PR: 0, Index: i + 1, SHA: "smoke"}
			if d.processLesson(ctx, id, body, opt.DryRun, opt.Report) {
				candidates++
			}
		}
		return candidates, nil
	}

	prs, err := d.Watcher.MergedSince(ctx, opt.Since, opt.MaxPRs)
	if err != nil {
		return 0, err
	}
	for _, pr := range prs {
		if skip, _ := prefilter.Skip(prefilter.PR{Number: pr.Number, Title: pr.Title, ChangedFiles: pr.Files}); skip {
			continue
		}
		bodies, err := d.Lessons.ForPR(ctx, pr.Number)
		if err != nil {
			continue // one PR's lesson read failing shouldn't kill the run
		}
		for i, body := range bodies {
			id := config.LessonID{PR: pr.Number, Index: i + 1, SHA: pr.MergeSHA}
			if d.processLesson(ctx, id, body, opt.DryRun, opt.Report) {
				candidates++
			}
		}
	}
	return candidates, nil
}

// processLesson runs one lesson through redact → extract → gate → reconcile →
// emit. Returns true if it counted as a generalizable candidate. A non-emitting
// outcome (non-generalizable, gate-blocked, duplicate, dry-run, emit error) is
// handled by returning the right boolean, never by panicking.
func (d Deps) processLesson(ctx context.Context, id config.LessonID, body string, dryRun bool, rep *Report) bool {
	// REDACT (deterministic, before any API call).
	redacted := d.Redactor.Redact(body).Clean

	// EXTRACT — is it a generalizable pattern?
	cand, err := d.Extractor.Extract(ctx, redacted)
	if err != nil {
		// An API/infra error — surface it, never swallow (the credit-balance bug).
		rep.add(id, ReasonExtractError, classifyErr(err))
		return false
	}
	if !cand.Generalizable {
		rep.add(id, ReasonNonGeneralizable, "")
		return false
	}

	// GATE — deterministic control first, then optional Claude verify.
	if v := d.Gate.Check(cand.Body); !v.Safe {
		rep.add(id, ReasonGateBlocked, strings.Join(v.Reasons, ","))
		return false
	}
	if d.Verifier != nil {
		if v := d.Verifier.Verify(cand.Body); !v.Safe {
			rep.add(id, ReasonGateBlocked, "claude-verify")
			return false
		}
	}

	// RECONCILE — novel/overlaps create or edit; duplicate drops.
	if dec, _ := d.Reconciler.Classify(cand.SkillName, cand.Summary); dec == reconcile.Duplicate {
		rep.add(id, ReasonDuplicate, cand.SkillName)
		return false
	}

	if dryRun {
		return true // counted as a candidate, but no PR opened
	}

	// EMIT — idempotent draft PR (the Emitter handles dedup + body gating).
	fp := cand.SkillName + ":" + redacted
	if _, err := d.Emitter.EmitDraft(ctx, id, cand.Body, cand.Summary, fp); err != nil {
		rep.add(id, ReasonEmitError, classifyErr(err))
		return false
	}
	return true
}

// apiError is the typed view classifyErr needs from an upstream API error: an
// HTTP status code and an error-type string. The SDK's *anthropic.Error
// satisfies it (StatusCode is a field, Type() a method) — see the adapter in
// classifyErr. This avoids string-matching the error MESSAGE (the canonical
// anti-pattern: a message format change would silently misclassify).
type apiError interface {
	Status() int
	Type() shared.ErrorType
}

// classifyErr maps an upstream error to a sanitized ErrClass using the typed
// status code + error type, never the message text. Falls back to ErrClassUnknown
// for non-API errors (network, context cancellation, etc.).
func classifyErr(err error) ErrClass {
	if ae, ok := err.(apiError); ok {
		return classifyAPI(ae.Status(), string(ae.Type()))
	}
	// The SDK's concrete error type (StatusCode is a field, not a method, so it
	// doesn't satisfy apiError directly): adapt it.
	if se, ok := errors.AsType[*anthropic.Error](err); ok {
		return classifyAPI(se.StatusCode, string(se.Type()))
	}
	return ErrClassUnknown
}

func classifyAPI(status int, typ string) ErrClass {
	switch {
	case typ == "billing_error":
		return ErrClassBilling
	case status == 401:
		return ErrClassAuth
	case status == 429:
		return ErrClassRateLimited
	case status >= 500:
		return ErrClassServer
	case status == 400:
		return ErrClassBadRequest
	default:
		return ErrClassUnknown
	}
}
