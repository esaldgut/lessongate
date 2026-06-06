package prefilter

import "testing"

func TestSkip_ChoreLessonsCommit(t *testing.T) {
	// The pipeline's own capture commits must never be reprocessed (no recursion).
	pr := PR{Number: 47, Title: "chore(lessons): capture PR#40", ChangedFiles: []string{"memory/feedback_lessons_learned.md"}}
	skip, reason := Skip(pr)
	if !skip {
		t.Fatal("chore(lessons) commit must be skipped")
	}
	if reason == "" {
		t.Fatal("skip reason must be set")
	}
}

func TestSkip_DependencyBump(t *testing.T) {
	cases := []string{
		"chore(deps): bump go-github to v88",
		"build(deps): bump anthropic-sdk-go from 1.45 to 1.46",
		"chore: update dependencies",
	}
	for _, title := range cases {
		pr := PR{Number: 1, Title: title, ChangedFiles: []string{"go.mod", "go.sum"}}
		if skip, _ := Skip(pr); !skip {
			t.Errorf("dependency bump should be skipped: %q", title)
		}
	}
}

func TestSkip_DocsOnly(t *testing.T) {
	pr := PR{Number: 2, Title: "docs: clarify README", ChangedFiles: []string{"README.md", "docs/02-arch.md"}}
	if skip, _ := Skip(pr); !skip {
		t.Fatal("docs-only PR should be skipped")
	}
}

func TestSkip_RealFeaturePRIsNotSkipped(t *testing.T) {
	pr := PR{
		Number:       38,
		Title:        "feat(auth): add Cognito federation with PKCE",
		ChangedFiles: []string{"Core/Auth/Federation.swift", "docs/09-auth.md"},
	}
	if skip, reason := Skip(pr); skip {
		t.Fatalf("a real feature PR must not be skipped (reason was %q)", reason)
	}
}

func TestSkip_MixedDocsAndCodeIsNotSkipped(t *testing.T) {
	// Docs CHANGED alongside code → still has a code change → not docs-only.
	pr := PR{Number: 3, Title: "fix: handle nil token", ChangedFiles: []string{"docs/x.md", "Core/Token.swift"}}
	if skip, _ := Skip(pr); skip {
		t.Fatal("mixed docs+code must not be treated as docs-only")
	}
}
