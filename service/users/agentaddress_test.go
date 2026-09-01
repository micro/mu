package users

// An agent has no address here.
//
// The directory gave every account id@domain, so on micro.mu the agent's
// profile read:
//
//	Name: micro
//	Address: micro@micro.mu
//
// Reported as a stutter, which is how it got noticed, and the stutter is the
// smaller half. micro@ is not where you write to the agent — agent@ is, and
// agent+<name>@ for a particular one — so this was a published address that
// nothing routes, in a field documented as "Where to write to them".
//
// The right answer for the agent's real address is service/mail, which this
// package may not import: a service does not reach sideways. It does not need
// to. The Connect page already publishes agent@, which is the page for it, and
// an address that cannot receive is worse than none — the same reason this
// returns nothing on an instance with no mail domain rather than
// name@localhost.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAnAgentIsNotGivenAnAddress(t *testing.T) {
	const bot, person = "addragent", "addrperson"
	auth.Create(&auth.Account{ID: bot, Name: bot, Agent: true}) //nolint:errcheck
	auth.Create(&auth.Account{ID: person, Name: person})        //nolint:errcheck

	got, ok := Get(bot)
	if !ok {
		t.Fatalf("@%s is not in the directory", bot)
	}
	if got.Address != "" {
		t.Errorf("the agent is published at %q — a local part nothing routes, in "+
			"the field that says where to write to somebody", got.Address)
	}
	if !got.Account.Agent {
		t.Error("the agent is not marked as one, which is the fact this turns on")
	}

	// And a person still gets theirs, when the instance has a domain. Without
	// one nobody does, which is the older rule and is not what this changes.
	who, ok := Get(person)
	if !ok {
		t.Fatalf("@%s is not in the directory", person)
	}
	if d := mailDomain(); d != "" {
		if want := person + "@" + d; who.Address != want {
			t.Errorf("@%s is published at %q, want %q — dropping the agent's "+
				"address must not drop everybody's", person, who.Address, want)
		}
	} else if who.Address != "" {
		t.Errorf("no mail domain and @%s is still published at %q", person, who.Address)
	}
}

// The rule is about agents, not about the name "micro".
//
// The id is one instance of it, and acc.Agent is the rule — the same line the
// strip on Home and the room roster both turn on. Anything else that gets an
// account for the same reason is covered without a second edit.
func TestTheRuleIsAgentNotAName(t *testing.T) {
	const other = "addrsecondbot"
	auth.Create(&auth.Account{ID: other, Name: other, Agent: true}) //nolint:errcheck

	got, ok := Get(other)
	if !ok {
		t.Fatalf("@%s is not in the directory", other)
	}
	if got.Address != "" {
		t.Errorf("@%s is an agent and is published at %q — the rule is keyed on "+
			"the name rather than on what the account is", other, got.Address)
	}
	if strings.Contains(got.Profile.Page, "@@") {
		t.Errorf("the page path is malformed: %q", got.Profile.Page)
	}
}
