package mail

import (
	"strings"
	"testing"
)

func TestHelloUsesSharedAddressRules(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	if !sharedRecipient("hello@example.test") || !sharedMailbox("HELLO") || !fromSharedAgent("hello@example.test") {
		t.Fatal("hello not recognized")
	}
	if sharedRecipient("hello+other@example.test") || sharedRecipient("hello@elsewhere.test") {
		t.Fatal("intro alias too broad")
	}
}

func TestHelloSMTPQueuesOnlyAuthenticatedHumanMail(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	old := Introduction
	defer func() { Introduction = old }()
	calls := 0
	Introduction = func(email, id, subject, text string) error {
		calls++
		if email != "stranger@example.net" || id != "<hello-test@example.net>" || !strings.Contains(text, "A question") {
			t.Fatal("wrong introduction")
		}
		return nil
	}
	message := "From: stranger@example.net\r\nTo: hello@example.test\r\nSubject: Hello\r\nMessage-ID: <hello-test@example.net>\r\nContent-Type: text/plain\r\n\r\nA question"
	s := &Session{from: "stranger@example.net", to: []string{"hello@example.test"}, spfPass: true, isLocalhost: true}
	if err := s.Data(strings.NewReader(message)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("introduction not queued")
	}
	s.spfPass = false
	if err := s.Data(strings.NewReader(message)); err == nil {
		t.Fatal("unauthenticated message accepted")
	}
	s.spfPass = true
	if err := s.Data(strings.NewReader("Auto-Submitted: auto-replied\r\n" + message)); err == nil {
		t.Fatal("automatic reply accepted")
	}
	if err := s.Data(strings.NewReader("Cc: other@example.net\r\n" + message)); err == nil {
		t.Fatal("group mail accepted for introduction")
	}
	if calls != 1 {
		t.Fatal("unsafe mail queued")
	}
}
