package mail

import "testing"

// TestSearchScopedToAccount verifies mail.Search only ever returns messages
// belonging to the requesting account, and never spam.
//
// The fixtures are put in the index as well as the store, because search reads
// the index now — and a message written straight into `messages` is exactly the
// case that is invisible to it. That is worth knowing rather than working
// around: anything that adds mail without indexing it cannot be found, which is
// why both write paths index and why Reindex runs at boot.
func TestSearchScopedToAccount(t *testing.T) {
	indexed(t)
	mutex.Lock()
	setMessages([]*Message{
		{ID: "m1", From: "acme", FromID: "ext-acme", To: "alice", ToID: "alice", Subject: "Invoice", Body: "your bitcoin invoice is ready"},
		{ID: "m2", From: "bobcorp", FromID: "ext-bob", To: "bob", ToID: "bob", Subject: "Bitcoin update", Body: "bob's private bitcoin note"},
		{ID: "m3", From: "spammer", FromID: "ext-spam", To: "alice", ToID: "alice", Subject: "Bitcoin riches", Body: "claim your bitcoin now", Spam: true},
	})
	mutex.Unlock()
	Reindex()

	alice := Search("alice", "bitcoin", 10)
	if len(alice) != 1 || alice[0].ID != "m1" {
		t.Fatalf("alice should see only m1, got %v", idsOf(alice))
	}

	bob := Search("bob", "bitcoin", 10)
	if len(bob) != 1 || bob[0].ID != "m2" {
		t.Fatalf("bob should see only m2, got %v", idsOf(bob))
	}

	// A different account sees none of the above.
	if other := Search("carol", "bitcoin", 10); len(other) != 0 {
		t.Fatalf("carol should see nothing, got %v", idsOf(other))
	}
}

func idsOf(ms []*Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}
