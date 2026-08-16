package mail

// Mail you sent yourself is not mail waiting to be read.
//
// agent@ resolves to whoever wrote to it, so a question put to the agent is
// filed in the asker's own inbox. Left unread it became a count on the home
// page offering to deal with four emails that were four questions you had
// asked — and the agent, whose context includes that count, reported on the
// inbox the question was sitting in.

import "testing"

func TestSelfAddressedMailIsRecognised(t *testing.T) {
	// Local delivery records the sender as an account id.
	if !selfAddressed(&Message{FromID: "asim", ToID: "asim"}) {
		t.Error("a message from an account to itself is not recognised as self-sent")
	}
	// Ordinary correspondence is not.
	if selfAddressed(&Message{FromID: "someone@example.com", ToID: "asim"}) {
		t.Error("a stranger's message was treated as self-sent, so it would be " +
			"silently marked read and never seen")
	}
	if selfAddressed(&Message{FromID: "", ToID: "asim"}) {
		t.Error("a message with no sender was treated as self-sent")
	}
}

func TestMarkingSelfSentReadLeavesRealMailAlone(t *testing.T) {
	mutex.Lock()
	saved := messages
	messages = []*Message{
		{ID: "1", FromID: "asim", ToID: "asim", Read: false},
		{ID: "2", FromID: "stranger@example.com", ToID: "asim", Read: false},
		{ID: "3", FromID: "asim", ToID: "asim", Read: true},
	}
	mine := messages
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		messages = saved
		mutex.Unlock()
	})

	markSelfSentRead()

	if !mine[0].Read {
		t.Error("a message sent to oneself is still unread")
	}
	if mine[1].Read {
		t.Error("a stranger's unread message was marked read — this pass must never " +
			"touch correspondence somebody has not seen")
	}
}
