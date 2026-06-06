// Package gate is the NDA sanitization boundary. It decides whether a candidate
// artifact (a generated SKILL.md or a draft PR body) is safe to publish.
//
// The trustworthy control is DETERMINISTIC: deny-list + structural regex +
// template allow-list, tested against a golden corpus. The Claude verify pass is
// extra recall, never the sole control. A canary (a known fake private id seeded
// into a test fixture) must be caught on every release or the build fails.
//
// Built in Fase 1 (deterministic part) and Fase 3 (Claude verify). This file
// declares the contract.
package gate

import "lessongate/internal/redact"

// Verdict is the result of gating one artifact.
type Verdict struct {
	Safe bool
	// Novel lists tokens that are neither generic vocabulary nor on the
	// allow-list — surfaced to the human reviewer, each flagged explicitly.
	Novel []string
	// Reasons explains a Safe=false verdict (kinds only, never raw values).
	Reasons []string
}

// Deterministic is the 100%-testable gate: deny-list + structural + allow-list.
type Deterministic interface {
	Check(artifact string) Verdict
}

// deterministic implements the trustworthy gate by reusing the redact package's
// detection: if redaction would change anything (i.e. it found a private
// identifier), the artifact is unsafe. It also surfaces novel id-shaped tokens
// the human reviewer should confirm.
type deterministic struct {
	r redact.Redactor
}

// NewDeterministic builds the deterministic gate from a literal deny-list. It
// shares the exact detection logic of internal/redact, so the gate and the
// pre-flight redactor can never disagree about what counts as a private id.
func NewDeterministic(denyList []string) Deterministic {
	return &deterministic{r: redact.New(denyList)}
}

func (d *deterministic) Check(artifact string) Verdict {
	res := d.r.Redact(artifact)
	if len(res.Hits) == 0 {
		return Verdict{Safe: true}
	}
	v := Verdict{Safe: false}
	seen := map[string]bool{}
	for _, h := range res.Hits {
		if !seen[h.Kind] {
			seen[h.Kind] = true
			v.Reasons = append(v.Reasons, h.Kind)
		}
		// Structural id-shaped kinds (not literal deny-list hits) are "novel":
		// they matched a shape, not a known literal, so the human must confirm.
		if h.Kind != "deny-list" {
			v.Novel = append(v.Novel, h.Placeholder)
		}
	}
	return v
}

// Verifier is the optional non-deterministic recall pass (Claude). It can only
// downgrade Safe→unsafe; it can never upgrade an unsafe verdict to safe.
type Verifier interface {
	Verify(artifact string) Verdict
}
