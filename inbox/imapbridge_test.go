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

func TestCheckinMailAnswerIsNotBridgedBeforeOrAfterDelivery(t *testing.T) {
	const owner = "checkin-mail-answer"
	defer thread.Forget(owner)
	th := thread.Open(owner, thread.WebClient, "checkin:mail-answer")
	from := "agent@" + mail.ConfiguredDomain()
	ref := "<checkin-answer@test>"
	answer := thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "One answer", From: from, Ref: ref, Workflow: "mail-run"}
	id := thread.Add(answer)
	if got := Bridge(owner); len(got) != 0 {
		t.Fatalf("mail answer bridged before delivery: %+v", got)
	}
	if err := mail.SendMessageTo(mail.Delivery{FromID: from, ToID: owner, Subject: "Re: Daily Checkin", Body: "One answer", MessageID: ref}); err != nil {
		t.Fatal(err)
	}
	// The independent arrival consumer may run before the mail responder returns.
	if added := thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RolePerson, Text: "One answer", From: from, Ref: ref, Workflow: "mail-run"}); added != id {
		t.Fatal("delivery recorded a second conversation turn")
	}
	if got := Bridge(owner); len(got) != 0 {
		t.Fatalf("mail answer bridged after delivery: %+v", got)
	}
	// Old answers whose delivery raced the late reference update are suppressed too.
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "Older mail answer", From: from, Workflow: "older-mail-run"})
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "One answer"})
	got := Bridge(owner)
	if len(got) != 1 || got[0].Body != "One answer" || got[0].InReplyTo != ref {
		t.Fatalf("lost genuine web answer or mail references: %+v", got)
	}
}
