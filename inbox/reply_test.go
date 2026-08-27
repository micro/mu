package inbox

// An inbox you cannot answer from is not an inbox.
//
// Reading a message here, the two controls were "← Inbox" and a box captioned
// "This is not a reply". Somebody who wanted to turn a sender down politely had
// no way to do it and no way to find out there was no way, so they typed the
// instruction into the only box on the page and pressed the only button — twice,
// because a POST that redirects to an identical page looks like a press that did
// not register.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/thread"
)

// A mail conversation offers a reply, addressed to whoever wrote in.
func TestAMailConversationCanBeAnswered(t *testing.T) {
	const who = "reply_reader"
	th := thread.Open(who, mailClient, "reply_1")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who, Text: "Are you interested?",
		From: "henrik@example.com", To: who + "@example.org"})

	page := ConversationView(who, th)

	if !strings.Contains(page, ">Reply</a>") {
		t.Fatal("a mail conversation has no Reply — the only thing on the page is a " +
			"box that says it is not a reply")
	}
	// The site's own button, not one drawn here. mu.css has a global
	// `a:visited { color: #000 }`, so a hand-made black pill is legible until
	// you use it and black-on-black afterwards — which is what happened. a.btn
	// carries color:#fff !important precisely because this was fixed once.
	if !strings.Contains(page, `class="btn"`) {
		t.Error("Reply is not an a.btn, so it is a second button that has to " +
			"re-learn everything the first one already knows")
	}
	// Addressed to the sender, on this conversation, with the subject carried.
	for _, want := range []string{"to=henrik%40example.com", "on=" + th.ID} {
		if !strings.Contains(page, want) {
			t.Errorf("the reply link is missing %q — it has to arrive filled in, or "+
				"it is a link to a blank compose form", want)
		}
	}
	// And the old "answer it where it arrived" note is gone, because now it can
	// be answered here.
	if strings.Contains(page, "so a reply carries on there") {
		t.Error("the page still says to go and answer it somewhere else")
	}
}

// One Re:, however many times the subject has been round.
func TestASubjectIsNotReprefixed(t *testing.T) {
	const who = "reply_prefix"
	th := thread.Open(who, mailClient, "reply_2")
	if th == nil {
		t.Fatal("no conversation")
	}
	th.Subject = "Re: Mu has a listing"
	if got := actionBar(th, "henrik@example.com", true); strings.Contains(got, "Re%3A+Re%3A") {
		t.Errorf("the subject picked up a second Re: — %s", got)
	}
}

// Anything that did not arrive by mail still points at where it did.
//
// A Discord thread is answered in Discord. Offering a Reply that sends an email
// to somebody who wrote on another network is a worse answer than no button.
func TestAConversationFromElsewhereStillPointsThere(t *testing.T) {
	const who = "reply_discord"
	th := thread.Open(who, thread.WebClient, "reply_3")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who, Text: "hello", From: "someone"})

	page := ConversationView(who, th)
	if strings.Contains(page, "/inbox/new?") {
		t.Error("a conversation that did not arrive by mail is offering to send an email")
	}
}

// Somebody else's conversation id is not a conversation to file a message onto.
//
// The id travels in a form field, so it is whatever the sender says it is. It is
// checked on the way in and again at the point of writing — replyTarget is the
// second look, and it is the one that matters, because it is the call that would
// otherwise put your message on a stranger's thread.
func TestAReplyCannotBeFiledOntoSomebodyElsesThread(t *testing.T) {
	const mine, theirs = "reply_mine", "reply_theirs"
	th := thread.Open(theirs, mailClient, "reply_4")
	if th == nil {
		t.Fatal("no conversation")
	}
	if got := replyTarget(mine, form{On: th.ID}); got != nil {
		t.Error("a conversation belonging to another account was accepted as the " +
			"thread to reply on")
	}
	if got := replyTarget(theirs, form{On: th.ID}); got == nil {
		t.Error("the owner cannot reply on their own conversation, so this check " +
			"is refusing everything")
	}
}

// The handover cannot be fired twice by pressing it twice.
//
// Making the task is quick, but it redirects to a page that looks identical —
// so the only feedback that the press registered has to come from the button,
// and without it the second press makes a second task.
func TestHandingOverSaysItRegistered(t *testing.T) {
	r := httptest.NewRequest("GET", "/inbox?id=x", nil)

	box := assignDialog(r, "dlg_x", &thread.Thread{ID: "x"}, "henrik@example.com")
	if !strings.Contains(box, "disabled=true") {
		t.Error("it can be pressed again, which is two tasks for one instruction")
	}
	if !strings.Contains(box, "Assigned") {
		t.Error("nothing on the page says the press registered")
	}
	// And with a reply available, the caption points at it rather than only
	// saying what this box is not.
	if !strings.Contains(box, "Reply above") {
		t.Error("the caption still only says this is not a reply, on a page that " +
			"now has one")
	}
}
