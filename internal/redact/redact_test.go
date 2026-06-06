package redact

import (
	"strings"
	"testing"
)

// denyList is the literal NDA deny-list used in tests. The production redactor
// is constructed with the same kind of list via New. These literals mirror the
// project's NDA constraint set (brand, payment provider, personal name).
var denyList = []string{"acmecorp", "PayVendor", "exampleapp", "exampletech"}

func newRedactor() Redactor { return New(denyList) }

func TestRedact_StructuralAWSAccountID(t *testing.T) {
	r := newRedactor()
	got := r.Redact("we operate in account 123456789012 today")
	if strings.Contains(got.Clean, "123456789012") {
		t.Fatalf("account id survived: %q", got.Clean)
	}
	if !strings.Contains(got.Clean, "<AWS_ACCOUNT_ID>") {
		t.Fatalf("expected placeholder, got: %q", got.Clean)
	}
	if len(got.Hits) != 1 || got.Hits[0].Kind != "aws-account-id" {
		t.Fatalf("expected one aws-account-id hit, got: %+v", got.Hits)
	}
}

func TestRedact_StructuralARN(t *testing.T) {
	r := newRedactor()
	got := r.Redact("role is arn:aws:iam::123456789012:role/Deploy here")
	if strings.Contains(got.Clean, "arn:aws:iam") {
		t.Fatalf("arn survived: %q", got.Clean)
	}
	if !strings.Contains(got.Clean, "<ARN>") {
		t.Fatalf("expected <ARN> placeholder, got: %q", got.Clean)
	}
}

func TestRedact_StructuralBundleID(t *testing.T) {
	r := newRedactor()
	// reverse-DNS bundle id with 3+ segments
	got := r.Redact("bundle com.acme.exampleapp ships")
	if strings.Contains(got.Clean, "com.acme") {
		t.Fatalf("bundle id survived: %q", got.Clean)
	}
}

func TestRedact_StructuralSecretTokens(t *testing.T) {
	r := newRedactor()
	cases := map[string]string{
		"ghp_0123456789abcdefghijklmnopqrstuvwxyz": "ghp_0123456789",
		"AKIAIOSFODNN7EXAMPLE":                     "AKIAIOSFODNN7EXAMPLE",
		"github_pat_11ABCDE0000aaaaaaaaaaa_bbbb":   "github_pat_11ABCDE",
	}
	for input, mustNotSurvive := range cases {
		got := r.Redact("token " + input + " end")
		if strings.Contains(got.Clean, mustNotSurvive) {
			t.Fatalf("secret %q survived in: %q", mustNotSurvive, got.Clean)
		}
	}
}

func TestRedact_LiteralDenyList(t *testing.T) {
	r := newRedactor()
	got := r.Redact("the acmecorp app uses PayVendor for payments")
	if strings.Contains(strings.ToLower(got.Clean), "acmecorp") {
		t.Fatalf("brand survived: %q", got.Clean)
	}
	if strings.Contains(got.Clean, "PayVendor") {
		t.Fatalf("payment provider survived: %q", got.Clean)
	}
}

func TestRedact_LiteralDenyListIsCaseInsensitive(t *testing.T) {
	r := newRedactor()
	got := r.Redact("ACMECORP and Acmecorp and acmecorp")
	if strings.Contains(strings.ToLower(got.Clean), "acmecorp") {
		t.Fatalf("case variants survived: %q", got.Clean)
	}
}

func TestRedact_CleanTextUnchanged(t *testing.T) {
	r := newRedactor()
	clean := "Use NavigationSplitView for adaptive iPad layouts; prefer actor isolation."
	got := r.Redact(clean)
	if got.Clean != clean {
		t.Fatalf("clean text was altered:\n in: %q\nout: %q", clean, got.Clean)
	}
	if len(got.Hits) != 0 {
		t.Fatalf("expected no hits on clean text, got: %+v", got.Hits)
	}
}

func TestRedact_DoesNotMangleGenericPlaceholders(t *testing.T) {
	r := newRedactor()
	// These are the deliberate generic-vocabulary tokens the emitted SKILL.md
	// uses (allow-list, threat C2). The redactor must leave them intact, else it
	// would corrupt safe output and create churn.
	allow := "Set the bundle id to com.example.app and the host to auth.example.com"
	got := r.Redact(allow)
	if got.Clean != allow {
		t.Fatalf("generic placeholders were mangled:\n in: %q\nout: %q", allow, got.Clean)
	}
}

func TestRedact_HitsRecordKindNotValue(t *testing.T) {
	r := newRedactor()
	got := r.Redact("account 123456789012")
	for _, h := range got.Hits {
		// The original secret value must never appear in a Hit (it would leak
		// into the rejection audit log).
		if strings.Contains(h.Kind, "123456789012") || strings.Contains(h.Placeholder, "123456789012") {
			t.Fatalf("hit leaked original value: %+v", h)
		}
	}
}
