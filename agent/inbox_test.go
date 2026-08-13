package agent

// An agent that can be written to.
//
// Mu runs a real SMTP server with DKIM, which is most of why mail is a service
// here rather than a wrapper — and an agent could only be reached at one by
// knowing the plus-address convention and inventing a tag by hand. Nothing
// recorded that a tag belonged to an agent, so nothing could show it, and the
// agents page never mentioned mail at all.

import (
	"strings"
	"testing"
)

// A name becomes an address. "Morning Briefer" has a space and a capital, and
// neither survives a round trip through the mail servers between here and a
// sender.
func TestAnAgentsNameBecomesAnAddress(t *testing.T) {
	for name, want := range map[string]string{
		"Morning Briefer": "morningbriefer",
		"research":        "research",
		"Ops/Alerts!":     "opsalerts",
	} {
		if got := tagFor("asim", name, nil); got != want {
			t.Errorf("tagFor(%q) = %q, want %q", name, got, want)
		}
	}
}

// Two agents must not share an inbox: a tag is what decides whose mail is
// whose, so a collision would let one agent read another's.
func TestTwoAgentsCannotShareATag(t *testing.T) {
	existing := []*Agent{{Name: "Research", Tag: "research"}}
	got := tagFor("asim", "Research", existing)
	if got == "research" {
		t.Fatal("a second agent was given the same tag as the first")
	}
	if got != "research2" {
		t.Errorf("got %q, want research2", got)
	}

	existing = append(existing, &Agent{Name: "Research", Tag: got})
	if third := tagFor("asim", "Research", existing); third != "research3" {
		t.Errorf("third agent got %q, want research3", third)
	}
}

// A name with nothing usable in it still has to produce an address.
func TestAnUnusableNameStillGetsATag(t *testing.T) {
	if got := tagFor("asim", "!!!", nil); got == "" {
		t.Error("an agent was left with no tag, so it cannot be written to")
	}
}

// The address is owner+tag, which is what delivery resolves.
func TestTheAddressIsTheOwnersPlusAddress(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	a := &Agent{Owner: "asim", Name: "Research", Tag: "research"}
	got := a.Address()
	if !strings.HasPrefix(got, "asim+research@") {
		t.Errorf("Address() = %q, want asim+research@…", got)
	}
}

// With no mail domain it is a handle, not an address.
//
// The instance still has an inbox and still delivers between accounts — that
// needs no configuration — so an agent still has somewhere to be reached. What
// it does not have is a domain, and appending one anyway is how this page came
// to offer asim+research@localhost: an address that looks real, is presented as
// real, and reaches nobody.
func TestWithNoMailDomainAnAgentHasAHandleRatherThanAnAddress(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "")

	a := &Agent{Owner: "asim", Name: "Research", Tag: "research"}
	got := a.Address()
	if got != "asim+research" {
		t.Errorf("Address() = %q, want the bare handle asim+research", got)
	}
	if strings.Contains(got, "@") {
		t.Errorf("%q is offered as an address on an instance that cannot receive mail", got)
	}
}

// Agents made before tags existed have none, and must not be given a broken
// address — an empty tag would render as "asim+@", which delivers to nobody.
func TestAnAgentWithNoTagHasNoAddress(t *testing.T) {
	a := &Agent{Owner: "asim", Name: "Old"}
	if got := a.Address(); got != "" {
		t.Errorf("an untagged agent reported the address %q", got)
	}
	var nilAgent *Agent
	if got := nilAgent.Address(); got != "" {
		t.Errorf("a nil agent reported the address %q", got)
	}
}
