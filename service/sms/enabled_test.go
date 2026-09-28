package sms

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"mu/internal/settings"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWhatsAppExplicitDisable(t *testing.T) {
	t.Setenv("TWILIO_WHATSAPP_FROM", "+447700900123")
	for _, value := range []string{"", "true", "0", "false", " FALSE "} {
		t.Setenv("WHATSAPP_ENABLED", value)
		disabled := strings.EqualFold(strings.TrimSpace(value), "false")
		if WhatsAppEnabled() == disabled || (len(SendersFor(ChannelWhatsApp)) == 0) != disabled {
			t.Fatalf("value %q", value)
		}
		if settings.Get("TWILIO_WHATSAPP_FROM") != "+447700900123" {
			t.Fatal("sender was removed")
		}
		if disabled {
			if _, err := SendOn(ChannelWhatsApp, "owner", "+447700900456", "hello"); err == nil || !strings.Contains(err.Error(), "disabled") {
				t.Fatal("send not disabled")
			}
			if _, err := sendForm(ChannelWhatsApp, "+447700900456", "hello"); err == nil {
				t.Fatal("provider send form still available")
			}
		}
	}
}

func TestDisabledWhatsAppWebhookIsAcknowledgedWithoutReply(t *testing.T) {
	t.Setenv("WHATSAPP_ENABLED", "false")
	token := "test-auth-token"
	t.Setenv("TWILIO_AUTH_TOKEN", token)
	address := "https://micro.test/whatsapp/twilio"
	t.Setenv("TWILIO_WEBHOOK_URL", address)
	form := url.Values{"Body": {"help"}, "From": {"whatsapp:+447700900456"}}
	r := httptest.NewRequest("POST", address, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mac := hmac.New(sha1.New, []byte(token))
	mac.Write([]byte(address + "BodyhelpFromwhatsapp:+447700900456"))
	r.Header.Set("X-Twilio-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	w := httptest.NewRecorder()
	WebhookHandler(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<Message>") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
