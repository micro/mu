package tool

// mail_send stays account-only, wherever it is declared.
//
// It moved: it was a hand-written registration in internal/api and is now
// derived from the mail Spec, which is exactly the move that could have
// dropped the flag. The test moved with it, because this is the package that
// builds the thing being asserted about.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/api"
)

// withPayer makes this call look like one from a settled x402 payment.
func withPayer(t *testing.T, addr string) {
	t.Helper()
	restore := api.WalletPayer
	api.WalletPayer = func(r *http.Request) string { return addr }
	t.Cleanup(func() { api.WalletPayer = restore })
}

// mail_send stays account-only: paying proves funds, not accountability, and an
// anonymous funded wallet sending from this domain spends its reputation.
func TestMailSendRefusesAPaidWallet(t *testing.T) {
	registerScopedMail(t)
	DeriveTools()
	withPayer(t, "0xdeadbeef00000000000000000000000000001234")

	var mailSend *api.Tool
	for _, reg := range api.Tools() {
		if reg.Name == "mail_send" {
			found := reg
			mailSend = &found
		}
	}
	if mailSend == nil {
		t.Fatal("mail_send is gone; the guard needs rechecking")
	}
	if !mailSend.AccountOnly {
		t.Fatal("mail_send is no longer account-only — a funded wallet could send from this domain")
	}
	_ = mailSend

	text, isErr, err := api.ExecuteTool(httptest.NewRequest("POST", "/mcp", nil), "mail_send",
		map[string]any{"to": "someone@example.com", "subject": "x", "body": "y"})
	if err == nil || !isErr {
		t.Fatalf("a paid wallet was allowed to send mail: %q", text)
	}
	if !strings.Contains(text, "requires an account") {
		t.Errorf("refused for the wrong reason: %q", text)
	}
}
