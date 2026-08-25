package api

// Pinning a service says which it means, and leaves you where you were.
//
// Two bugs in one control. It redirected everyone to /services however they
// arrived, so pinning from /tools?view=services moved you to a different URL
// in the middle of what you were doing. And the form posted one field meaning
// "flip this", so the request was not idempotent: two tabs on the same page
// disagree the moment one of them acts, and pressing pin in each leaves the
// service unpinned.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestThePinSaysWhichItMeans(t *testing.T) {
	r := httptest.NewRequest("GET", "/tools?view=services", nil)

	unpinned := pinControl(withSession(t, r), "news", false)
	if !strings.Contains(unpinned, `name="pin" value="news"`) {
		t.Errorf("an unpinned service does not offer to pin:\n%s", unpinned)
	}
	pinned := pinControl(withSession(t, r), "news", true)
	if !strings.Contains(pinned, `name="unpin" value="news"`) {
		t.Errorf("a pinned service does not offer to unpin — it posts a flip, so two "+
			"tabs undo each other:\n%s", pinned)
	}
	// And it carries the page it was pressed on.
	for _, got := range []string{unpinned, pinned} {
		if !strings.Contains(got, `name="return" value="/tools?view=services"`) {
			t.Errorf("the pin does not carry where it was pressed:\n%s", got)
		}
	}
}

// Pressing pin twice leaves it pinned, which is what makes the request safe to
// send from two places.
func TestPinningIsIdempotent(t *testing.T) {
	acc := &auth.Account{ID: "pinner"}

	acc.Pin("news")
	acc.Pin("news")
	if got := acc.PinnedServices(); len(got) != 1 || got[0] != "news" {
		t.Errorf("pinning twice gave %v, want it pinned once", got)
	}

	acc.Unpin("news")
	acc.Unpin("news")
	if got := acc.PinnedServices(); len(got) != 0 {
		t.Errorf("unpinning twice gave %v, want nothing pinned", got)
	}

	// Order is the reader's and survives an unrelated change.
	acc.Pin("mail")
	acc.Pin("news")
	acc.Pin("weather")
	acc.Unpin("news")
	if got := strings.Join(acc.PinnedServices(), ","); got != "mail,weather" {
		t.Errorf("unpinning reordered the rest: %q", got)
	}
}

// withSession returns the request with a signed-in cookie on it. pinControl
// renders nothing for a guest, which is correct and not what these check.
func withSession(t *testing.T, r *http.Request) *http.Request {
	t.Helper()
	const id = "pin_tester"
	if _, err := auth.GetAccount(id); err != nil {
		if err := auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(id)
	if err != nil {
		t.Fatal(err)
	}
	out := r.Clone(r.Context())
	out.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return out
}
