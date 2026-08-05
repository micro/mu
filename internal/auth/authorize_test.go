package auth

import (
	"net/url"
	"strings"
	"testing"
)

// A client following the MCP authorization spec registers itself, opens a
// browser and lands the user on this page. Someone arriving for the first time
// met a username and a password field and nothing else — the flow could
// authenticate an account but not enrol one, so a new user had no way forward
// and the client sat waiting.
func TestAuthorizePageOffersAWayToCreateAnAccount(t *testing.T) {
	page := authorizePage("cid-1", "https://claude.ai/api/mcp/auth_callback",
		"st", "chal", "S256", "", "")

	if !strings.Contains(page, "/signup?redirect=") {
		t.Fatalf("no way to create an account:\n%s", page)
	}

	// The whole request has to come back, or the client is left waiting after
	// the account is made.
	i := strings.Index(page, "/signup?redirect=")
	rest := page[i+len("/signup?redirect="):]
	enc := rest[:strings.IndexAny(rest, `"`)]
	back, err := url.QueryUnescape(enc)
	if err != nil {
		t.Fatalf("the redirect is not decodable: %v", err)
	}
	if !strings.HasPrefix(back, "/oauth/authorize?") {
		t.Errorf("signup returns to %q, not the authorize request", back)
	}
	q, err := url.ParseQuery(strings.TrimPrefix(back, "/oauth/authorize?"))
	if err != nil {
		t.Fatalf("the returned request is not parseable: %v", err)
	}
	for k, want := range map[string]string{
		"client_id": "cid-1", "redirect_uri": "https://claude.ai/api/mcp/auth_callback",
		"state": "st", "code_challenge": "chal", "code_challenge_method": "S256",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("the round trip lost %s: got %q, want %q", k, got, want)
		}
	}
}

// Both renders come from one function so the sign-in screen and the
// wrong-password screen cannot drift — the second used to be a second copy of
// the HTML, and it was the copy that never got the signup link.
func TestTheFailedAttemptKeepsTheSameWayOut(t *testing.T) {
	page := authorizePage("cid-1", "https://example.com/cb", "", "", "", "someone", "Invalid username or password.")

	if !strings.Contains(page, "Invalid username or password.") {
		t.Error("the failure is not reported")
	}
	if !strings.Contains(page, "/signup?redirect=") {
		t.Error("the failed attempt has no way to create an account")
	}
	if !strings.Contains(page, `value="someone"`) {
		t.Error("the username was not kept, so it has to be typed again")
	}
}

// The page is built from values a client supplies, so they are escaped.
func TestAuthorizePageEscapesClientSuppliedValues(t *testing.T) {
	page := authorizePage(`"><script>alert(1)</script>`, "https://example.com/cb", "", "", "", "", "")
	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Error("a client_id was rendered unescaped into the page")
	}
}
