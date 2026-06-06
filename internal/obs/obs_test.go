package obs

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// newTestLogger returns a logger writing JSON to buf, wrapped in the redactor,
// so tests can assert on exactly what would hit the sink.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	base := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(redactingHandler{inner: base})
}

func TestRedactor_ScrubsStructuralSecrets(t *testing.T) {
	cases := []struct {
		name    string
		leak    string
		mustNot string // substring that must NOT survive into the sink
	}{
		{"aws-account-id", "processing account 123456789012 now", "123456789012"},
		{"arn", "role arn:aws:iam::123456789012:role/Foo", "arn:aws:iam"},
		{"aws-access-key", "key AKIAIOSFODNN7EXAMPLE leaked", "AKIAIOSFODNN7EXAMPLE"},
		{"github-pat-classic", "token ghp_0123456789abcdefghijklmnopqrstuvwxyz", "ghp_0123456789"},
		{"github-pat-fine", "token github_pat_11ABCDE0000aaaaaaaaaaa_bbbb", "github_pat_11ABCDE"},
		{"slack", "xoxb-12345-67890-abcdef", "xoxb-12345"},
		{"anthropic-key", "sk-ant-api03-abcDEF123", "sk-ant-api03"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := newTestLogger(&buf)

			// Leak via both the message and an attribute value — both paths scrub.
			log.Info(tc.leak, "detail", tc.leak)

			out := buf.String()
			if strings.Contains(out, tc.mustNot) {
				t.Fatalf("secret survived into log sink: found %q in:\n%s", tc.mustNot, out)
			}
			if !strings.Contains(out, redacted) {
				t.Fatalf("expected %q marker in output, got:\n%s", redacted, out)
			}
		})
	}
}

func TestRedactor_PassesCleanText(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	log.Info("processed lesson", "stage", "extracted", "count", 3)

	out := buf.String()
	if strings.Contains(out, redacted) {
		t.Fatalf("clean text was wrongly redacted:\n%s", out)
	}
	for _, want := range []string{"processed lesson", "extracted", "\"count\":3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q to survive, got:\n%s", want, out)
		}
	}
}

func TestRedactor_WithAttrsScrubs(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	// Secrets attached via With() must also be scrubbed.
	log.With("creds", "AKIAIOSFODNN7EXAMPLE").Info("starting")
	if strings.Contains(buf.String(), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("secret in With() attr survived:\n%s", buf.String())
	}
}

func TestRedactingHandler_Enabled(t *testing.T) {
	var buf bytes.Buffer
	h := redactingHandler{inner: slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})}
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("INFO should be disabled at WARN level")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("ERROR should be enabled at WARN level")
	}
}
