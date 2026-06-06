package claude

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// sdkClient is the production Client backed by anthropic-sdk-go v1.46.0.
//
// Verified API surface (2026-06-05):
//   - Model anthropic.ModelClaudeOpus4_8.
//   - Opus 4.8 is adaptive-thinking-only: temperature/top_p/top_k/budget_tokens
//     all 400. For maximum determinism in the gate we rely on structured-output
//     forcing (a single-field tool), NOT a temperature knob.
//   - Prompt caching: NewCacheControlEphemeralParam() on the last System block
//     caches the stable prefix (system + deny-list + instructions); the volatile
//     lesson text goes in the user message after the breakpoint.
//
// This adapter is exercised by the Fase 5 smoke test, not unit tests (it hits
// the network). The cassette client covers the offline path.
type sdkClient struct {
	api   anthropic.Client
	model anthropic.Model
}

// NewSDKClient builds a production Client. The API key is read from the
// ANTHROPIC_API_KEY environment by the SDK unless overridden via opts.
func NewSDKClient(model string, opts ...option.RequestOption) Client {
	m := anthropic.Model(model)
	if model == "" {
		m = anthropic.ModelClaudeOpus4_8
	}
	return &sdkClient{api: anthropic.NewClient(opts...), model: m}
}

const structuredToolName = "emit_result"

// Complete sends the cached static prefix as a system block and the volatile
// suffix as the user turn, forcing a single-tool call whose input is the
// structured result, then decodes that input into out.
func (c *sdkClient) Complete(ctx context.Context, staticPrefix, volatile string, out any) error {
	// Force structured output via a tool the model must call. The tool's input
	// schema is permissive (free-form object); the stage prompt defines the
	// exact fields, and out's json tags drive decoding.
	tool := anthropic.ToolParam{
		Name:        structuredToolName,
		Description: anthropic.String("Return the structured result for this task."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
	}

	resp, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 8000,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		System: []anthropic.TextBlockParam{{
			Text:         staticPrefix,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(volatile)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: structuredToolName},
		},
	})
	if err != nil {
		return fmt.Errorf("claude: messages.new: %w", err)
	}

	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), out); err != nil {
				return fmt.Errorf("claude: decode tool input: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("claude: model returned no tool_use block")
}
