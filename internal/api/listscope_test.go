package api

import (
	"net/http"
	"testing"
	"time"

	"mu/internal/auth"
)

// A scoped token should be shown the tools it may use, and no others.
//
// Scoping was enforced at dispatch and ignored by tools/list, so an agent you
// had deliberately confined to two services was handed every tool definition on
// the instance and found out by trial and error that almost all of them were
// refused. That is the worst of both: the context cost of the whole catalogue
// and none of its use.
func TestToolsListFollowsTheTokenScope(t *testing.T) {
	secret := scopedTokenFor(t, "listscope", "scopeallowed")

	r, _ := http.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	if got := scopeFrom(r); len(got) != 1 || got[0] != "scopeallowed" {
		t.Fatalf("expected the listing to follow the token's scope, got %v", got)
	}

	// An explicit ?tools= still wins, because it can only narrow further — and
	// every name in it is checked against the token when called anyway.
	r2, _ := http.NewRequest("POST", "/mcp?tools=news", nil)
	r2.Header.Set("Authorization", "Bearer "+secret)
	if got := scopeFrom(r2); len(got) != 1 || got[0] != "news" {
		t.Fatalf("expected the explicit ?tools= list to win, got %v", got)
	}

	// A caller with no token is not confined at all.
	plain, _ := http.NewRequest("POST", "/mcp", nil)
	if got := scopeFrom(plain); len(got) != 0 {
		t.Fatalf("expected an unconfined caller to see everything, got %v", got)
	}
}

// The filter the listing applies must agree with the check dispatch applies. If
// they disagree the catalogue either promises what the boundary refuses, or
// hides what it would have allowed.
func TestListedToolsAreExactlyTheCallableOnes(t *testing.T) {
	secret := scopedTokenFor(t, "listagree", "scopeallowed")
	r, _ := http.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+secret)

	scope := scopeFrom(r)
	checked := 0
	for _, tool := range mcpTools() {
		listed := inScope(tool, scope)
		callable := checkTokenScope(r, tool.Name) == nil
		if listed != callable {
			t.Errorf("%s: listed=%v callable=%v — the catalogue and the boundary disagree",
				tool.Name, listed, callable)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no tools were registered, so this proved nothing")
	}
}

// scopedTokenFor mints a token confined to the named services and returns its
// secret, using the same probe services as the dispatch scope tests so the
// result does not depend on which service packages this binary happens to link.
func scopedTokenFor(t *testing.T, account string, services ...string) string {
	t.Helper()
	registerScopeProbes(t)
	acc := &auth.Account{ID: account, Name: account, Secret: "s"}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	_, secret, err := auth.CreateToken(acc.ID, "agent: "+account, auth.ScopeFor(services), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

// The number on the front page has to be the number an agent gets.
//
// It was len(tools) — every registered entry, including the RESTOnly ones that
// are HTTP endpoints and not tools — so the landing advertised 84 while
// tools/list served 82. That is the first claim anybody checks and it is
// checkable in one curl.
func TestToolCountIsWhatAnAgentIsOffered(t *testing.T) {
	if got, want := ToolCount(), len(mcpTools()); got != want {
		t.Fatalf("the landing would claim %d tools, an agent is offered %d", got, want)
	}
	// And that is not simply every registered entry, or the check above is
	// tautological the moment RESTOnly stops being filtered.
	rest := 0
	for _, tl := range tools {
		if tl.RESTOnly {
			rest++
		}
	}
	if rest > 0 && ToolCount() == len(tools) {
		t.Fatalf("%d REST-only endpoints are being counted as tools", rest)
	}
}
