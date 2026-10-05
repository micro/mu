package agent

import (
	"mu/internal/thread"
	"mu/service/mail"
	"testing"
)

func TestCheckinConversationUsesOwnedOrigin(t *testing.T) {
	owner := "checkin_ack_owner"
	web := thread.Open(owner, thread.WebClient, "checkin:original")
	if !checkinConversation(owner, web.ID) {
		t.Fatal("web check-in not recognised")
	}
	if checkinConversation("other", web.ID) {
		t.Fatal("foreign check-in accepted")
	}
	normal := thread.Open(owner, thread.WebClient, "ordinary")
	thread.Name(owner, normal.ID, "Daily Checkin")
	if checkinConversation(owner, normal.ID) {
		t.Fatal("title treated as check-in identity")
	}
	mailThread := thread.Open(owner, "mail", "checkin-mail")
	thread.Add(thread.Message{Account: owner, Thread: mailThread.ID, From: "agent@" + mail.ConfiguredDomain(), To: owner + "+checkin@example.test", Text: "Morning. How's it going?"})
	if !checkinConversation(owner, mailThread.ID) {
		t.Fatal("mail check-in not recognised")
	}
}
