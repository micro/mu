package agent

// Sharing an agent shares the recipe, never the account.
//
// An app is inert HTML, so running somebody's app is just rendering it. An
// agent calls tools, and whose tools it calls is the whole question — so a
// published agent runs on the asker's account, with their credits and their
// scope, carrying only its author's standing instruction and tool list. These
// tests hold that boundary, and the money that crosses it.

import (
	"testing"
)

func publish(t *testing.T, owner, name, prompt string, price int) *Agent {
	t.Helper()
	a, _, err := CreateAgent(owner, name, Hosted, prompt, "", nil, false)
	if err != nil {
		t.Skipf("cannot create an agent in this environment: %v", err)
	}
	t.Cleanup(func() { _ = RemoveAgent(owner, a.ID) })
	if err := Publish(owner, a.ID, true, price); err != nil {
		t.Fatalf("publishing: %v", err)
	}
	return a
}

// A published agent is one a stranger can find, run, and be answered by in its
// own voice.
func TestAPublishedAgentAnswersForSomebodyElse(t *testing.T) {
	const author, stranger = "share-author", "share-stranger"
	a := publish(t, author, "Pirate", "You are a pirate. Always open with AHOY.", 0)

	found := false
	for _, p := range PublicAgents(stranger) {
		if p.ID == a.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("a published agent was not in the directory")
	}

	got := resolveAgent(stranger, a.ID, false)
	if got == nil {
		t.Fatal("a stranger could not run a published agent")
	}
	if got.SystemPrompt != "You are a pirate. Always open with AHOY." {
		t.Fatalf("the author's instruction did not travel: %q", got.SystemPrompt)
	}
}

// An unpublished agent is not reachable by knowing its id. This is the whole
// boundary: ids appear in URLs and logs, so an id must not be a capability.
func TestAPrivateAgentIsNotReachableById(t *testing.T) {
	const owner, stranger = "share-private", "share-nosy"
	a, _, err := CreateAgent(owner, "Secretive", Hosted, "Always open with ZEBRA.", "", nil, false)
	if err != nil {
		t.Skipf("cannot create an agent in this environment: %v", err)
	}
	t.Cleanup(func() { _ = RemoveAgent(owner, a.ID) })

	if got := resolveAgent(stranger, a.ID, false); got != nil {
		t.Fatalf("a stranger ran a private agent by knowing its id: %+v", got)
	}
	for _, p := range PublicAgents(stranger) {
		if p.ID == a.ID {
			t.Fatal("a private agent was listed in the directory")
		}
	}

	// Withdrawing works the same way round.
	if err := Publish(owner, a.ID, true, 0); err != nil {
		t.Fatal(err)
	}
	if resolveAgent(stranger, a.ID, false) == nil {
		t.Fatal("publishing did not make it reachable")
	}
	if err := Publish(owner, a.ID, false, 0); err != nil {
		t.Fatal(err)
	}
	if got := resolveAgent(stranger, a.ID, false); got != nil {
		t.Fatal("withdrawing did not make it unreachable again")
	}
}

// The asker pays the author, and the author does not pay themselves.
func TestRunningAPaidAgentPaysItsAuthor(t *testing.T) {
	const author, asker = "share-seller", "share-buyer"
	a := publish(t, author, "Priced", "You are expensive.", 5)

	var paidBy, paidTo string
	var paid int
	ChargeForRun = func(from, to string, price int) error {
		paidBy, paidTo, paid = from, to, price
		return nil
	}
	t.Cleanup(func() { ChargeForRun = nil })

	if got := resolveAgent(asker, a.ID, false); got == nil {
		t.Fatal("a paid agent could not be run")
	}
	if paidBy != asker || paidTo != author || paid != 5 {
		t.Fatalf("wrong charge: %s paid %s %d credits", paidBy, paidTo, paid)
	}

	// The author running their own is not a sale.
	paidBy, paidTo, paid = "", "", 0
	if got := resolveAgent(author, a.ID, false); got == nil {
		t.Fatal("the author could not run their own agent")
	}
	if paid != 0 {
		t.Fatalf("the author was charged %d credits for their own agent", paid)
	}

	// A refused payment refuses the run, rather than running it for free.
	ChargeForRun = func(string, string, int) error { return errTestBroke }
	if got := resolveAgent(asker, a.ID, false); got != nil {
		t.Fatal("a paid agent ran without being paid for")
	}
}

var errTestBroke = testErr("insufficient credits")

type testErr string

func (e testErr) Error() string { return string(e) }

// Free agents can be copied; paid ones cannot, because the prompt is the whole
// product and a copy button would be a way around the price.
func TestOnlyFreeAgentsCanBeCopied(t *testing.T) {
	const author, copier = "share-giver", "share-copier"
	free := publish(t, author, "Freebie", "You are free to copy.", 0)
	paid := publish(t, author, "Costly", "You are not.", 3)

	got, err := Fork(copier, free.ID)
	if err != nil {
		t.Fatalf("a free agent could not be copied: %v", err)
	}
	t.Cleanup(func() { _ = RemoveAgent(copier, got.ID) })
	if got.Owner != copier {
		t.Fatalf("the copy belongs to %s, not the person who made it", got.Owner)
	}
	if got.Prompt != free.Prompt {
		t.Fatal("the copy did not carry the instruction")
	}
	if got.ForkedOf != free.ID {
		t.Fatal("the copy does not record what it came from")
	}
	// A copy is yours alone until you say otherwise.
	if got.Public {
		t.Fatal("copying something published it")
	}
	if got.TokenID != "" {
		t.Fatal("copying minted a credential nobody asked for")
	}

	if _, err := Fork(copier, paid.ID); err == nil {
		t.Fatal("a paid agent was copied, which is a way around its price")
	}
	if _, err := Fork(author, free.ID); err == nil {
		t.Fatal("an author copied their own agent")
	}
}

// Publishing an agent with no instruction would offer a name and deliver the
// default assistant.
func TestAnEmptyAgentCannotBePublished(t *testing.T) {
	const owner = "share-empty"
	a, _, err := CreateAgent(owner, "Hollow", Hosted, "", "", nil, false)
	if err != nil {
		t.Skipf("cannot create an agent in this environment: %v", err)
	}
	t.Cleanup(func() { _ = RemoveAgent(owner, a.ID) })
	if err := Publish(owner, a.ID, true, 0); err == nil {
		t.Fatal("an agent with no system prompt was published")
	}
}
