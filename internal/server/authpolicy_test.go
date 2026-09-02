package server

// Which paths need a session, decided the same way every time.
//
// The answer used to depend on Go's map iteration order. The check ranged over
// the policy map and broke on the first prefix that matched, so wherever one
// entry is a prefix of another — and there are a dozen such pairs — which of
// the two answered was random per request.
//
// Ten identical requests to the unsubscribe link on the live instance came back
//
//	303 303 303 404 303 404 404 303 303 303
//
// A link in an email that works slightly less than half the time, which is
// worse than one that never works: nobody reports it, they just give up.

import (
	"strings"
	"testing"
)

// resolve is the rule the middleware applies: longest prefix wins.
func resolve(policy map[string]bool, path string) (bool, bool) {
	longest, authed, found := -1, false, false
	for url, need := range policy {
		if strings.HasPrefix(path, url) && len(url) > longest {
			longest, authed, found = len(url), need, true
		}
	}
	return authed, found
}

// The specific line is there to override the general one, which is the only
// reason to write both.
func TestTheMoreSpecificPathWins(t *testing.T) {
	policy := authRequired()

	for path, want := range map[string]bool{
		"/mail":             true,  // your inbox
		"/mail/unsubscribe": false, // the token in the link is the credential
		// A public page whose one private slice is named separately. The
		// listing of this instance's own agents is for anybody; the JSON of
		// *your* agents behind the chat's picker is not, and it is a longer
		// prefix, so it has to win.
		"/agents":          false,
		"/agents/data":     true,
		"/oauth2/google":   false, // sign-in start, no session yet
		"/oauth2/callback": false,
	} {
		got, found := resolve(policy, path)
		if !found {
			t.Errorf("%s matches no policy entry", path)
			continue
		}
		if got != want {
			t.Errorf("%s: auth required = %v, want %v", path, got, want)
		}
	}
}

// And the same answer every time, whatever order the map happens to yield.
//
// The bug was not that the rule was wrong; it was that there was no rule. This
// runs the resolution enough times that a random winner would show up.
func TestThePolicyIsNotDecidedByMapOrder(t *testing.T) {
	policy := authRequired()

	for _, path := range []string{
		"/mail/unsubscribe", "/agents/data", "/oauth2/google/connect", "/maps/tile",
	} {
		first, found := resolve(policy, path)
		if !found {
			continue
		}
		for i := 0; i < 500; i++ {
			got, _ := resolve(policy, path)
			if got != first {
				t.Fatalf("%s answered %v and then %v — the policy depends on map\n"+
					"iteration order, so the same request gets different answers", path, first, got)
			}
		}
	}
}

// The unsubscribe link in particular, because it is the one that has to work
// for somebody who is not signed in and does not want to be.
func TestUnsubscribeNeedsNoSession(t *testing.T) {
	if authed, _ := resolve(authRequired(), "/mail/unsubscribe"); authed {
		t.Error("the unsubscribe link asks for a session. Somebody who wants these\n" +
			"emails to stop is by definition somebody who does not want to sign\n" +
			"in first, and the mail arrived at an address only they read.")
	}
}
