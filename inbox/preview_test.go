package inbox

import (
	"strconv"
	"strings"
	"testing"

	"mu/service/mail"
)

// deliver puts a message in an account's mailbox and returns it.
func deliver(t *testing.T, to, from, subject, tag string) *mail.Message {
	t.Helper()
	if err := mail.SendMessageTo(from, "", to, to, tag, subject,
		"the body of "+subject, "", "", false, 0, nil, "", "", nil); err != nil {
		t.Skipf("cannot deliver in this environment: %v", err)
	}
	msgs := mail.ListMessages(to, 200)
	for _, m := range msgs {
		if m.Subject == subject {
			return m
		}
	}
	t.Fatalf("delivered %q but it is not in the mailbox", subject)
	return nil
}

// Home shows what is in the inbox, and the inbox is mail.
//
// The block used to list agent conversations under a heading naming a mail
// address — the same confusion the page itself had. Mail that arrived and was
// answered overnight left Home looking exactly as it had the night before,
// which is the one thing this block exists to prevent.
func TestHomeShowsWhatArrived(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "preview-owner"
	m := deliver(t, who, "bob@example.com", "Invoice for March", "")

	out := Preview(who)

	if !strings.Contains(out, "Invoice for March") {
		t.Fatalf("the message is not on the page:\n%s", out)
	}
	if !strings.Contains(out, "id="+m.ID) {
		t.Error("the row does not open the message")
	}
	if !strings.Contains(out, "bob@example.com") {
		t.Errorf("the sender is missing:\n%s", out)
	}
	if !strings.Contains(out, `href="/inbox"`) {
		t.Error("no way through to the inbox itself")
	}
	// Unread is the fact worth carrying: it is why you would look.
	if !strings.Contains(out, "peek-new") {
		t.Error("an unopened message is not marked as one")
	}
}

// An empty inbox shows nothing here. An empty list is worse than the address
// line on its own, which at least says what to do about it.
func TestAnEmptyInboxAddsNothingToHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if out := Preview("nobody-has-written-to-this-account"); out != "" {
		t.Errorf("an account with no mail gets: %s", out)
	}
	if out := Preview(""); out != "" {
		t.Errorf("a signed-out visitor gets: %s", out)
	}
}

// Home carries a few, not the inbox.
func TestHomeCarriesOnlyTheMostRecentFew(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "preview-busy-owner"
	for i := 0; i < previewShown*3; i++ {
		deliver(t, who, "sender@example.com", "Message "+strconv.Itoa(i), "")
	}

	if got := strings.Count(Preview(who), `class="peek-row`); got != previewShown {
		t.Fatalf("Home shows %d messages, want %d", got, previewShown)
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
