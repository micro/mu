package account

// The things about an account that could not be changed.
//
// A display name is set once at signup, optionally, and then shown on every
// post, in mail, in invites and on the profile — with no form anywhere that
// edits it. A verified address showed with a tick and a full stop: no way to
// move it, and no sight of the others the account had proved. Both are the same
// mistake, which is that a page called Account was a page you read.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

func holder(t *testing.T, id, name string) *http.Cookie {
	t.Helper()
	auth.Create(&auth.Account{ID: id, Name: name, Secret: "s"}) //nolint:errcheck
	if _, err := auth.GetAccount(id); err != nil {
		// A username the rules refuse is a bug in this test, not a fact about
		// the machine it runs on. Skipping on it is how eighteen tests across
		// this repository quietly stopped running the day usernames became
		// validated — a red suite would have said so on the first push.
		if strings.Contains(err.Error(), "username") {
			t.Fatalf("the test account name is not a valid username: %v", err)
		}
		t.Skipf("cannot create an account here: %v", err)
	}
	t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	sess, err := auth.CreateSession(id)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

func post(t *testing.T, cookie *http.Cookie, form url.Values) {
	t.Helper()
	r := httptest.NewRequest("POST", "/account", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	Account(httptest.NewRecorder(), r)
}

func TestTheDisplayNameCanBeChanged(t *testing.T) {
	const id = "acct-name"
	c := holder(t, id, "Old Name")

	post(t, c, url.Values{"save_name": {"1"}, "display_name": {"New Name"}})

	acc, _ := auth.GetAccount(id)
	if acc.Name != "New Name" {
		t.Errorf("name is %q, want New Name", acc.Name)
	}
	// The username is not the display name and must not follow it.
	if acc.ID != id {
		t.Errorf("changing the display name renamed the account to %q", acc.ID)
	}
}

// Clearing it falls back to the username rather than leaving an account with no
// name at all, which is what an account that never set one already shows.
func TestClearingTheNameFallsBackToTheUsername(t *testing.T) {
	const id = "acct-noname"
	c := holder(t, id, "Something")

	post(t, c, url.Values{"save_name": {"1"}, "display_name": {"   "}})

	if acc, _ := auth.GetAccount(id); acc.Name != id {
		t.Errorf("name is %q, want the username %q", acc.Name, id)
	}
}

// An address proved by code can be given up. The one the account signs in with
// cannot be dropped from here — it is what a password reset goes to, and it is
// changed by verifying a different one.
func TestAProvedAddressCanBeRemovedAndTheSignInOneCannot(t *testing.T) {
	const id = "acct-address"
	c := holder(t, id, "Somebody")

	acc, _ := auth.GetAccount(id)
	acc.Email, acc.EmailVerified = "me@example.com", true
	auth.UpdateAccount(acc) //nolint:errcheck
	if err := auth.AddVerifiedAddress(id, "work@example.com"); err != nil {
		t.Fatalf("adding: %v", err)
	}

	post(t, c, url.Values{"forget_address": {"work@example.com"}})
	if auth.OwnsAddress(id, "work@example.com") {
		t.Error("a proved address survived being removed")
	}

	post(t, c, url.Values{"forget_address": {"me@example.com"}})
	if !auth.OwnsAddress(id, "me@example.com") {
		t.Error("the sign-in address was removed from the wrong place")
	}
}

// Logging out must be a real page load. A soft navigation swaps #content and
// leaves the sidebar drawn for somebody who is no longer signed in, so it reads
// as nothing having happened.
func TestLoggingOutIsNotSoftNavigated(t *testing.T) {
	src, err := os.ReadFile("../internal/app/app.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `u.pathname === '/logout'`) {
		t.Error("the soft-navigation interceptor does not exclude /logout, so " +
			"logging out swaps the content and leaves the signed-in chrome up")
	}
}
