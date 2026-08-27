package inbox

// The subject is said once.
//
// Mail was recorded as "Subject\n\nbody", because prepending it was the only
// way a conversation learned what it was about — thread.Add derives a subject
// from the first line of the first message. So reading a thread meant seeing
// the subject as the heading, then again at the top of every message under it,
// and in the preview on the list before you opened it. On a chain of six it is
// on the screen seven times.
//
// thread.Name is how a client says what a conversation is about now. The
// subject belongs to the conversation; a message is what somebody wrote.

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

// A message recorded before thread.Name existed still has the subject in it,
// and a reader of an old conversation should not see it twice.
func TestAnOldMessageDoesNotRepeatTheSubject(t *testing.T) {
	for what, tc := range map[string]struct{ text, subject, want string }{
		"the subject, then the body": {
			"Invoice 4021\n\nAttached is this month's invoice.",
			"Invoice 4021", "Attached is this month's invoice."},
		"a reply marker on it": {
			"Re: Invoice 4021\n\nThanks.", "Invoice 4021", "Thanks."},
		"a forward marker": {
			"Fwd: Invoice 4021\n\nSee below.", "Invoice 4021", "See below."},
		"different case": {
			"invoice 4021\n\nThanks.", "Invoice 4021", "Thanks."},
	} {
		if got := withoutSubject(tc.text, tc.subject); got != tc.want {
			t.Errorf("%s: got %q, want %q", what, got, tc.want)
		}
	}
}

// Only an exact first line, or it deletes what somebody wrote.
func TestOnlyAWholeRepeatedSubjectIsCut(t *testing.T) {
	for what, tc := range map[string]struct{ text, subject string }{
		"a line that starts the same way": {
			"Invoice 4021 has not arrived\n\nCan you resend?", "Invoice 4021"},
		"the subject further down": {
			"Hello\n\nInvoice 4021\n\nthanks", "Invoice 4021"},
		"a one-line message that is the subject": {
			"Invoice 4021", "Invoice 4021"},
		"no subject at all": {
			"Invoice 4021\n\nthanks", ""},
	} {
		if got := withoutSubject(tc.text, tc.subject); got != tc.text {
			t.Errorf("%s: %q was cut to %q", what, tc.text, got)
		}
	}
}

// The whole conversation says it once: the heading, and nowhere else.
func TestAConversationSaysItsSubjectOnce(t *testing.T) {
	const who = "subject_once"
	th := thread.Open(who, mailClient, "<inv@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Name(who, th.ID, "Invoice 4021")
	// Two messages recorded the old way, with the subject inside them.
	thread.Add(thread.Message{Thread: th.ID, Account: who, From: "them@example.com",
		Text: "Invoice 4021\n\nAttached.", Ref: "<inv@example.com>"})
	thread.Add(thread.Message{Thread: th.ID, Account: who,
		Text: "Re: Invoice 4021\n\nGot it, thanks."})

	got := thread.Get(who, th.ID)
	if got == nil {
		t.Fatal("the conversation is gone")
	}
	page := ConversationView(who, got)
	if n := strings.Count(page, "Invoice 4021"); n != 1 {
		t.Errorf("the subject appears %d times on the page, want once as the heading:\n%s", n, page)
	}
}

// thread.Name is what a mail client uses to say so, and the first name wins:
// "Re: Re: Lunch" on message nine is not a rename.
func TestAConversationKeepsTheNameItWasGiven(t *testing.T) {
	const who = "subject_named"
	th := thread.Open(who, mailClient, "<n@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Name(who, th.ID, "Lunch")
	thread.Name(who, th.ID, "Re: Re: Lunch")

	if got := thread.Get(who, th.ID); got == nil || got.Subject != "Lunch" {
		t.Errorf("the conversation is called %q", got.Subject)
	}
}

// A conversation nobody named still gets one from what was said, which is what
// every non-mail client relies on.
func TestAnUnnamedConversationIsStillNamed(t *testing.T) {
	const who = "subject_derived"
	th := thread.Open(who, thread.WebClient, "web_subject")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who, Text: "what is the weather"})

	if got := thread.Get(who, th.ID); got == nil || got.Subject != "what is the weather" {
		t.Errorf("an unnamed conversation is called %q", got.Subject)
	}
}
