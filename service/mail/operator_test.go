package mail

import (
	"mu/internal/auth"
	"strings"
	"testing"
)

func TestOperatorAliasesAndDelivery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "example.test")
	for _, test := range []struct{ local, tag string }{{"admin", ""}, {"admin+dmarc", "dmarc"}, {"support", "support"}, {"abuse+report", "abuse+report"}, {"postmaster", "postmaster"}, {"security", "security"}} {
		owner, tag := operatorMailbox(test.local)
		if owner != auth.OperatorID || tag != test.tag {
			t.Fatal(test.local, owner, tag)
		}
		s := &Session{}
		if err := s.Rcpt(test.local+"@example.test", nil); err != nil {
			t.Fatal(err)
		}
	}
	if owner, _ := operatorMailbox("asim+dmarc"); owner != "" {
		t.Fatal("personal address captured")
	}
	old := messages
	defer func() { setMessages(old); rebuildInboxes() }()
	body := "From: reporter@google.com\r\nTo: admin+dmarc@example.test\r\nSubject: Report\r\nMessage-ID: <operator-test@google.com>\r\nContent-Type: text/plain\r\n\r\nReport contents"
	s := &Session{from: "reporter@google.com", to: []string{"admin+dmarc@example.test"}, isLocalhost: true}
	if err := s.Data(strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	actor := "operator-test-admin"
	auth.SetAccountForTest(&auth.Account{ID: actor, Admin: true})
	defer auth.RemoveAccountForTest(actor)
	got, err := OperatorMessages(actor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range got {
		if m.MessageID == "<operator-test@google.com>" {
			found = true
			if m.ToID != auth.OperatorID || m.Tag != "dmarc" || m.Arrival != nil {
				t.Fatalf("unsafe routing %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("missing operator mail")
	}
	if _, err := OperatorMessages("ordinary-user"); err == nil {
		t.Fatal("ordinary user read shared mail")
	}
	if _, err := OperatorMessages(auth.OperatorID); err == nil {
		t.Fatal("system identity used as an administrator")
	}
}
