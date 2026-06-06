package gate

import (
	"context"

	"lessongate/internal/claude"
)

// verifyResponse is the structured output of the Claude verify pass.
type verifyResponse struct {
	Leaked bool   `json:"leaked"`
	Reason string `json:"reason,omitzero"`
}

// verifyInstructions is the stable cacheable prefix for the verify pass. The
// artifact under review travels as the volatile suffix.
const verifyInstructions = `You are an adversarial reviewer checking whether a candidate skill artifact
leaks any private/confidential identifier that a deterministic regex gate might
miss: internal codenames, project names, a specific company or product, a person's
name, a non-obvious internal host, a vendor name tied to one business, etc.

Default to caution: if you are unsure whether something is a private identifier,
treat it as leaked. Call emit_result with:
- leaked (bool)
- reason (one line naming the KIND of leak, never quoting a secret verbatim)

The artifact follows after this instruction block.`

// claudeVerifier implements Verifier via the Claude API. It can only DOWNGRADE
// a verdict (safe→unsafe): it is extra recall on top of the deterministic gate,
// never the sole control (threat I4).
type claudeVerifier struct {
	c claude.Client
}

// NewVerifier builds a Claude-backed Verifier.
func NewVerifier(c claude.Client) Verifier {
	return &claudeVerifier{c: c}
}

func (v *claudeVerifier) Verify(artifact string) Verdict {
	var resp verifyResponse
	if err := v.c.Complete(context.Background(), verifyInstructions, artifact, &resp); err != nil {
		// Fail closed: if the verify pass errors, treat as unsafe so a human
		// reviews. The deterministic gate already ran; this only adds caution.
		return Verdict{Safe: false, Reasons: []string{"verify-pass-error"}}
	}
	if resp.Leaked {
		return Verdict{Safe: false, Reasons: []string{"claude-verify"}}
	}
	return Verdict{Safe: true}
}
