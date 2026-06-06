// Package extract asks Claude whether a (redacted) lesson is generalizable — a
// reusable engineering pattern not tied to the private business — and, if so,
// drafts the generic shape. Input is the REDACTED lesson text (post-redact),
// never the raw diff (threat C1). One lesson may already be generic, or yield
// nothing; a PR may produce several lessons (1:N handled upstream).
//
// Built in Fase 3. This file declares the contract.
package extract

import (
	"context"

	"lessongate/internal/claude"
)

// Candidate is a generalizable pattern proposed from a lesson. JSON tags match
// the emit_result tool's field names so the model's structured output decodes
// directly.
type Candidate struct {
	Generalizable bool   `json:"generalizable"`
	SkillName     string `json:"skill_name,omitzero"` // proposed kebab-case name
	Summary       string `json:"summary,omitzero"`    // one-line description for the skill
	Body          string `json:"body,omitzero"`       // generic pattern body, generated from template vocabulary
}

// Extractor runs the extract stage.
type Extractor struct {
	C claude.Client
}

// extractInstructions is the stable, cacheable prefix. It never contains the
// lesson text (that goes in the volatile suffix), so the prompt cache is reused
// across every lesson in a run.
const extractInstructions = `You are reviewing engineering lessons to find GENERALIZABLE patterns.

A lesson is generalizable if it teaches a reusable engineering pattern that is
NOT tied to a specific private business — e.g. a Swift concurrency idiom, an AWS
IAM scoping pattern, a Go initialization pattern. It is NOT generalizable if its
value is entirely in private/business-specific data (internal ticket numbers,
a specific account, one team's process).

Call the emit_result tool with:
- generalizable (bool)
- skill_name (kebab-case, <=64 chars, lowercase letters/digits/hyphens) — only if generalizable
- summary (one line describing when to use the pattern) — only if generalizable
- body (the generic pattern, using placeholder names like MyApp / com.example.app,
  never private identifiers) — only if generalizable

The lesson text follows after this instruction block.`

// Extract evaluates one redacted lesson. The redacted lesson is sent as the
// VOLATILE suffix so it sits after the cache breakpoint; the instructions are
// the cached prefix (threat I6 — cost).
func (e Extractor) Extract(ctx context.Context, redactedLesson string) (Candidate, error) {
	var c Candidate
	volatile := "Lesson:\n" + redactedLesson
	if err := e.C.Complete(ctx, extractInstructions, volatile, &c); err != nil {
		return Candidate{}, err
	}
	return c, nil
}
