package extract

import (
	"context"
	"encoding/json"
	"testing"

	"lessongate/internal/claude"
)

func decodeJSON(s string, out any) error { return json.Unmarshal([]byte(s), out) }

// buildCassette records the response the model should give for a specific
// redacted lesson, keyed the same way claude.fingerprint keys it. We reach the
// fingerprint indirectly: the Extractor builds (prefix, volatile) from the
// lesson, so we record against the same pair the Extractor will send.

func TestExtract_GeneralizableLesson(t *testing.T) {
	lesson := "Segregate sync.Once by use path so the happy path skips unused cold starts."
	// The cassette is keyed by the prompt the Extractor constructs. We capture
	// that by using a recording client that remembers the last request.
	rec := &recordingClient{
		response: `{"generalizable": true, "skill_name": "lambda-go-lazy-init", "summary": "Segregate sync.Once by use path.", "body": "Give each use path its own sync.Once."}`,
	}
	e := Extractor{C: rec}

	got, err := e.Extract(context.Background(), lesson)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !got.Generalizable {
		t.Fatal("expected generalizable=true")
	}
	if got.SkillName != "lambda-go-lazy-init" {
		t.Fatalf("skill name = %q", got.SkillName)
	}
	if got.Body == "" {
		t.Fatal("expected a non-empty body")
	}
}

func TestExtract_NonGeneralizableLesson(t *testing.T) {
	rec := &recordingClient{response: `{"generalizable": false, "skill_name": "", "summary": "", "body": ""}`}
	e := Extractor{C: rec}
	got, err := e.Extract(context.Background(), "Our internal ticket TICKET-123 was closed.")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got.Generalizable {
		t.Fatal("expected generalizable=false for business-specific lesson")
	}
}

func TestExtract_SendsLessonAsVolatileNotPrefix(t *testing.T) {
	// The redacted lesson text must travel in the volatile suffix (after the
	// cache breakpoint), not in the cached static prefix — otherwise every
	// lesson invalidates the cache.
	rec := &recordingClient{response: `{"generalizable": false}`}
	e := Extractor{C: rec}
	const lesson = "UNIQUE_LESSON_MARKER_42"
	_, _ = e.Extract(context.Background(), lesson)

	if want := lesson; !contains(rec.lastVolatile, want) {
		t.Fatalf("lesson text must be in the volatile suffix; got volatile=%q", rec.lastVolatile)
	}
	if contains(rec.lastPrefix, lesson) {
		t.Fatalf("lesson text leaked into the cached prefix: %q", rec.lastPrefix)
	}
}

// recordingClient is a claude.Client that returns a fixed response and records
// the last (prefix, volatile) it was called with.
type recordingClient struct {
	response     string
	lastPrefix   string
	lastVolatile string
}

func (r *recordingClient) Complete(_ context.Context, prefix, volatile string, out any) error {
	r.lastPrefix = prefix
	r.lastVolatile = volatile
	return decodeJSON(r.response, out)
}

// small helpers (avoid extra imports in the test)
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ claude.Client = (*recordingClient)(nil)
