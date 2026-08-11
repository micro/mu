package sms

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The signature covers the URL as Twilio called it, and this process cannot see
// that URL. Getting it wrong fails closed and silent: Twilio reports 11200,
// "HTTP retrieval failure", which reads like the server is down, and every
// inbound message is dropped.
func TestSignedURLCandidates(t *testing.T) {
	setup(t)
	t.Setenv("MU_DOMAIN", "micro.mu")

	r := httptest.NewRequest(http.MethodPost, "/sms/webhook", nil)
	r.Host = "10.0.0.4:8080" // what a proxy leaves behind

	got := signedURLs(r)
	want := map[string]bool{
		"https://micro.mu/sms/webhook":      false,
		"http://micro.mu/sms/webhook":       false,
		"https://www.micro.mu/sms/webhook":  false,
		"https://10.0.0.4:8080/sms/webhook": false,
	}
	for _, u := range got {
		if _, ok := want[u]; ok {
			want[u] = true
		}
	}
	for u, seen := range want {
		if !seen {
			t.Errorf("%s is not among the candidates: %v", u, got)
		}
	}

	// The operator's word settles it, and comes first.
	t.Setenv("TWILIO_WEBHOOK_URL", "https://sms.example.com/hook/")
	if first := signedURLs(r)[0]; first != "https://sms.example.com/hook" {
		t.Errorf("first candidate = %q, want the configured URL with its trailing slash trimmed", first)
	}
}
