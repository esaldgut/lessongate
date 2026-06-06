// Package claude wraps the anthropic-sdk-go client for lessongate's two
// non-deterministic stages (extract, gate-verify). It pins the model ID, uses
// prompt caching for the static prefix (system + instructions + deny-list), and
// requests structured output via tool-use forcing. For the gate it sets the
// most deterministic configuration available so verify results are as stable as
// possible (the deterministic gate remains the trustworthy control).
//
// Built in Fase 3. This file declares the contract.
package claude

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Client is the minimal surface the pipeline needs from the Anthropic SDK,
// behind an interface so Fase 3 can record/replay responses (VCR cassettes) for
// deterministic offline CI.
type Client interface {
	// Complete sends a cached static prefix plus a volatile suffix and returns
	// the model's structured response decoded into out (a pointer).
	Complete(ctx context.Context, staticPrefix, volatile string, out any) error
}

// fingerprint is the deterministic key for a (staticPrefix, volatile) request,
// used to index cassettes. Independent of the model so cassettes survive a
// model-ID change in test fixtures.
func fingerprint(staticPrefix, volatile string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s", staticPrefix, volatile))
	return fmt.Sprintf("%x", sum[:])
}

// cassetteClient replays recorded JSON responses keyed by request fingerprint.
// It is the offline test double for the real SDK client — a miss is an error,
// never a silent pass, so CI can't accidentally skip a stage.
type cassetteClient struct {
	tape map[string]string // fingerprint → recorded JSON response
}

// NewCassetteClient builds a Client backed by a fingerprint→JSON tape.
func NewCassetteClient(tape map[string]string) Client {
	return &cassetteClient{tape: tape}
}

func (c *cassetteClient) Complete(_ context.Context, staticPrefix, volatile string, out any) error {
	raw, ok := c.tape[fingerprint(staticPrefix, volatile)]
	if !ok {
		return fmt.Errorf("claude: cassette miss for request (no recorded response)")
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("claude: decode cassette response: %w", err)
	}
	return nil
}
