package lessons

import "testing"

// realisticRegistry mirrors the actual feedback_lessons_learned.md format:
// "## Lesson N: <title>", a markdown body with **Do instead:**/**Why:**/
// **How to apply:** sections and fenced code, separated by "---".
const realisticRegistry = "## Lesson 1: SE-0478 typealias does NOT work\n" +
	"\n" +
	"The compiler does NOT support it.\n" +
	"\n" +
	"**Do instead:** Use `nonisolated` directly.\n" +
	"\n" +
	"**Why:** flag not available yet.\n" +
	"\n" +
	"---\n" +
	"\n" +
	"## Lesson 2: DTO properties must use camelCase\n" +
	"\n" +
	"SwiftLint rejects PascalCase.\n" +
	"\n" +
	"**Do instead:**\n" +
	"```swift\n" +
	"struct DTO: Decodable {\n" +
	"  let idToken: String\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"---\n"

func TestParse_CountsLessons(t *testing.T) {
	ls, err := Parse(realisticRegistry)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(ls) != 2 {
		t.Fatalf("expected 2 lessons, got %d", len(ls))
	}
}

func TestParse_ExtractsNumberAndTitle(t *testing.T) {
	ls, _ := Parse(realisticRegistry)
	if ls[0].Number != 1 {
		t.Fatalf("lesson[0].Number = %d, want 1", ls[0].Number)
	}
	if ls[0].Title != "SE-0478 typealias does NOT work" {
		t.Fatalf("lesson[0].Title = %q", ls[0].Title)
	}
	if ls[1].Number != 2 {
		t.Fatalf("lesson[1].Number = %d, want 2", ls[1].Number)
	}
}

func TestParse_BodyExcludesHeading(t *testing.T) {
	ls, _ := Parse(realisticRegistry)
	// The body should carry the lesson content but not the "## Lesson N:" line.
	if got := ls[0].Body; len(got) == 0 {
		t.Fatal("lesson body is empty")
	}
	if containsLine(ls[0].Body, "## Lesson 1:") {
		t.Fatalf("body should not contain the heading line:\n%s", ls[0].Body)
	}
	if !contains(ls[0].Body, "Do instead") {
		t.Fatalf("body lost its content:\n%s", ls[0].Body)
	}
}

func TestParse_DoesNotSplitOnFencedCodeContent(t *testing.T) {
	// A "## Lesson" or "---" INSIDE a fenced code block must not create a phantom
	// lesson. This is the dangerous edge case.
	reg := "## Lesson 1: real lesson\n" +
		"\n" +
		"```markdown\n" +
		"## Lesson 99: this is example text, not a real lesson\n" +
		"---\n" +
		"```\n" +
		"\n" +
		"**Why:** because.\n" +
		"\n" +
		"---\n"
	ls, err := Parse(reg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(ls) != 1 {
		t.Fatalf("fenced code created a phantom lesson: got %d, want 1", len(ls))
	}
	if ls[0].Number != 1 {
		t.Fatalf("wrong lesson number: %d", ls[0].Number)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	ls, err := Parse("")
	if err != nil {
		t.Fatalf("empty input should not error, got: %v", err)
	}
	if len(ls) != 0 {
		t.Fatalf("expected 0 lessons, got %d", len(ls))
	}
}

func TestParse_IgnoresPreamble(t *testing.T) {
	// Content before the first "## Lesson" (a file header) must be ignored.
	reg := "# Lessons Learned\n\nSome intro text.\n\n## Lesson 1: first\n\nbody\n\n---\n"
	ls, _ := Parse(reg)
	if len(ls) != 1 || ls[0].Number != 1 {
		t.Fatalf("preamble handling failed: %+v", ls)
	}
}

// --- tiny local helpers (avoid importing strings in the test for clarity) ---

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func containsLine(s, line string) bool {
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			if s[start:i] == line {
				return true
			}
			start = i + 1
		}
	}
	return false
}
