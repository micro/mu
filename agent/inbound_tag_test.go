package agent

// Mail addressed to an agent has to find the agent.
//
// Every agent has an address — you+<tag>@ — and writing to one used to file a
// message in the owner's inbox and do nothing else. Answering it starts here:
// turning the part after the plus back into the agent that answers on it.
//
// The lookup has to be quiet about tags that are not agents. A plus address is
// an ordinary mail feature — you+receipts@, you+newsletters@ — and those must
// keep filing rather than waking something up or erroring.

import "testing"

func TestATagResolvesToTheAgentThatAnswersOnIt(t *testing.T) {
	acc := owner(t, "tag_lookup")

	made, _, err := CreateAgent(acc, "Research", Hosted, "You research things", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if made.Tag == "" {
		t.Fatal("a new agent got no mail tag, so nothing can be written to it")
	}

	got := ForTag(acc, made.Tag)
	if got == nil || got.ID != made.ID {
		t.Fatalf("the agent's own tag %q did not resolve back to it", made.Tag)
	}
	// Mail headers arrive in whatever case the sender used.
	if got := ForTag(acc, upper(made.Tag)); got == nil || got.ID != made.ID {
		t.Error("an upper-case tag did not resolve")
	}
}

// A plus address that is not an agent is an ordinary plus address.
func TestATagThatIsNotAnAgentWakesNothing(t *testing.T) {
	acc := owner(t, "tag_quiet")
	if _, _, err := CreateAgent(acc, "Research", Hosted, "You research things", "", nil, false); err != nil {
		t.Fatal(err)
	}

	for _, tag := range []string{"receipts", "newsletters", "", "   "} {
		if got := ForTag(acc, tag); got != nil {
			t.Errorf("you+%s@ resolved to the agent %q — plain tagged mail must just file", tag, got.Name)
		}
	}
	// And an agent belongs to one account: somebody else's tag is not yours.
	other := owner(t, "tag_other")
	if got := ForTag(other, "research"); got != nil {
		t.Error("one account's tag resolved on another account")
	}
	if got := ForTag("", "research"); got != nil {
		t.Error("an empty owner matched an agent")
	}
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}
