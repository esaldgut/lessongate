package lessons

import (
	"os"
	"testing"
)

// TestParse_RealRegistry validates the parser against the actual production
// lesson registry when present (it lives outside the module, in the iOS
// project's memory dir). Skipped in CI / on machines without it — never fails
// the build for its absence, only proves correctness where the file exists.
func TestParse_RealRegistry(t *testing.T) {
	path := os.Getenv("LESSONGATE_REAL_REGISTRY")
	if path == "" {
		t.Skip("set LESSONGATE_REAL_REGISTRY to the real feedback_lessons_learned.md to run")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read real registry: %v", err)
	}
	ls, err := Parse(string(b))
	if err != nil {
		t.Fatalf("Parse real registry: %v", err)
	}
	if len(ls) == 0 {
		t.Fatal("parsed zero lessons from a non-empty real registry")
	}
	// Numbers should be strictly increasing and start at 1.
	for i, l := range ls {
		if l.Title == "" {
			t.Errorf("lesson #%d has empty title", l.Number)
		}
		if l.Body == "" {
			t.Errorf("lesson #%d has empty body", l.Number)
		}
		if i > 0 && l.Number <= ls[i-1].Number {
			t.Errorf("non-increasing lesson number: %d after %d", l.Number, ls[i-1].Number)
		}
	}
	t.Logf("parsed %d lessons from real registry (first=%d last=%d)", len(ls), ls[0].Number, ls[len(ls)-1].Number)
}
