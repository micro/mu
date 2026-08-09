package mail

// The guards on waking an agent from inbound mail.
//
// The hook itself needs the agent and the roster, so it is wired in main.go.
// What lives here is the decision to call it at all, and the cases where
// calling it would be wrong: spam, our own outbound, a plus address nobody
// claimed, a sender who is not who they say, and a stranger who is. Each of
// the first three is a loop or a bill — an agent replying to its own reply
// costs a model call per turn, forever. The last two are worse: an agent runs
// with its owner's scope and tools, so whoever can wake it can act as them.
//
// This file used to carry its own copy of the rule and check the copy. The
// rule now lives in inbound_agent.go and this tests that.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

// wake stands the world up around shouldWakeAgent: a hook to call, a domain to
// compare against, and an owner whose verified address is asim@aslam.me.
func wake(t *testing.T, from string, isSpam, authenticated bool, known ...string) bool {
	t.Helper()
	withDomain(t, "micro.mu")

	prevHook, prevKnown := InboundAgent, KnownSender
	InboundAgent = func(InboundMail) {}
	KnownSender = func(_, addr string) bool {
		for _, k := range known {
			if strings.EqualFold(k, addr) {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { InboundAgent, KnownSender = prevHook, prevKnown })

	return shouldWakeAgent("wakeowner", "research", from, isSpam, authenticated)
}

func wakeOwner(t *testing.T) {
	t.Helper()
	if _, err := auth.GetAccount("wakeowner"); err == nil {
		return
	}
	err := auth.Create(&auth.Account{
		ID: "wakeowner", Name: "wakeowner", Secret: "s",
		Email: "asim@aslam.me", EmailVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWhenInboundMailWakesAnAgent(t *testing.T) {
	wakeOwner(t)

	// The owner, writing to their own agent. The case the whole feature exists
	// for, and it has to keep working.
	if !wake(t, "asim@aslam.me", false, true) {
		t.Error("the account holder cannot wake their own agent from their own verified address")
	}
	// Somebody in the address book. Deliberate: you decided they could reach
	// you, and an agent answering your contacts is the point of giving it an
	// address at all.
	if !wake(t, "colleague@example.com", false, true, "colleague@example.com") {
		t.Error("a contact cannot reach an agent, so the address is only useful to its owner")
	}

	// A stranger who knows the address. This is the hole: the tag was the only
	// thing standing between anyone and an agent holding the owner's tools.
	if wake(t, "stranger@example.com", false, true) {
		t.Error("a stranger who guessed the tag can drive the agent and spend the owner's credits")
	}
	// A known address on unauthenticated mail. From headers are free to write,
	// so without SPF or DKIM "from your contact" means nothing.
	if wake(t, "colleague@example.com", false, false, "colleague@example.com") {
		t.Error("an unauthenticated From header is enough to impersonate a contact")
	}
	if wake(t, "asim@aslam.me", false, false) {
		t.Error("the owner's address can be forged into a wake-up")
	}

	// The guards that were already here.
	if wake(t, "asim@aslam.me", true, true) {
		t.Error("spam wakes an agent")
	}
	if wake(t, "agent@micro.mu", false, true) {
		t.Error("our own reply wakes an agent, which is an unbounded exchange with ourselves")
	}
	if wake(t, "Agent@Micro.MU", false, true) {
		t.Error("the loop guard is case-sensitive, so a differently-cased reply loops")
	}
}

// Either signal is enough. Requiring both would drop more real mail than it
// stops; requiring neither is where this started.
func TestSPFAloneAndDKIMAloneBothCount(t *testing.T) {
	wakeOwner(t)
	if !wake(t, "asim@aslam.me", false, true) {
		t.Error("a single passing signal is not accepted")
	}
}

// With no address book wired, the owner is still the owner. An instance that
// forgets to set the hook must fail closed for strangers, not open.
func TestWithNoAddressBookOnlyTheOwnerGetsThrough(t *testing.T) {
	wakeOwner(t)
	withDomain(t, "micro.mu")

	prevHook, prevKnown := InboundAgent, KnownSender
	InboundAgent = func(InboundMail) {}
	KnownSender = nil
	defer func() { InboundAgent, KnownSender = prevHook, prevKnown }()

	if !shouldWakeAgent("wakeowner", "research", "asim@aslam.me", false, true) {
		t.Error("the owner cannot reach their agent when no address book is wired")
	}
	if shouldWakeAgent("wakeowner", "research", "stranger@example.com", false, true) {
		t.Error("an unwired address book lets everybody in")
	}
}

func TestNoTagAndNoHookNeverWake(t *testing.T) {
	wakeOwner(t)
	withDomain(t, "micro.mu")

	prev := InboundAgent
	InboundAgent = func(InboundMail) {}
	defer func() { InboundAgent = prev }()

	if shouldWakeAgent("wakeowner", "", "asim@aslam.me", false, true) {
		t.Error("untagged mail wakes an agent, so every newsletter in the inbox starts a run")
	}
	InboundAgent = nil
	if shouldWakeAgent("wakeowner", "research", "asim@aslam.me", false, true) {
		t.Error("an instance with no agent configured still tries to wake one")
	}
}

// The rule has to stay wired into the path, and the path has to keep storing
// the message first, so a failure to answer never loses mail.
func TestTheGuardIsStillInThePath(t *testing.T) {
	src := readSource(t, "smtp.go")
	if !strings.Contains(src, "if shouldWakeAgent(toAcc.ID, toTag, fromAddr.Address, spamResult.IsSpam, dkimPass || s.spfPass)") {
		t.Error("the inbound-agent guard is no longer the one in inbound_agent.go, " +
			"or has stopped being given the authentication result")
	}

	store := strings.Index(src, "if err := SendMessageTo(")
	woken := strings.Index(src, "if shouldWakeAgent(")
	if store < 0 || woken < 0 || woken < store {
		t.Error("the agent is woken before the message is stored, so a panic there loses the mail")
	}

	// SPF and DKIM are computed for the spam score. If either stops being
	// worked out, the argument above is a constant and the gate is off.
	rule := readSource(t, "inbound_agent.go")
	for _, want := range []string{"authenticated", "senderKnownTo", "SenderIsAccountOwner"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the rule lost %q", want)
		}
	}
	if _, err := os.Stat("inbound_agent.go"); err != nil {
		t.Fatal(err)
	}
}
