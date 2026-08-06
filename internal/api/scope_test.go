package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/service"
)

// ScopeProbe is a service registered by these tests. Reading the live registry
// instead would make the result depend on which service packages happen to be
// linked into this test binary, and a confinement rule must not be tested by an
// accident of imports.
type ScopeProbe struct{}

func (ScopeProbe) List(ctx context.Context, req *struct{}, rsp *struct {
	Text string `json:"text"`
}) error {
	return nil
}

func registerScopeProbes(t *testing.T) {
	t.Helper()
	for _, name := range []string{"scopeallowed", "scopedenied"} {
		if _, known := service.SpecFor(name); known {
			continue
		}
		if err := service.Register(service.Spec{
			Name: name, Handler: new(ScopeProbe), Page: "/" + name, Scoped: true,
			Endpoints: map[string]service.Endpoint{"List": {Doc: "probe"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// The check that makes a scope real.
//
// Without it the scope is a label on a settings page: every dispatch branch
// resolves the caller to an account and calls the tool, so a "news only" agent
// holding a valid token could read mail. This asserts the boundary refuses.
func TestAScopedTokenIsRefusedToolsOutsideItsScope(t *testing.T) {
	acc := &auth.Account{ID: "scopecaller", Name: "scopecaller", Secret: "s"}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	registerScopeProbes(t)
	_, secret, err := auth.CreateToken(acc.ID, "agent: reader", auth.ScopeFor([]string{"scopeallowed"}), time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	r, _ := http.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+secret)

	if err := checkTokenScope(r, "scopeallowed_list"); err != nil {
		t.Errorf("a token was refused the service it names: %v", err)
	}
	if err := checkTokenScope(r, "scopedenied_list"); err == nil {
		t.Error("a scoped token reached a service it does not name")
	}
}

// A tool with no service behind it — the platform verbs — is reachable only by
// an unscoped token. Somebody who said "news and mail" did not say "and
// whatever else is not a service".
func TestAScopedTokenCannotReachToolsWithNoServiceBehindThem(t *testing.T) {
	acc := &auth.Account{ID: "scopecaller2", Name: "scopecaller2", Secret: "s"}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	registerScopeProbes(t)
	_, secret, err := auth.CreateToken(acc.ID, "agent: reader", auth.ScopeFor([]string{"scopeallowed"}), time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	r, _ := http.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+secret)

	if err := checkTokenScope(r, "somethingwithnoservice"); err == nil {
		t.Error("a scoped token reached a tool with no service behind it")
	}
}

// Everything that authenticated some other way is unconfined: a cookie session
// is a person in their own browser, and every token issued before scopes
// existed must keep working exactly as it did.
func TestUnscopedCallersAreNotConfined(t *testing.T) {
	acc := &auth.Account{ID: "scopecaller3", Name: "scopecaller3", Secret: "s"}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	_, secret, err := auth.CreateToken(acc.ID, "plain", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	withToken, _ := http.NewRequest("POST", "/mcp", nil)
	withToken.Header.Set("Authorization", "Bearer "+secret)

	noAuth, _ := http.NewRequest("POST", "/mcp", nil)

	for _, r := range []*http.Request{withToken, noAuth, nil} {
		for _, tool := range []string{"scopeallowed_list", "scopedenied_list", "whatever"} {
			if err := checkTokenScope(r, tool); err != nil {
				t.Errorf("an unscoped caller was refused %s: %v", tool, err)
			}
		}
	}
}
