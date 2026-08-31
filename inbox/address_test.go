package inbox

// The two addresses at the top of the inbox, and which agent the second one is.

import (
	"strings"
	"testing"
)

// The top of the mailbox is the two things you do from it.
//
// It printed "You asim@micro.mu / Agent agent@micro.mu / IMAP" above every
// list once. Three facts, none of them what somebody opening their inbox came
// to find out. That was cut back to a sentence — "Everything sent to you, on
// every channel" — which is the page's own title said again, read once and
// then read past on every visit afterwards. Now it is the controls alone, in
// the shape and the place every other page puts its actions.
func TestTheTopOfTheInboxIsItsActions(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	Address = func() string { return "agent@micro.mu" }
	t.Cleanup(func() { Address = nil })

	all := addressBar("asim", "")

	// The identity strip is gone, and staying gone is the point of the test.
	for _, addr := range []string{"asim@micro.mu", "agent@micro.mu"} {
		if strings.Contains(all, addr) {
			t.Errorf("an address is still printed above every list: %s\n%s", addr, all)
		}
	}
	// And so is the sentence that replaced it.
	if strings.Contains(all, "Everything sent to") {
		t.Errorf("the lede is back above every list:\n%s", all)
	}

	// One control, and only one.
	//
	// Connect a mail client sat beside New and has moved to /account. Setting
	// up IMAP is done once in the life of an account, if ever, and this is the
	// screen its owner opens every day — so the link was read past several
	// thousand times to be used never again, and two controls side by side said
	// those were the same size of decision. See account.mailClientCard, which
	// is where it went and where TestTheMailClientPageIsReachable now looks.
	if strings.Contains(all, `href="/inbox/imap"`) {
		t.Errorf("the mail-client link is back at the top of the inbox:\n%s", all)
	}
	if !strings.Contains(all, `href="/inbox/new"`) {
		t.Errorf("no way to write one:\n%s", all)
	}

	// In the standard shape, in the standard place. Two pills floated right
	// were this page's own arrangement and nowhere else's.
	if !strings.Contains(all, "page-action") {
		t.Errorf("the controls are not where every other page puts its actions:\n%s", all)
	}
	if strings.Contains(all, "pill") {
		t.Errorf("the controls are still pills:\n%s", all)
	}

	// And they are not the same size of decision. Writing one is what somebody
	// does from here; setting up a mail client is done once, if ever — so one
	// is the button and the other is a link beside it.
	if !strings.Contains(all, `class="btn"`) {
		t.Errorf("New is not the page's action:\n%s", all)
	}
	if strings.Contains(all, `href="/inbox/imap" class="btn"`) {
		t.Errorf("the mail client is a second button:\n%s", all)
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
