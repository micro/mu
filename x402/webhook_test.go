package x402

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStripeWebhookDoesNotRequireBrowserLogin(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_test")
	body := `{"id":"evt_route","type":"test.unhandled","data":{"object":{}}}`
	for _, signed := range []bool{false, true} {
		r := httptest.NewRequest("POST", "http://m3o.test/stripe/webhook", strings.NewReader(body))
		if signed {
			stamp := time.Now().Unix()
			mac := hmac.New(sha256.New, []byte("whsec_test"))
			fmt.Fprintf(mac, "%d.%s", stamp, body)
			r.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%x", stamp, mac.Sum(nil)))
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		want := 400
		if signed {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("signed=%v: %d %s", signed, w.Code, w.Body.String())
		}
	}
}
