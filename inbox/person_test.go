package inbox

// The page says what it is, and exists even when it is empty.
//
// Both reported from a live instance, by somebody who clicked a username on the
// blog: they landed on an exchange with no label and could not tell what the
// page was, and a person they had never written to bounced them to /inbox/new
// with nothing saying they had been moved.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

// person creates two accounts and a session for the viewer.
func person(t *testing.T, viewer, other string) *http.Cookie {
	t.Helper()
	for _, id := range []string{viewer, other} {
		if _, err := auth.GetAccount(id); err != nil {
			if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
				t.Fatalf("could not create %s: %v", id, err)
			}
		}
	}
	sess, err := auth.CreateSession(viewer)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

func TestThePersonPageSaysWhoseConversationItIs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := person(t, "reader", "micro")

	r := httptest.NewRequest(http.MethodGet, "/@micro", nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	PersonHandler(w, r)

	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("the page did not render: %d %s", w.Code, w.Header().Get("Location"))
	}
	if !strings.Contains(body, "Your conversation with @micro") {
		t.Errorf("the page never says what it is:\n%s", body)
	}
}

// And a person you have never written to gets their page, not a redirect to the
// compose screen. The address bar saying /inbox/new after clicking @micro is a
// page you did not ask for, with nothing explaining where the one you did ask
// for went.
func TestSomebodyYouHaveNeverWrittenToStillHasAPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := person(t, "reader2", "stranger")

	r := httptest.NewRequest(http.MethodGet, "/@stranger", nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	PersonHandler(w, r)

	if w.Code == http.StatusSeeOther || w.Code == http.StatusFound {
		t.Fatalf("an empty conversation redirects to %q instead of rendering",
			w.Header().Get("Location"))
	}
	body := w.Body.String()
	if !strings.Contains(body, "Start a conversation") {
		t.Errorf("no way to start one:\n%s", body)
	}
	// And it is still their page: it says whose, and where the button goes.
	if !strings.Contains(body, "Your conversation with @stranger") {
		t.Error("the empty page does not say whose it is")
	}
	if !strings.Contains(body, "to=%40stranger") {
		t.Error("Start a conversation does not address them")
	}
}
