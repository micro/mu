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
	"mu/internal/thread"
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

// The page says whose it is, with the handle under the name.
//
// It said "Your conversation with @x", which described the page rather than
// the person. That was right when the page was one exchange and wrong now that
// it is a profile: on your own page it would have had to say you were in
// correspondence with yourself, and on somebody else's it labelled the whole
// thing with the relationship before saying anything about them. The handle is
// what belongs under a display name, because the display name is the
// changeable one and the handle is the address.
func TestThePersonPageSaysWhoseItIs(t *testing.T) {
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
	if !strings.Contains(body, `ib-person-sub">@micro`) {
		t.Errorf("the page never says whose it is:\n%s", body)
	}
	// And since when, which is what turns a handle into somebody with a
	// history here.
	if !strings.Contains(body, "Joined ") {
		t.Errorf("the profile does not say since when:\n%s", body)
	}
}

// Your own page is a page, not a redirect to your inbox.
//
// It redirected, on the argument that mail to your own address is just mail so
// this would be everything you have ever been sent. True while the page was a
// correspondence; it is a profile now, so /@you is the one identity on this
// instance you could not look at. Clicking your own name in /users and landing
// in your inbox is the report that found it.
func TestYourOwnPageIsAPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := person(t, "selfreader", "selfreader")

	r := httptest.NewRequest(http.MethodGet, "/@selfreader", nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	PersonHandler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("your own page redirects to %q instead of rendering",
			w.Header().Get("Location"))
	}
	body := w.Body.String()
	if !strings.Contains(body, `ib-person-sub">@selfreader`) {
		t.Errorf("your own page does not name you:\n%s", body)
	}
	// No way to write to yourself, and no correspondence: your correspondence
	// is the inbox and this is not a second one.
	if strings.Contains(body, "Send a message") {
		t.Error("your own page offers to send you a message")
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
	// "Send mail", not "Send a message" and not "Start a conversation". The
	// first two name neither door: mail and chat are different promises — one
	// answered when they get to it, the other answered now — and the label has
	// to say which this is. It is also a button, not a sentence.
	if !strings.Contains(body, "Send mail") {
		t.Errorf("no way to write to them, or it does not say which door:\n%s", body)
	}
	// And it is still their page: it says whose, and where the button goes.
	//
	// The handle, not "Your conversation with @stranger". That line described
	// the page rather than the person — right when the page was one exchange,
	// and wrong now that it is a profile: it labelled the whole thing with the
	// relationship before saying anything about them.
	if !strings.Contains(body, `ib-person-sub">@stranger`) {
		t.Error("the empty page does not say whose it is")
	}
	if !strings.Contains(body, "to=%40stranger") {
		t.Error("Start a conversation does not address them")
	}
}

// A conversation you started can be carried on from the inbox.
//
// Reported: start one from somebody's page, open it in the inbox, and there is
// no Reply — only "assign to an agent". The thread you had just written was the
// one thread you could not write to again.
//
// replyTo walked the messages backwards for an author who is not you, which is
// right for a conversation that arrived and wrong for one you began: it has
// exactly one author, so the loop found nothing. /@somebody looked fine on the
// same thread because that page states the reply target instead of inferring
// it, which is what made this look like an inbox problem rather than a shared
// one.
func TestAConversationYouStartedCanBeRepliedToInTheInbox(t *testing.T) {
	th := &thread.Thread{
		ID:      "started-by-me",
		Client:  mailClient,
		Subject: "Hello",
		Parties: []thread.Party{
			{Kind: thread.RolePerson, Key: "them@example.test"},
		},
	}
	// One message, from the account, because nobody has answered yet.
	msgs := []thread.Message{
		{ID: "m1", Thread: th.ID, Account: "me", Role: thread.RolePerson,
			Text: "Are you around?", To: "them@example.test"},
	}

	if got := replyTo("me", th, msgs); got != "them@example.test" {
		t.Errorf("replyTo = %q, want them@example.test — with nobody to answer, "+
			"the conversation cannot be continued", got)
	}
}

// And a thread that did arrive is still answered to whoever spoke last, not to
// the first party on the list. The party fallback must not outrank the
// messages: three people on a thread are answered to the one who just wrote.
func TestAnArrivedConversationStillAnswersWhoeverSpokeLast(t *testing.T) {
	th := &thread.Thread{
		ID:     "arrived",
		Client: mailClient,
		Parties: []thread.Party{
			{Kind: thread.RolePerson, Key: "first@example.test"},
			{Kind: thread.RolePerson, Key: "second@example.test"},
		},
	}
	msgs := []thread.Message{
		{ID: "m1", Role: thread.RolePerson, From: "first@example.test"},
		{ID: "m2", Role: thread.RolePerson, From: "second@example.test"},
	}
	if got := replyTo("me", th, msgs); got != "second@example.test" {
		t.Errorf("replyTo = %q, want second@example.test (whoever spoke last)", got)
	}
}
