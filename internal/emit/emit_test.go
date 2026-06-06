package emit

import (
	"context"
	"strings"
	"testing"

	"lessongate/internal/config"
	"lessongate/internal/gate"
)

func TestFingerprint_StableAndContentSensitive(t *testing.T) {
	id := config.LessonID{PR: 40, Index: 1}
	a := Fingerprint(id, "hashA")
	b := Fingerprint(id, "hashA")
	c := Fingerprint(id, "hashB")
	if a != b {
		t.Fatal("fingerprint must be stable for same id+content")
	}
	if a == c {
		t.Fatal("fingerprint must change when content changes")
	}
}

func TestBodyMarker_RoundTrips(t *testing.T) {
	fp := "deadbeef"
	marker := BodyMarker(fp)
	if !strings.Contains(marker, fp) {
		t.Fatalf("marker must embed the fingerprint: %q", marker)
	}
}

// fakeForge records draft-PR creates and can pretend a fingerprint already exists.
type fakeForge struct {
	existing  map[string]string // fingerprint → existing PR URL
	created   []createdPR
	committed []committedFile
}

type createdPR struct {
	branch, title, body string
}

type committedFile struct {
	branch, path, content string
}

func (f *fakeForge) FindByFingerprint(_ context.Context, fp string) (string, bool, error) {
	url, ok := f.existing[fp]
	return url, ok, nil
}

func (f *fakeForge) CommitFile(_ context.Context, branch, path, content, _ string) error {
	f.committed = append(f.committed, committedFile{branch, path, content})
	return nil
}

func (f *fakeForge) CreateDraft(_ context.Context, branch, title, body string) (string, error) {
	f.created = append(f.created, createdPR{branch, title, body})
	return "https://example/pr/new", nil
}

// passGate is a gate.Deterministic that approves everything (so emit-gating is
// tested in isolation from the gate's own logic).
type passGate struct{}

func (passGate) Check(string) gate.Verdict { return gate.Verdict{Safe: true} }

// blockGate rejects everything, to prove emit re-gates the PR body.
type blockGate struct{}

func (blockGate) Check(string) gate.Verdict {
	return gate.Verdict{Safe: false, Reasons: []string{"blocked"}}
}

func TestEmit_CreatesDraftWhenNovel(t *testing.T) {
	f := &fakeForge{existing: map[string]string{}}
	e := New(f, passGate{})
	id := config.LessonID{PR: 40, Index: 1}

	url, err := e.EmitDraft(context.Background(), id, "skill md", "PR body", "fp123")
	if err != nil {
		t.Fatalf("EmitDraft: %v", err)
	}
	if url == "" {
		t.Fatal("expected a PR URL")
	}
	if len(f.created) != 1 {
		t.Fatalf("expected one draft PR created, got %d", len(f.created))
	}
	if f.created[0].branch != id.Branch() {
		t.Fatalf("wrong branch: %q", f.created[0].branch)
	}
	if !strings.Contains(f.created[0].body, BodyMarker("fp123")) {
		t.Fatal("PR body must embed the fingerprint marker for idempotency")
	}
}

func TestEmit_CommitsSkillFileToBranchBeforePR(t *testing.T) {
	f := &fakeForge{existing: map[string]string{}}
	e := New(f, passGate{})
	id := config.LessonID{PR: 40, Index: 1}

	_, err := e.EmitDraft(context.Background(), id, "the skill md body", "PR body", "fp123")
	if err != nil {
		t.Fatalf("EmitDraft: %v", err)
	}
	if len(f.committed) != 1 {
		t.Fatalf("expected the SKILL.md committed to the branch, got %d commits", len(f.committed))
	}
	if f.committed[0].branch != id.Branch() {
		t.Fatalf("committed to wrong branch: %q", f.committed[0].branch)
	}
	if f.committed[0].content != "the skill md body" {
		t.Fatalf("committed wrong content: %q", f.committed[0].content)
	}
	if !strings.HasSuffix(f.committed[0].path, "SKILL.md") {
		t.Fatalf("committed path should end in SKILL.md, got %q", f.committed[0].path)
	}
}

func TestEmit_IdempotentSkipsWhenFingerprintExists(t *testing.T) {
	f := &fakeForge{existing: map[string]string{"fp123": "https://example/pr/7"}}
	e := New(f, passGate{})
	id := config.LessonID{PR: 40, Index: 1}

	url, err := e.EmitDraft(context.Background(), id, "skill md", "PR body", "fp123")
	if err != nil {
		t.Fatalf("EmitDraft: %v", err)
	}
	if url != "https://example/pr/7" {
		t.Fatalf("expected the existing PR URL, got %q", url)
	}
	if len(f.created) != 0 {
		t.Fatal("must NOT create a second PR when the fingerprint already exists")
	}
}

func TestEmit_RegatesPRBodyAndRefusesOnLeak(t *testing.T) {
	f := &fakeForge{existing: map[string]string{}}
	e := New(f, blockGate{}) // gate blocks the body
	id := config.LessonID{PR: 40, Index: 1}

	_, err := e.EmitDraft(context.Background(), id, "skill md", "leaky body", "fp123")
	if err == nil {
		t.Fatal("emit must refuse when the gate blocks the PR body")
	}
	if len(f.created) != 0 {
		t.Fatal("no PR may be created when the body fails the gate")
	}
}
