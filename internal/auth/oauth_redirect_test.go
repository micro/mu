package auth

import "testing"

// A code goes only where the client said to send it.
//
// /oauth/authorize took the redirect_uri from the query string and, when the
// visitor already had a session, issued a code and sent it there immediately —
// no check, no consent screen. One click on a crafted link by somebody signed
// in handed that account's authorization code to an address the attacker chose.
// PKCE is no help: in that attack the attacker generates the challenge, so they
// hold the verifier. Registration had been collecting redirect_uris the whole
// time and nothing ever read them.
func TestACodeOnlyGoesToARegisteredAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c := RegisterOAuthClient("", "Some MCP client", []string{"https://app.example.com/callback"})

	if got, err := RedirectFor(c.ClientID, "https://app.example.com/callback"); err != nil {
		t.Fatalf("the registered address was refused: %v", err)
	} else if got != "https://app.example.com/callback" {
		t.Errorf("RedirectFor = %q", got)
	}

	// The attack, in the forms it is actually attempted in.
	for _, evil := range []string{
		"https://attacker.example/callback",
		"https://app.example.com.attacker.example/callback",
		"https://app.example.com/callback/../../steal",
		"https://app.example.com/callback?next=https://attacker.example",
		"http://app.example.com/callback",
	} {
		if _, err := RedirectFor(c.ClientID, evil); err == nil {
			t.Errorf("a code would be sent to %q", evil)
		}
	}
}

// An unregistered client is refused outright. The registry was decorative
// during the flow — any client_id at all was accepted — which is also why
// removing a client did nothing.
func TestAnUnregisteredClientCannotStartAFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := RedirectFor("never-registered", "https://app.example.com/callback"); err == nil {
		t.Fatal("a client nobody registered can start an authorization flow")
	}
}

// The one exception the specification allows: a native or CLI client binds
// whatever port it can get, so the port of a loopback address may differ.
// RFC 8252 §7.3 — and Mu's own registration default is localhost:0.
func TestALoopbackClientMayComeBackOnAnyPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c := RegisterOAuthClient("", "A CLI", []string{"http://localhost:0/callback"})

	if _, err := RedirectFor(c.ClientID, "http://localhost:51763/callback"); err != nil {
		t.Errorf("a loopback client was refused its own port: %v", err)
	}
	if _, err := RedirectFor(c.ClientID, "http://127.0.0.1:8931/callback"); err != nil {
		t.Errorf("the loopback literal was refused: %v", err)
	}

	// The port is the only thing that may move. Everything else is exact.
	for _, evil := range []string{
		"http://localhost:51763/stolen",
		"http://localhost:51763/callback?x=1",
		"http://notlocalhost:51763/callback",
		"https://localhost:51763/callback",
	} {
		if _, err := RedirectFor(c.ClientID, evil); err == nil {
			t.Errorf("the loopback exception let through %q", evil)
		}
	}
}

// A client with exactly one address does not have to repeat it — the parameter
// is optional in that case, and clients do omit it.
func TestOneRegisteredAddressMayBeLeftUnsaid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	one := RegisterOAuthClient("", "One", []string{"https://app.example.com/callback"})
	if got, err := RedirectFor(one.ClientID, ""); err != nil || got != "https://app.example.com/callback" {
		t.Errorf("RedirectFor(\"\") = %q, %v", got, err)
	}

	// With more than one there is nothing to infer, so it must be said.
	two := RegisterOAuthClient("", "Two", []string{
		"https://app.example.com/callback", "https://other.example.com/callback"})
	if _, err := RedirectFor(two.ClientID, ""); err == nil {
		t.Error("a client with two addresses had one chosen for it")
	}
}

// What may be registered in the first place: localhost or HTTPS.
func TestOnlyLocalhostOrHTTPSMayBeRegistered(t *testing.T) {
	ok := []string{
		"https://app.example.com/callback",
		"http://localhost:0/callback",
		"http://127.0.0.1:1234/cb",
	}
	for _, u := range ok {
		if !RegisterableRedirect(u) {
			t.Errorf("%q should be registerable", u)
		}
	}
	bad := []string{
		"http://app.example.com/callback",
		"ftp://example.com/",
		"not a url",
		"",
	}
	for _, u := range bad {
		if RegisterableRedirect(u) {
			t.Errorf("%q should not be registerable", u)
		}
	}
}
