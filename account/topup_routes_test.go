package account

import (
	"mu/internal/auth"
	"mu/internal/settings"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTopupDoesNotOfferUnusableCryptoFunding(t *testing.T) {
	t.Setenv("X402_PAY_TO", "")
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "")
	for _, key := range []string{"X402_PAY_TO", "STRIPE_SECRET_KEY", "STRIPE_PUBLISHABLE_KEY"} {
		old := settings.Get(key)
		settings.Set(key, "")
		t.Cleanup(func() { settings.Set(key, old) })
	}
	acc := &auth.Account{ID: "topup_unconfigured", Secret: "fixture"}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(session.Token)
	for _, payTo := range []string{"", "0x1111111111111111111111111111111111111111"} {
		t.Setenv("X402_PAY_TO", payTo)
		t.Setenv("X402_NETWORK", "unsupported-network")
		for _, path := range []string{"/account/topup", "/wallet/topup"} {
			r := httptest.NewRequest("GET", path+"?error=Conversion+failed+%3Cscript%3E", nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
			w := httptest.NewRecorder()
			BalanceHandler(w, r)
			body := w.Body.String()
			if !strings.Contains(body, "Conversion failed &lt;script&gt;") || strings.Contains(body, "Conversion failed <script>") {
				t.Fatalf("funding error missing or unescaped on %s", path)
			}
			if !strings.Contains(body, "No payment methods available") || strings.Contains(body, "cw-qrnote") {
				t.Fatalf("unusable funding offered on %s", path)
			}
		}
	}
}

func TestAccountShowsConversionConfirmation(t *testing.T) {
	cookie := holder(t, "convert-notice", "Conversion")
	req := httptest.NewRequest("GET", "/account?saved=converted", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Account(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "USDC converted to account credits.") {
		t.Fatalf("conversion confirmation missing: status %d", rec.Code)
	}
}

func TestTopupShowsBalanceAndPaymentNoteInTheFormCard(t *testing.T) {
	got := renderStripeDeposit("topup-display-empty", "")
	balance := strings.Index(got, "Current balance")
	divider := strings.Index(got, "<hr")
	button := strings.Index(got, "Continue to Payment")
	note := strings.Index(got, "Secure payment via Stripe")
	if balance < 0 || balance > divider || !strings.Contains(got, "0 credits") {
		t.Fatal("current balance must appear above the top-up divider")
	}
	if button < 0 || note < button || strings.Count(got, `class="card"`) != 1 {
		t.Fatal("payment note must follow the button inside the same card")
	}
}
