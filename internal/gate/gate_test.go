package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newGate() Deterministic {
	// Fake brand/vendor literals — same SHAPE as a real deny-list, but no real
	// NDA term lives in this public-repo test fixture.
	return NewDeterministic([]string{"acmecorp", "PayVendor", "exampletech"})
}

// TestGate_GoldenCorpus runs every fixture under testdata/corpus. Files in
// leaky/ MUST be flagged unsafe; files in clean/ MUST pass. A single
// false-negative (leaky marked safe) fails the build — this is the trustworthy
// control and it must have zero tolerance.
func TestGate_GoldenCorpus(t *testing.T) {
	g := newGate()
	root := filepath.Join("..", "..", "testdata", "corpus")

	t.Run("leaky-must-be-blocked", func(t *testing.T) {
		dir := filepath.Join(root, "leaky")
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read leaky corpus: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("leaky corpus is empty — the gate has nothing to prove against")
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			t.Run(e.Name(), func(t *testing.T) {
				b, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				v := g.Check(string(b))
				if v.Safe {
					t.Fatalf("FALSE NEGATIVE: leaky fixture %q passed the gate", e.Name())
				}
			})
		}
	})

	t.Run("clean-must-pass", func(t *testing.T) {
		dir := filepath.Join(root, "clean")
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read clean corpus: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("clean corpus is empty")
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			t.Run(e.Name(), func(t *testing.T) {
				b, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				v := g.Check(string(b))
				if !v.Safe {
					t.Fatalf("FALSE POSITIVE: clean fixture %q was blocked: %v", e.Name(), v.Reasons)
				}
			})
		}
	})
}

// TestGate_Canary seeds a known fake private identifier and asserts the gate
// always catches it. If this ever passes, the gate has silently weakened.
func TestGate_Canary(t *testing.T) {
	g := newGate()
	const canary = "CANARY account 123456789012 with arn:aws:iam::123456789012:role/Seed"
	v := g.Check(canary)
	if v.Safe {
		t.Fatal("CANARY LEAKED: the gate failed to catch a seeded private identifier")
	}
}

func TestGate_FlagsNovelTokens(t *testing.T) {
	g := newGate()
	// A token that is neither generic vocabulary nor on the deny-list, but is
	// shaped like a private identifier, should be surfaced as novel.
	v := g.Check("integration with NewPaymentVendorXYZ account 999988887777")
	if v.Safe {
		t.Fatal("expected unsafe due to 12-digit account id")
	}
	if len(v.Novel) == 0 {
		t.Fatalf("expected at least one novel token surfaced, got none")
	}
}

func TestGate_CleanGenericSkillPasses(t *testing.T) {
	g := newGate()
	skill := `---
name: swift-adaptive-layout
description: Use NavigationSplitView for iPad layouts.
---
Set the bundle id to com.example.app. See developer.apple.com for details.`
	v := g.Check(skill)
	if !v.Safe {
		t.Fatalf("a clean generic skill was blocked: %v", v.Reasons)
	}
	if strings.Contains(strings.Join(v.Reasons, " "), "example") {
		t.Fatalf("generic vocabulary wrongly flagged: %v", v.Reasons)
	}
}
