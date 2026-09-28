package client

import (
	"strings"
	"testing"
)

func TestDisabledWhatsAppNotAdvertised(t *testing.T) {
	t.Setenv("TWILIO_ACCOUNT_SID", "ACtest")
	t.Setenv("TWILIO_AUTH_TOKEN", "test-token")
	t.Setenv("TWILIO_WHATSAPP_FROM", "+447700900123")
	t.Setenv("WHATSAPP_ENABLED", "true")
	has := func() bool {
		for _, c := range All() {
			if c.ID == "whatsapp" {
				return true
			}
		}
		return false
	}
	if !has() {
		t.Fatal("enabled channel not shown")
	}
	t.Setenv("WHATSAPP_ENABLED", "false")
	if has() {
		t.Fatal("disabled channel advertised")
	}
	if strings.Contains(VCard("Micro"), "447700900123") {
		t.Fatal("disabled sender in contact card")
	}
}
