package inbox

// The two addresses at the top of the inbox, and which agent the second one is.

import (
	"strings"
	"testing"
)

// The top of the mailbox says what the list is, not who you are.
//
// It printed "You asim@micro.mu / Agent agent@micro.mu / IMAP" above every
// list. Three facts, and none of them is what somebody opening their inbox came
// to find out. What replaced them is a sentence and the two things you do from
// here.
func TestTheTopOfTheInboxSaysWhatTheListIs(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	Address = func() string { return "agent@micro.mu" }
	t.Cleanup(func() { Address = nil })

	all := addressBar("asim", "")
	if !strings.Contains(all, "Everything sent to you") {
		t.Errorf("the inbox does not say what it holds:\n%s", all)
	}
	// The identity strip is gone, and staying gone is the point of the test.
	for _, addr := range []string{"asim@micro.mu", "agent@micro.mu"} {
		if strings.Contains(all, addr) {
			t.Errorf("an address is still printed above every list: %s\n%s", addr, all)
		}
	}
	// Both controls, and only these two.
	if !strings.Contains(all, `href="/inbox/imap"`) {
		t.Errorf("no way through to a mail client:\n%s", all)
	}
	if !strings.Contains(all, `href="/inbox/new"`) {
		t.Errorf("no way to write one:\n%s", all)
	}

	// A box narrows the sentence rather than adding a second line to it.
	if box := addressBar("asim", "research"); !strings.Contains(box, "Everything sent to research") {
		t.Errorf("a box does not say which box:\n%s", box)
	}
}

// The address is where it answers a question: an empty box.
//
// Above a full mailbox it helped nobody — you are reading, not addressing. In a
// box with nothing in it the only useful thing to say is where to write so that
// there is something, and that is the behaviour the roster tests protect.
func TestAnEmptyBoxSaysWhereToWrite(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	got := boxAddress("asim", "research")
	if got != "asim+research@micro.mu" {
		t.Errorf("boxAddress = %q", got)
	}
	// And it opens compose with itself filled in, rather than being a string to
	// copy by hand into a form two clicks away.
	if link := writeTo(got); !strings.Contains(link, "/inbox/new?to=asim%2Bresearch%40micro.mu") {
		t.Errorf("the address does not open compose with itself filled in:\n%s", link)
	}
	// No box, no alias — the account's own mailbox is not an agent's.
	if got := boxAddress("asim", ""); got != "" {
		t.Errorf("the whole inbox reported an alias: %q", got)
	}
}

// With no mail domain there is no address to write to, so the line is text
// rather than a link into a form that cannot send.
func TestWithNoMailDomainTheAddressIsNotALink(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "")
	if got := writeTo("asim@localhost"); strings.Contains(got, "<a") {
		t.Errorf("an unreachable instance offered a compose link:\n%s", got)
	}
}
