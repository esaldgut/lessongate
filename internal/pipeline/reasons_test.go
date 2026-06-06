package pipeline

import (
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/shared"
)

// apiErrStub satisfies the apiError interface classifyErr uses, so the typed
// classification can be tested without constructing an SDK *anthropic.Error
// (which has unexported fields).
type apiErrStub struct {
	status int
	typ    string
	msg    string
}

func (e *apiErrStub) Status() int            { return e.status }
func (e *apiErrStub) Type() shared.ErrorType { return shared.ErrorType(e.typ) }
func (e *apiErrStub) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "api error"
}

func TestClassifyErr_UsesTypedStatusCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrClass
	}{
		{"401 auth", &apiErrStub{status: 401, typ: "authentication_error"}, ErrClassAuth},
		{"429 rate", &apiErrStub{status: 429, typ: "rate_limit_error"}, ErrClassRateLimited},
		{"400 billing", &apiErrStub{status: 400, typ: "billing_error"}, ErrClassBilling},
		{"400 other", &apiErrStub{status: 400, typ: "invalid_request_error"}, ErrClassBadRequest},
		{"500 server", &apiErrStub{status: 500, typ: "api_error"}, ErrClassServer},
		{"non-api error", errors.New("dial tcp: connection refused"), ErrClassUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyErr(tc.err); got != tc.want {
				t.Fatalf("classifyErr = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestClassifyErr_DoesNotStringMatchMessage proves classification is driven by
// the typed status code, not by substring-matching the message — a billing
// error whose MESSAGE happens not to contain "credit" still classifies as
// billing via its type/status.
func TestClassifyErr_DoesNotStringMatchMessage(t *testing.T) {
	e := &apiErrStub{status: 400, typ: "billing_error", msg: "insufficient funds"}
	if got := classifyErr(e); got != ErrClassBilling {
		t.Fatalf("typed billing error misclassified as %q (string-matching the message?)", got)
	}
}
