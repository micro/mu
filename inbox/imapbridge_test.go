package inbox

import (
	"mu/internal/thread"
	"testing"
)

func TestIMAPBridgeExcludesLocalConversations(t *testing.T) {
	const owner = "imap_bridge_local_test"
	defer thread.Forget(owner)
	for _, client := range []string{thread.WebClient, thread.CLIClient, "sms"} {
		th := thread.Open(owner, client, "bridge-test-"+client)
		thread.Add(thread.Message{Thread: th.ID, Account: owner, Role: thread.RoleAgent, Text: "**Answer** from " + client})
	}
	messages := Bridge(owner)
	if len(messages) != 1 || messages[0].Body != "**Answer** from sms" || !messages[0].Markdown {
		t.Fatalf("unexpected bridge: %+v", messages)
	}
}
