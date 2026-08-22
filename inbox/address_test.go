package inbox

// The two addresses at the top of the inbox, and which agent the second one is.

import (
	"strings"
	"testing"
)

// A box is an agent, so the address above it is that agent's.
//
// It showed the instance agent on every box. So the switcher changed which
// mail you were looking at and the line above it went on naming a different
// agent — and that line is the one somebody copies when they want to write to
// the agent whose mail they are reading.
func TestTheAgentAddressFollowsTheBox(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	Address = func() string { return "agent@micro.mu" }
	t.Cleanup(func() { Address = nil })

	all := addressBar("asim", "")
	if !strings.Contains(all, "asim@micro.mu") {
		t.Errorf("the account's own address is missing:\n%s", all)
	}
	if !strings.Contains(all, "agent@micro.mu") {
		t.Errorf("All does not name the instance agent:\n%s", all)
	}

	box := addressBar("asim", "research")
	if !strings.Contains(box, "asim+research@micro.mu") {
		t.Errorf("the research box does not name its own alias:\n%s", box)
	}
	if strings.Contains(box, "agent@micro.mu") {
		t.Errorf("the research box still names the instance agent:\n%s", box)
	}
	// And yours does not move, because it is the same mailbox either way.
	if !strings.Contains(box, "asim@micro.mu") {
		t.Errorf("the account's own address went missing on a box:\n%s", box)
	}
}

// The agent's address is a way to write to it. "Write to the agent" is what
// somebody reading that line is trying to do, and the alternative was copying
// it by hand into a form two clicks away.
func TestTheAgentAddressOpensCompose(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	Address = func() string { return "agent@micro.mu" }
	t.Cleanup(func() { Address = nil })

	got := addressBar("asim", "research")
	if !strings.Contains(got, "/inbox/compose?to=asim%2Bresearch%40micro.mu") {
		t.Errorf("the address does not open compose with itself filled in:\n%s", got)
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
