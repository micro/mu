package auth

import (
	"net/http"
	"testing"
	"time"
)

// Every token ever issued before scopes existed has no scope, and must go on
// reaching everything. A confinement that silently applied to old credentials
// would break every working integration on upgrade.
func TestAnUnscopedTokenReachesEverything(t *testing.T) {
	tok := &Token{Permissions: nil}
	if tok.Scoped() {
		t.Error("a token with no permissions reads as scoped")
	}
	for _, svc := range []string{"news", "mail", "events", "anything"} {
		if !tok.AllowsService(svc) {
			t.Errorf("an unscoped token was refused %s", svc)
		}
	}

	// Permissions that are not scopes are not scopes.
	legacy := &Token{Permissions: []string{"read", "write", "admin"}}
	if legacy.Scoped() {
		t.Error("an old permission list was mistaken for a scope")
	}
	if !legacy.AllowsService("mail") {
		t.Error("an old token was confined by permissions that are not scopes")
	}
}

// The point of the whole feature: a token that names services reaches those and
// is refused everything else, default-deny.
func TestAScopedTokenIsConfinedToWhatItNames(t *testing.T) {
	tok := &Token{Permissions: ScopeFor([]string{"news", "weather"})}

	if !tok.Scoped() {
		t.Fatal("a token with named services does not read as scoped")
	}
	for _, allowed := range []string{"news", "weather"} {
		if !tok.AllowsService(allowed) {
			t.Errorf("a scoped token was refused %s, which it names", allowed)
		}
	}
	for _, denied := range []string{"mail", "events", "contacts", "wallet", "files"} {
		if tok.AllowsService(denied) {
			t.Errorf("a token scoped to news and weather reached %s", denied)
		}
	}
}

// Case and spacing are how a scope gets written by hand or by a form post, and
// none of it should change what is reachable.
func TestScopeMatchingIsNotFooledByFormatting(t *testing.T) {
	tok := &Token{Permissions: ScopeFor([]string{" News ", "MAIL", "news", ""})}

	if got := len(tok.Services()); got != 2 {
		t.Fatalf("scope has %d entries, want 2 after trimming and deduping: %v", got, tok.Services())
	}
	if !tok.AllowsService("news") || !tok.AllowsService(" MAIL ") {
		t.Error("formatting changed what a scope allows")
	}
}

// A scope names a service, never a tool. A tool added to a service later must
// not widen a grant by accident, and a person choosing a scope is thinking in
// services — not enumerating news_list, news_read and news_search.
func TestAScopeNamesServicesNotTools(t *testing.T) {
	for _, p := range ScopeFor([]string{"news"}) {
		if p != ScopePrefix+"news" {
			t.Errorf("scope entry %q is not a service scope", p)
		}
	}
	tok := &Token{Permissions: ScopeFor([]string{"news"})}
	// The tool name is resolved to its service by the caller; the token itself
	// must not accidentally match a tool name.
	if tok.AllowsService("news_list") {
		t.Error("a scope matched a tool name, which would let a renamed tool widen the grant")
	}
}

// A cookie session is a person in their own browser, not a program holding a
// credential, so there is nothing to confine.
func TestARequestWithNoTokenIsNotConfined(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	if got := TokenFromRequest(r); got != nil {
		t.Errorf("a request with no Authorization header produced a token: %+v", got)
	}

	r.Header.Set("Authorization", "Bearer not-a-real-token")
	if got := TokenFromRequest(r); got != nil {
		t.Errorf("an unrecognised bearer token resolved to %+v", got)
	}
}

// End to end through the real store: the token a request presents is the token
// whose scope is read.
func TestThePresentedTokenIsTheOneWhoseScopeApplies(t *testing.T) {
	acc := &Account{ID: "scopeowner", Name: "scopeowner", Secret: "s"}
	if err := Create(acc); err != nil {
		t.Fatal(err)
	}

	_, narrowSecret, err := CreateToken(acc.ID, "narrow", ScopeFor([]string{"news"}), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, wideSecret, err := CreateToken(acc.ID, "wide", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+narrowSecret)
	narrow := TokenFromRequest(r)
	if narrow == nil {
		t.Fatal("the presented token did not resolve")
	}
	if !narrow.AllowsService("news") || narrow.AllowsService("mail") {
		t.Errorf("the narrow token allows %v", narrow.Services())
	}

	r2, _ := http.NewRequest("GET", "/", nil)
	r2.Header.Set("Authorization", "Bearer "+wideSecret)
	wide := TokenFromRequest(r2)
	if wide == nil {
		t.Fatal("the unscoped token did not resolve")
	}
	if wide.Scoped() {
		t.Error("a token created with no scope reads as scoped")
	}
	if !wide.AllowsService("mail") {
		t.Error("an unscoped token was confined")
	}
}
