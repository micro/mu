package inbox

import (
	"strconv"
	"strings"
	"testing"

	"mu/internal/thread"
)

// Home shows what is in the inbox.
//
// The link on its own was a word pointing at a page, so the product's whole
// claim — that the agent is reachable and answers whether or not you have this
// open — was visible only to somebody who clicked through to check. Mail that
// arrived and was answered overnight left Home looking exactly as it had the
// night before.
func TestHomeShowsWhatIsInTheInbox(t *testing.T) {
	const who = "preview-owner"

	byMail := thread.Open(who, "mail", "<invoice@example.com>")
	if byMail == nil {
		t.Fatal("could not open a conversation")
	}
	thread.Add(thread.Message{Thread: byMail.ID, Account: who, From: "bob@example.com",
		Text: "Could you send the invoice for March"})
	thread.Add(thread.Message{Thread: byMail.ID, Account: who, Role: thread.RoleAgent,
		Text: "Sent it over — the total is 420."})

	out := Preview(who)

	if !strings.Contains(out, "invoice for March") {
		t.Fatalf("the conversation is not on the page:\n%s", out)
	}
	if !strings.Contains(out, "session="+byMail.ID) && !strings.Contains(out, "id="+byMail.ID) {
		t.Error("the row does not open the conversation")
	}
	// Where it happened, because it happened without you.
	if !strings.Contains(out, "Email") {
		t.Error("a conversation that arrived by mail does not say so")
	}
	if !strings.Contains(out, "the total is 420") {
		t.Errorf("the latest message is missing:\n%s", out)
	}
	if !strings.Contains(out, `href="/inbox"`) {
		t.Error("no way through to the inbox itself")
	}
}

// An empty inbox shows nothing here. An empty list is worse than the address
// line on its own, which at least says what to do about it.
func TestAnEmptyInboxAddsNothingToHome(t *testing.T) {
	if out := Preview("nobody-has-written-to-this-account"); out != "" {
		t.Errorf("an account with no conversations gets: %s", out)
	}
	if out := Preview(""); out != "" {
		t.Errorf("a signed-out visitor gets: %s", out)
	}
}

// Home carries a few, not the inbox.
func TestHomeCarriesOnlyTheMostRecentFew(t *testing.T) {
	const who = "preview-busy-owner"

	for i := 0; i < previewShown*3; i++ {
		n := strconv.Itoa(i)
		// Mail, not the web page: Home previews what arrived, and a chat you
		// started here is on /agent — see thread.Arrived.
		th := thread.Open(who, "mail", "<chain-"+n+"@example.com>")
		if th == nil {
			t.Fatal("could not open a conversation")
		}
		thread.Add(thread.Message{Thread: th.ID, Account: who,
			Text: "conversation number " + n, From: "them@example.com"})
	}

	// Prefix, not the whole attribute: an unread row carries a second class.
	if got := strings.Count(Preview(who), `class="peek-row`); got != previewShown {
		t.Fatalf("Home shows %d conversations, want %d", got, previewShown)
	}
}

// A message keeps the newlines it was typed with. A snippet rendered with them
// in it is three short lines where the layout has room for one.
func TestASnippetIsOneLine(t *testing.T) {
	got := trimTo("Could you\n\n  send   the invoice\nfor March", 100)
	if want := "Could you send the invoice for March"; got != want {
		t.Errorf("trimTo = %q, want %q", got, want)
	}
	long := trimTo(strings.Repeat("word ", 40), 20)
	if !strings.HasSuffix(long, "…") || len([]rune(long)) > 21 {
		t.Errorf("a long message was not cut to fit: %q", long)
	}
}
