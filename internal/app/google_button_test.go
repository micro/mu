package app

// Sign in with Google is on the pages you sign in from.
//
// It was injected by string-replacing the literal `<h1>Signup</h1>` in the
// template. Rewriting that heading — which happened when these pages were
// given the landing page's copy — made the replace match nothing, and the
// button vanished from both /signup and /login. Nothing failed: the function
// returned the string unchanged, the page rendered fine, and the only way to
// find out was for somebody to go looking for the button.
//
// It is a template slot now, so it cannot miss. These tests hold that: the
// slot exists, it is filled when Google is configured, and it is empty rather
// than broken when it is not.

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

func withGoogle(t *testing.T) {
	t.Helper()
	settings.Set("GOOGLE_CLIENT_ID", "test-client-id")
	settings.Set("GOOGLE_CLIENT_SECRET", "test-client-secret")
	t.Cleanup(func() {
		settings.Set("GOOGLE_CLIENT_ID", "")
		settings.Set("GOOGLE_CLIENT_SECRET", "")
	})
}

func TestSignupAndLoginOfferGoogleWhenItIsConfigured(t *testing.T) {
	withGoogle(t)
	if !GoogleConfigured() {
		t.Fatal("test setup did not configure Google")
	}

	for name, page := range map[string]string{
		"signup": renderSignup(""),
		"login":  loginPage("", ""),
	} {
		if !strings.Contains(page, `href="/oauth2/google"`) {
			t.Errorf("%s does not offer Google sign-in", name)
		}
		if !strings.Contains(strings.ToLower(page), "with google") {
			t.Errorf("%s has the link but no label naming Google", name)
		}
	}
}

// Nothing to sign in with means no button — not a dead one.
func TestNoGoogleButtonWhenItIsNotConfigured(t *testing.T) {
	settings.Set("GOOGLE_CLIENT_ID", "")
	settings.Set("GOOGLE_CLIENT_SECRET", "")

	for name, page := range map[string]string{
		"signup": renderSignup(""),
		"login":  loginPage("", ""),
	} {
		if strings.Contains(page, "/oauth2/google") {
			t.Errorf("%s offers Google sign-in on an instance that has not configured it", name)
		}
		// The slot is still there and still renders a complete page.
		if !strings.Contains(page, "</form>") {
			t.Errorf("%s did not render a form", name)
		}
		if strings.Contains(page, "%!s(") || strings.Contains(page, "%!(EXTRA") {
			t.Errorf("%s has an unfilled template slot:\n%s", name, page)
		}
	}
}
