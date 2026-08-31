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
	// And no join date. It was the first fact the page said about somebody and
	// the least useful: when an account was made says nothing about the person,
	// changes nothing you would do next, and on an instance a year old says the
	// same thing about nearly everybody. It was there because a profile felt
	// like it ought to have a fact under the name.
	if strings.Contains(body, "Joined ") {
		t.Errorf("the profile is back to leading with a join date:\n%s", body)
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

// The profile lists conversations. It does not render one.
//
// It used to print the newest in full at the top with the rest listed under it,
// which made the page two things: a profile for one line and a conversation for
// the rest of the screen. The answer to "who is @henrik" was buried under the
// answer to "what did we last say" — and that second question already has a
// page, /inbox?id=, with the reply box and the read-marking built for it.
//
// It also declared one channel the real one. Somebody who writes by mail and by
// WhatsApp has two conversations, and rendering the newest whole made that one
// important on the grounds that it happened most recently.
func TestTheProfileListsConversationsRatherThanRenderingOne(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := person(t, "lister", "listed")

	// Two conversations, on two channels, so neither is obviously the one.
	// Named for the other person, because thread.With is what the page uses to
	// find them and it matches on how the record names a correspondent.
	for _, c := range []struct{ client, text string }{
		{"mail", "the invoice is attached"},
		{"whatsapp", "running ten minutes late"},
	} {
		// Keyed on the other person: thread.With matches the thread's own key as
		// well as its party list, which is how a conversation from before Parties
		// existed still turns up on somebody's page.
		said(t, "lister", c.client, "listed", "", c.text)
	}

	r := httptest.NewRequest(http.MethodGet, "/@listed", nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	PersonHandler(w, r)
	body := w.Body.String()

	if w.Code != http.StatusOK {
		t.Fatalf("the page did not render: %d", w.Code)
	}
	// Both are listed, and each is a link to the one page that reads a
	// conversation.
	if n := strings.Count(body, `href="/inbox?id=`); n < 2 {
		t.Errorf("the page links to %d conversations, want both:\n%s", n, body)
	}
	// And neither is rendered here. Checked on the reader's own markup rather
	// than on the message text, because the text is also the subject and a
	// subject belongs in a list.
	for _, markup := range []string{`class="ib-conv"`, `class="ib-msg`, `class="ib-reply"`} {
		if strings.Contains(body, markup) {
			t.Errorf("the profile still embeds the conversation reader (%s is on it); "+
				"/inbox?id= is the page that reads one:\n%s", markup, body)
		}
	}
}

// Your own profile says where people reach you.
//
// Your addresses were on /account, behind a click, in a section about
// configuration — and they are not configuration. They are your identity here,
// which is what a profile is for. Before this, /@you was your name, your handle
// and a link to settings: three lines, none of them a fact you did not know.
func TestYourOwnProfileNamesYourAddresses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := person(t, "addressed", "addressed")

	r := httptest.NewRequest(http.MethodGet, "/@addressed", nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	PersonHandler(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "Where people reach you") {
		t.Errorf("your own profile does not say where people reach you:\n%s", body)
	}
	// The handle, which is how somebody here reaches you.
	if !strings.Contains(body, "<code>@addressed</code>") {
		t.Errorf("your own profile does not name your handle:\n%s", body)
	}
	// And the mail address, which is how anybody else does. Resolved the same
	// way the To box resolves it, so the two agree by construction.
	if addr, ok := addressOfPerson("@addressed"); ok && !strings.Contains(body, addr) {
		t.Errorf("your own profile does not name %s, the address anybody outside "+
			"this instance would use:\n%s", addr, body)
	}
}
