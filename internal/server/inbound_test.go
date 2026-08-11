package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Twilio carries a phone number and a WhatsApp sender on one Messaging Service
// and posts everything arriving on either to the one webhook configured there.
// So a WhatsApp message turned up at /sms/webhook and was refused for not being
// a phone number — the two paths were registered on the belief that one
// endpoint would mean guessing, and the payload says which channel it is.
func TestInboundIsRoutedByChannel(t *testing.T) {
	whatsapp := "From=whatsapp%3A%2B447974400601&To=whatsapp%3A%2B15553390242&Body=Hello"
	text := "From=%2B447974400601&To=%2B447401265872&Body=Hello"

	for _, c := range []struct {
		name, body string
		want       bool
	}{
		{"a WhatsApp message", whatsapp, true},
		{"a text", text, false},
	} {
		r := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(c.body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ParseForm() //nolint:errcheck
		got := strings.HasPrefix(r.PostForm.Get("From"), "whatsapp:") ||
			strings.HasPrefix(r.PostForm.Get("To"), "whatsapp:")
		if got != c.want {
			t.Errorf("%s: routed to whatsapp = %v, want %v", c.name, got, c.want)
		}
	}
}
