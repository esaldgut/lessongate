package pipeline

// Canonical rejection reasons and error classes. These codes travel into the
// audit log and may be matched by consumers, so they live here as named
// constants — never as bare literals scattered across call sites (the
// no-literal-strings discipline). One literal, one symbol.

// Reason is why a lesson did not result in a draft PR.
type Reason = string

const (
	ReasonExtractError     Reason = "extract-error"     // the extract API call failed (infra)
	ReasonNonGeneralizable Reason = "non-generalizable" // the model judged it not reusable
	ReasonGateBlocked      Reason = "gate-blocked"      // the NDA gate (det or verify) blocked it
	ReasonDuplicate        Reason = "duplicate"         // reconcile found an existing equivalent skill
	ReasonEmitError        Reason = "emit-error"        // opening the draft PR failed (infra)
)

// ErrClass is a short, sanitized label for an upstream error, derived from the
// typed SDK error (status code / error type), NOT from string-matching the
// error message.
type ErrClass = string

const (
	ErrClassBilling     ErrClass = "credit-balance-too-low" // 400 + billing_error
	ErrClassAuth        ErrClass = "auth-401"               // 401
	ErrClassRateLimited ErrClass = "rate-limited-429"       // 429
	ErrClassBadRequest  ErrClass = "bad-request-400"        // 400 (other)
	ErrClassServer      ErrClass = "server-5xx"             // 5xx
	ErrClassUnknown     ErrClass = "error"                  // anything else
)
