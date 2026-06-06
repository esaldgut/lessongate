package claude

import (
	"context"
	"testing"
)

// extractResult is the shape the extract stage decodes into — used here to
// prove Complete decodes structured output into a caller-provided pointer.
type extractResult struct {
	Generalizable bool   `json:"generalizable"`
	SkillName     string `json:"skill_name"`
}

func TestCassetteClient_ReplaysRecordedResponse(t *testing.T) {
	// A cassette maps a request fingerprint → a recorded JSON response, so CI
	// runs the Claude stages deterministically with no network.
	cassette := map[string]string{
		fingerprint("SYS", "is this generalizable?"): `{"generalizable": true, "skill_name": "swift-actor-isolation"}`,
	}
	c := NewCassetteClient(cassette)

	var out extractResult
	err := c.Complete(context.Background(), "SYS", "is this generalizable?", &out)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !out.Generalizable || out.SkillName != "swift-actor-isolation" {
		t.Fatalf("decoded wrong result: %+v", out)
	}
}

func TestCassetteClient_MissReturnsError(t *testing.T) {
	c := NewCassetteClient(map[string]string{})
	var out extractResult
	err := c.Complete(context.Background(), "SYS", "unrecorded prompt", &out)
	if err == nil {
		t.Fatal("expected error on cassette miss (CI must not silently pass)")
	}
}

func TestFingerprint_StableForSameInput(t *testing.T) {
	a := fingerprint("system prefix", "volatile suffix")
	b := fingerprint("system prefix", "volatile suffix")
	if a != b {
		t.Fatal("fingerprint must be deterministic for identical input")
	}
}

func TestFingerprint_DiffersOnDifferentInput(t *testing.T) {
	if fingerprint("a", "b") == fingerprint("a", "c") {
		t.Fatal("fingerprint must differ when the volatile suffix differs")
	}
}
