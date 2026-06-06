// Package redact performs the deterministic PRE-FLIGHT pass that runs BEFORE any
// content leaves the process (i.e. before the Claude API call). It replaces
// known private identifiers — both literal deny-list entries and structural
// patterns (12-digit account IDs, ARNs, bundle IDs, secret-shaped tokens) — with
// placeholders. This is what resolves threat C1: the API only ever sees redacted
// lesson text, never raw private identifiers.
//
// Built in Fase 1 (sanitizer-first). This file declares the contract.
package redact

import (
	"regexp"
	"strings"
)

// Result is the outcome of a redaction pass.
type Result struct {
	// Clean is the text with all matched identifiers replaced by placeholders.
	Clean string
	// Hits records what was replaced (kind + placeholder), for the sanitized
	// rejection audit log — never the original value.
	Hits []Hit
}

// Hit describes one replacement. Original value is intentionally absent.
type Hit struct {
	Kind        string // "deny-list", "aws-account-id", "arn", "bundle-id", ...
	Placeholder string // what it was replaced with, e.g. "<AWS_ACCOUNT_ID>"
}

// Redactor applies the deny-list + structural patterns. Constructed from config.
type Redactor interface {
	Redact(text string) Result
}

// structuralPattern pairs a compiled regex with the kind/placeholder it maps to.
// Structural patterns catch *classes* of identifiers (threat C2): they fire on
// shapes (12-digit account IDs, ARNs, secret-token prefixes) the literal
// deny-list can never enumerate.
type structuralPattern struct {
	re          *regexp.Regexp
	kind        string
	placeholder string
}

// structural is ordered: more specific patterns first so they win over broader
// ones (e.g. an ARN is matched before a bare account-id inside it).
var structural = []structuralPattern{
	{regexp.MustCompile(`arn:aws:[^\s"']+`), "arn", "<ARN>"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "aws-access-key", "<AWS_ACCESS_KEY>"},
	{regexp.MustCompile(`ghp_[0-9A-Za-z]{36}`), "github-pat", "<GITHUB_TOKEN>"},
	{regexp.MustCompile(`github_pat_[0-9A-Za-z_]{22,}`), "github-pat", "<GITHUB_TOKEN>"},
	{regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]+`), "slack-token", "<SLACK_TOKEN>"},
	{regexp.MustCompile(`sk-ant-[0-9A-Za-z-]+`), "anthropic-key", "<ANTHROPIC_KEY>"},
	// reverse-DNS bundle id: 3+ dot-separated lowercase-alnum segments
	{regexp.MustCompile(`\b[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*){2,}\b`), "bundle-id", "<BUNDLE_ID>"},
	// 12-digit AWS account id (kept after ARN so ARNs are consumed first)
	{regexp.MustCompile(`\b\d{12}\b`), "aws-account-id", "<AWS_ACCOUNT_ID>"},
}

// allowList holds the deliberate generic-vocabulary tokens the emitted SKILL.md
// uses (threat C2). They are dotted/structured like real identifiers, so they
// must be exempted from structural matching or the redactor would corrupt safe
// output. Matched as whole tokens, case-insensitively.
var allowList = regexp.MustCompile(`(?i)\b(?:` + strings.Join([]string{
	`com\.example\.[a-z][a-z0-9]*`, // com.example.app, com.example.ios, ...
	`[a-z][a-z0-9-]*\.example\.com`, // auth.example.com, api.example.com, ...
	`developer\.apple\.com`,
	`docs\.aws\.amazon\.com`,
}, "|") + `)\b`)

const allowSentinel = "\x00ALLOW\x00" // private marker, never appears in real text

type redactor struct {
	deny *regexp.Regexp // case-insensitive alternation of literal deny-list terms
}

// New builds a Redactor from a literal deny-list. The deny-list is a
// case-insensitive, defense-in-depth layer; the structural patterns above are
// the primary control.
func New(denyList []string) Redactor {
	r := &redactor{}
	if len(denyList) > 0 {
		quoted := make([]string, len(denyList))
		for i, t := range denyList {
			quoted[i] = regexp.QuoteMeta(t)
		}
		r.deny = regexp.MustCompile(`(?i)(` + strings.Join(quoted, "|") + `)`)
	}
	return r
}

func (r *redactor) Redact(text string) Result {
	var hits []Hit
	out := text

	// Stash allow-listed generic tokens so structural patterns don't mangle them.
	var stash []string
	out = allowList.ReplaceAllStringFunc(out, func(m string) string {
		stash = append(stash, m)
		return allowSentinel
	})

	// Structural first (the trustworthy control), in declared order.
	for _, p := range structural {
		if p.re.MatchString(out) {
			out = p.re.ReplaceAllStringFunc(out, func(string) string {
				hits = append(hits, Hit{Kind: p.kind, Placeholder: p.placeholder})
				return p.placeholder
			})
		}
	}

	// Literal deny-list (defense in depth).
	if r.deny != nil && r.deny.MatchString(out) {
		out = r.deny.ReplaceAllStringFunc(out, func(string) string {
			hits = append(hits, Hit{Kind: "deny-list", Placeholder: "<REDACTED>"})
			return "<REDACTED>"
		})
	}

	// Restore stashed allow-listed tokens, in order.
	for _, orig := range stash {
		out = strings.Replace(out, allowSentinel, orig, 1)
	}

	return Result{Clean: out, Hits: hits}
}
