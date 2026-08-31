package agent

import (
	"testing"

	"mu/internal/thread"
)

// A conversation is adopted once, or not at all if it is already here.
//
// Ask opens a thread under a freshly minted key, and Record used to mint a
// second, different id for the workflow record of the same turn. Nothing tied
// them together — so adoptAll, which walks the flows at start-up looking for
// conversations the rail has not got and asks for a thread keyed by the root
// flow's id, found none and made one. Every restart, another copy.
//
// It only bit the clients that go through Ask: the CLI, the API, mail, SMS.
// The web keys its thread on the flow id it already holds, which is why the
// same conversation was fine from a browser and doubled from a terminal.
//
// Both halves are checked, because only the pair says what the invariant is
// for: keyed by its record, adopting leaves it alone; keyed by anything else,
// adopting makes a second one, which is the bug reproduced on purpose.
func TestAdoptingDoesNotDuplicateAConversationKeyedByItsRecord(t *testing.T) {
	const account = "adopt-test-account"

	agreed := Record(Recorded{
		Account: account, Source: thread.WebClient,
		Prompt: "keyed by its own record", Answer: "yes",
	})
	if thread.Open(account, thread.WebClient, agreed) == nil {
		t.Fatal("could not open the conversation")
	}

	before := len(thread.List(account, 0))
	adoptAll()
	if after := len(thread.List(account, 0)); after != before {
		t.Errorf("adopting made %d conversation(s) out of one that was already "+
			"here — the thread is keyed by its own record and should have been "+
			"found, not copied", after-before)
	}

	// And the shape that was broken: a record whose conversation is keyed by
	// something else is invisible to the lookup, so it gets adopted again.
	orphan := Record(Recorded{
		Account: account, Source: thread.WebClient,
		Prompt: "keyed by something else", Answer: "yes",
	})
	if thread.Open(account, thread.WebClient, orphan+"-not-the-record") == nil {
		t.Fatal("could not open the second conversation")
	}
	before = len(thread.List(account, 0))
	adoptAll()
	if after := len(thread.List(account, 0)); after == before {
		t.Skip("adopting no longer keys on the root record; if that is deliberate " +
			"this test has outlived the invariant it was guarding")
	}
}
