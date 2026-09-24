package mail

import (
	svc "mu/service/mail"
	"testing"
)

func TestOnlyAuthenticatedOwnerInstructionsStartAgent(t *testing.T) {
	base := svc.InboundMail{Owner: "owner", From: "owner@example.com", To: "agent@example.net", Shared: true}
	for _, tc := range []struct {
		name                          string
		authenticated, owned, machine bool
		tag                           string
		shared, want                  bool
	}{
		{"owner", true, true, false, "", true, true},
		{"spoofed", false, true, false, "", true, false},
		{"stranger", true, false, false, "", true, false},
		{"machine", true, true, true, "", true, false},
		{"tagged", true, true, false, "receipts", true, false},
		{"ordinary mail", true, true, false, "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.Tag = tc.tag
			m.Shared = tc.shared
			got := acceptedInstruction(m, map[string]interface{}{"authenticated": tc.authenticated, "owned": tc.owned, "machine": tc.machine})
			if got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestLocalAssistantMailCannotStartReplyLoop(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	for _, from := range []string{"hello@example.test", "agent@example.test", "agent+news@example.test"} {
		m := svc.InboundMail{Owner: "owner", From: from, To: "owner@example.test", Shared: true}
		if acceptedInstruction(m, map[string]interface{}{"authenticated": true, "owned": true}) {
			t.Fatalf("loop admitted: %s", from)
		}
	}
}
