package sms

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verification is two steps on the page, and the page shows the one you are on.
//
// It was one form with two boxes and a single button, and which thing it did
// depended on whether the second box was empty — so anything that put a value
// in a field named "code", a browser's autofill included, answered "ask for a
// code first" to somebody asking for a code. It also lived in a <details> that
// collapsed on reload, so the second step was hidden behind a triangle.
func TestVerificationIsTwoSteps(t *testing.T) {
	setup(t)
	const me, mine = "acct-1", "+447700900123"
	r := httptest.NewRequest(http.MethodGet, "/sms", nil)

	first := verifier(r, me, "csrf")
	if !strings.Contains(first, `name="start"`) {
		t.Error("step one does not ask for a number")
	}
	if strings.Contains(first, `name="code"`) {
		t.Error("step one asks for a code nobody has been sent")
	}
	if !strings.Contains(first, " open") {
		t.Error("an account with nothing verified should not have this behind a closed triangle")
	}

	send = func(to, body string) (string, error) { return "SM1", nil }
	t.Cleanup(func() { send = deliver })
	if err := StartVerify(me, mine); err != nil {
		t.Fatal(err)
	}

	second := verifier(r, me, "csrf")
	if !strings.Contains(second, `name="code"`) {
		t.Error("step two does not ask for the code")
	}
	if !strings.Contains(second, `name="confirm"`) {
		t.Error("step two has no way to confirm")
	}
	if !strings.Contains(second, mine) {
		t.Error("step two does not say which number the code went to")
	}
	if !strings.Contains(second, " open") {
		t.Error("the step you are on is hidden behind a closed triangle")
	}
}
