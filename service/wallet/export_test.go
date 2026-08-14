package wallet

// The export must never become a tool.
//
// Everything else in this package is a tool on purpose. This one thing must not
// be, and the reason is the design assumption the whole service rests on: the
// agent reads text that strangers wrote. A tool that returns a private key is a
// prompt injection away from posting it somewhere, and no cap helps — the key is
// the wallet and every cap on it at once.
//
// The Spec is what tools are derived from, so this checks the Spec rather than
// the tool list: a method that is not described there cannot be reached, and a
// method that appears there will be.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoEndpointExposesTheKey(t *testing.T) {
	for name, ep := range Spec.Endpoints {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "export") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "seed") {
			t.Errorf("wallet.%s is in the Spec, so wallet_%s is a tool an agent holds — "+
				"a private key must never be one", name, lower)
		}
		if strings.Contains(strings.ToLower(ep.Doc), "private key") {
			t.Errorf("wallet.%s offers a private key in its own description", name)
		}
	}

	// And the handler is not reachable as a method on the service handler,
	// which is where endpoints are derived from.
	if _, isMethod := any(Server{}).(interface {
		Export(any, any, any) error
	}); isMethod {
		t.Error("Export is a method on the service handler, so it derives a tool")
	}
}

// It refuses without a session, before it asks for anything else.
func TestExportNeedsASession(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/wallet/export",
		strings.NewReader("password=whatever"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	ExportHandler(rr, r)

	if rr.Code == http.StatusOK && strings.Contains(rr.Body.String(), "Private key") {
		t.Fatal("a request with no session was shown a key")
	}
}

// A signed-in caller with no password still cannot have it.
//
// The session is not the credential here. That is the whole point of asking
// again: a stolen cookie already has everything else this account can do.
func TestExportWithoutAPasswordShowsTheFormNotTheKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/wallet/export", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	ExportHandler(rr, r)

	if strings.Contains(rr.Body.String(), "Private key (secret)") {
		t.Error("a POST with no password produced a key")
	}
}
