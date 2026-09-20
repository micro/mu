package mail

import (
	"mu/internal/app"
	"mu/internal/auth"
	"testing"
	"time"
)

func TestSelfMailDoesNotNotifyOrForward(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	const owner = "notification_owner"
	if err := auth.Create(&auth.Account{ID: owner, Email: "owner@example.net", EmailVerified: true, Addresses: []string{"alias@example.net"}, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	old := app.EmailSender
	defer func() { app.EmailSender = old }()
	app.EmailSender = func(string, string, string, string, string) error { t.Fatal("self mail forwarded"); return nil }
	for _, from := range []string{owner, "  NOTIFICATION_OWNER  ", owner + "@example.test", "owner@example.net", "ALIAS@example.net"} {
		m := InboundMail{Owner: owner, From: from, Subject: "own submission"}
		if !FromOwner(m) {
			t.Errorf("not recognized: %s", from)
		}
		forward(m)
	}
	for _, m := range []InboundMail{{Owner: owner, From: "agent@example.test"}, {Owner: owner, From: "other@example.net"}, {Owner: "other", From: "owner@example.net"}, {Owner: owner}} {
		if FromOwner(m) {
			t.Errorf("wrongly suppressed: %+v", m)
		}
	}
}
