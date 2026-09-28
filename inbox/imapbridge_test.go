package inbox

import (
	"mu/internal/thread"
	"mu/service/mail"
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

func TestCheckinBridgeSkipsNativeMailAndKeepsReferences(t *testing.T) {
	owner := "checkin-bridge-native"
	defer thread.Forget(owner)
	th := thread.Open(owner, thread.WebClient, "checkin:native")
	ref := "<checkin-native@test>"
	if err := mail.SendMessageTo(mail.Delivery{FromID: "agent@test", ToID: owner, Subject: "Daily Checkin", Body: "Initial", MessageID: ref}); err != nil {
		t.Fatal(err)
	}
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "Initial", Ref: ref})
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RolePerson, Text: "Web reply"})
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "Web answer"})
	got := Bridge(owner)
	if len(got) != 2 || got[0].InReplyTo != ref || got[1].InReplyTo != got[0].MessageID || got[0].Subject != "Daily Checkin" {
		t.Fatalf("bad bridge: %+v", got)
	}
}
