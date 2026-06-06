package gate

import (
	"context"
	"testing"
)

// verifyClient is a claude.Client stub returning a fixed verify verdict.
type verifyClient struct {
	leaked bool
	reason string
}

func (v verifyClient) Complete(_ context.Context, _, _ string, out any) error {
	// Decode into whatever the verifier expects by writing through a tiny shim.
	res := out.(*verifyResponse)
	res.Leaked = v.leaked
	res.Reason = v.reason
	return nil
}

func TestVerifier_CanDowngradeSafeToUnsafe(t *testing.T) {
	// The deterministic gate passed it, but the Claude pass spots a subtle leak.
	v := NewVerifier(verifyClient{leaked: true, reason: "contains an internal codename"})
	verdict := v.Verify("a skill that looks clean to regex but names Project Bluebird")
	if verdict.Safe {
		t.Fatal("verifier must be able to downgrade a clean-looking artifact to unsafe")
	}
	if len(verdict.Reasons) == 0 {
		t.Fatal("expected a reason for the downgrade")
	}
}

func TestVerifier_CleanArtifactStaysSafe(t *testing.T) {
	v := NewVerifier(verifyClient{leaked: false})
	verdict := v.Verify("Use NavigationSplitView for adaptive layouts.")
	if !verdict.Safe {
		t.Fatalf("clean artifact downgraded: %v", verdict.Reasons)
	}
}
