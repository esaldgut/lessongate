package pipeline

import (
	"context"
	"testing"
	"time"

	"lessongate/internal/config"
	"lessongate/internal/extract"
	"lessongate/internal/gate"
	"lessongate/internal/redact"
	"lessongate/internal/reconcile"
	"lessongate/internal/watch"
)

// --- fakes ---

type fakeWatcher struct{ prs []watch.MergedPR }

func (f fakeWatcher) MergedSince(_ context.Context, _ time.Time, limit int) ([]watch.MergedPR, error) {
	if limit > 0 && len(f.prs) > limit {
		return f.prs[:limit], nil
	}
	return f.prs, nil
}

// fakeLessons returns a fixed set of lesson bodies for any PR.
type fakeLessons struct{ bodies []string }

func (f fakeLessons) ForPR(_ context.Context, _ int) ([]string, error) { return f.bodies, nil }

type fakeExtractor struct {
	result extract.Candidate
	err    error
}

func (f fakeExtractor) Extract(_ context.Context, _ string) (extract.Candidate, error) {
	return f.result, f.err
}

type fakeReconciler struct{ dec reconcile.Decision }

func (f fakeReconciler) Classify(_, _ string) (reconcile.Decision, reconcile.SkillRef) {
	return f.dec, reconcile.SkillRef{}
}

// fakeEmitter counts emits.
type fakeEmitter struct{ count int }

func (f *fakeEmitter) EmitDraft(_ context.Context, _ config.LessonID, _, _, _ string) (string, error) {
	f.count++
	return "https://example/pr/emitted", nil
}

// passGate / blockGate
type passGate struct{}

func (passGate) Check(string) gate.Verdict { return gate.Verdict{Safe: true} }

type blockGate struct{}

func (blockGate) Check(string) gate.Verdict { return gate.Verdict{Safe: false, Reasons: []string{"x"}} }

type passVerifier struct{}

func (passVerifier) Verify(string) gate.Verdict { return gate.Verdict{Safe: true} }

func basePR() watch.MergedPR {
	return watch.MergedPR{Number: 40, Title: "feat: add X", MergedAt: time.Now(), MergeSHA: "abc",
		Files: []string{"Core/X.swift"}}
}

func genericCandidate() extract.Candidate {
	return extract.Candidate{Generalizable: true, SkillName: "swift-x", Summary: "Do X.", Body: "Use X."}
}

func newDeps(emit *fakeEmitter, cand extract.Candidate, dec reconcile.Decision, g gate.Deterministic) Deps {
	return Deps{
		Watcher:    fakeWatcher{prs: []watch.MergedPR{basePR()}},
		Lessons:    fakeLessons{bodies: []string{"a redacted-safe lesson body"}},
		Redactor:   noopRedactor{},
		Extractor:  fakeExtractor{result: cand},
		Gate:       g,
		Verifier:   passVerifier{},
		Reconciler: fakeReconciler{dec: dec},
		Emitter:    emit,
	}
}

func TestRun_EmitsForNovelGeneralizableLesson(t *testing.T) {
	emit := &fakeEmitter{}
	deps := newDeps(emit, genericCandidate(), reconcile.Novel, passGate{})
	n, err := Run(context.Background(), deps, Options{MaxPRs: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if emit.count != 1 {
		t.Fatalf("expected 1 emit, got %d", emit.count)
	}
	if n != 1 {
		t.Fatalf("expected 1 emitted reported, got %d", n)
	}
}

func TestRun_SkipsNonGeneralizable(t *testing.T) {
	emit := &fakeEmitter{}
	deps := newDeps(emit, extract.Candidate{Generalizable: false}, reconcile.Novel, passGate{})
	_, _ = Run(context.Background(), deps, Options{MaxPRs: 10})
	if emit.count != 0 {
		t.Fatal("must not emit a non-generalizable lesson")
	}
}

func TestRun_DoesNotEmitWhenGateBlocks(t *testing.T) {
	emit := &fakeEmitter{}
	deps := newDeps(emit, genericCandidate(), reconcile.Novel, blockGate{})
	_, _ = Run(context.Background(), deps, Options{MaxPRs: 10})
	if emit.count != 0 {
		t.Fatal("must not emit when the deterministic gate blocks the candidate")
	}
}

func TestRun_DropsDuplicate(t *testing.T) {
	emit := &fakeEmitter{}
	deps := newDeps(emit, genericCandidate(), reconcile.Duplicate, passGate{})
	_, _ = Run(context.Background(), deps, Options{MaxPRs: 10})
	if emit.count != 0 {
		t.Fatal("a duplicate must be dropped, not emitted")
	}
}

func TestRun_DryRunNeverEmits(t *testing.T) {
	emit := &fakeEmitter{}
	deps := newDeps(emit, genericCandidate(), reconcile.Novel, passGate{})
	n, _ := Run(context.Background(), deps, Options{MaxPRs: 10, DryRun: true})
	if emit.count != 0 {
		t.Fatal("dry-run must not open any PR")
	}
	if n != 1 {
		t.Fatalf("dry-run should still report 1 candidate, got %d", n)
	}
}

func TestRun_ExtractErrorIsReportedNotSwallowed(t *testing.T) {
	// An extract API error (e.g. 400 credit balance) must surface as a Report,
	// not vanish into a silent candidates:0.
	emit := &fakeEmitter{}
	deps := newDeps(emit, extract.Candidate{}, reconcile.Novel, passGate{})
	deps.Extractor = fakeExtractor{err: errStub("400 credit balance too low")}
	deps.Lessons = fakeLessons{bodies: []string{"lesson A"}}

	rep := &Report{}
	_, err := Run(context.Background(), deps, Options{Once: true, Report: rep})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.ExtractErrors != 1 {
		t.Fatalf("expected 1 extract error recorded, got %d", rep.ExtractErrors)
	}
	if len(rep.Rejections) == 0 {
		t.Fatal("expected the rejection reason to be recorded")
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }

func TestRun_OnceProcessesLessonsWithoutWatch(t *testing.T) {
	// In Once mode the lessons are processed exactly once, independent of which
	// PRs the watcher returns (even none / all-skipped). Deterministic smoke.
	emit := &fakeEmitter{}
	deps := newDeps(emit, genericCandidate(), reconcile.Novel, passGate{})
	// Watcher returns a PR the prefilter would SKIP — Once must still process.
	deps.Watcher = fakeWatcher{prs: []watch.MergedPR{
		{Number: 99, Title: "chore(lessons): capture PR#1", MergedAt: time.Now()},
	}}
	deps.Lessons = fakeLessons{bodies: []string{"lesson A", "lesson B"}}

	n, err := Run(context.Background(), deps, Options{Once: true})
	if err != nil {
		t.Fatalf("Run once: %v", err)
	}
	if n != 2 {
		t.Fatalf("once mode should process both lessons (2 candidates), got %d", n)
	}
	if emit.count != 2 {
		t.Fatalf("once mode should emit both, got %d", emit.count)
	}
}

// noopRedactor passes text through unchanged (the gate is tested separately).
type noopRedactor struct{}

func (noopRedactor) Redact(text string) redact.Result { return redact.Result{Clean: text} }
