package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withPayer(t *testing.T, addr string) {
	t.Helper()
	restore := WalletPayer
	WalletPayer = func(r *http.Request) string { return addr }
	t.Cleanup(func() { WalletPayer = restore })
}

// A paid wallet is an identity. Before this, an agent that had settled a
// payment was still told "Authentication required", so every account-scoped
// tool was unreachable to exactly the callers the MCP endpoint exists for.
func TestPaidWalletIsAnIdentity(t *testing.T) {
	withPayer(t, "0xABCdef0000000000000000000000000000001234")

	got, err := callerIdentity(httptest.NewRequest("POST", "/mcp", nil))
	if err != nil {
		t.Fatalf("a settled payer was refused: %v", err)
	}
	if got != "x402:0xABCdef0000000000000000000000000000001234" {
		t.Fatalf("identity = %q, want the namespaced wallet", got)
	}
	if !IsWalletIdentity(got) {
		t.Error("a wallet identity was not recognised as one")
	}
}

// No session and no payment is still nobody.
func TestNoPaymentIsStillAnonymous(t *testing.T) {
	withPayer(t, "")
	if got, err := callerIdentity(httptest.NewRequest("POST", "/mcp", nil)); err == nil {
		t.Fatalf("an anonymous caller got the identity %q", got)
	}
}

// A wallet identity must never be mistaken for an account, or an account for a
// wallet — the two get different access.
func TestWalletAndAccountIdentitiesAreDistinguishable(t *testing.T) {
	if IsWalletIdentity("alice") || IsWalletIdentity("") {
		t.Error("an account id was treated as a wallet")
	}
	if !IsWalletIdentity("x402:0xabc") {
		t.Error("a wallet id was treated as an account")
	}
	// Account ids are usernames, which cannot contain a colon, so the two
	// namespaces cannot collide.
	if IsWalletIdentity("x402") {
		t.Error("a username-shaped id was treated as a wallet")
	}
}

// mail_send stays account-only: paying proves funds, not accountability, and an
// anonymous funded wallet sending from this domain spends its reputation.
func TestMailSendRefusesAPaidWallet(t *testing.T) {
	withPayer(t, "0xdeadbeef00000000000000000000000000001234")

	var mailSend *Tool
	for i := range tools {
		if tools[i].Name == "mail_send" {
			mailSend = &tools[i]
		}
	}
	if mailSend == nil {
		t.Fatal("mail_send is gone; the guard needs rechecking")
	}
	if !mailSend.AccountOnly {
		t.Fatal("mail_send is no longer account-only — a funded wallet could send from this domain")
	}

	text, isErr, err := ExecuteTool(httptest.NewRequest("POST", "/mcp", nil), "mail_send",
		map[string]any{"to": "someone@example.com", "subject": "x", "body": "y"})
	if err == nil || !isErr {
		t.Fatalf("a paid wallet was allowed to send mail: %q", text)
	}
	if !strings.Contains(text, "requires an account") {
		t.Errorf("refused for the wrong reason: %q", text)
	}
}

// The identity has to arrive at the handler, not merely be computed. This is
// the whole change: an auth-bound tool now receives the wallet as its account
// id instead of being refused.
func TestWalletIdentityReachesTheHandler(t *testing.T) {
	withPayer(t, "0x00000000000000000000000000000000c0ffee12")

	var got string
	RegisterToolWithAuth(Tool{Name: "identityprobe", Description: "probe"},
		func(_ map[string]any, accountID string) (string, error) {
			got = accountID
			return "ok", nil
		})
	defer func() { tools = tools[:len(tools)-1] }()

	text, isErr, err := ExecuteTool(httptest.NewRequest("POST", "/mcp", nil), "identityprobe", nil)
	if err != nil || isErr {
		t.Fatalf("a paid wallet was refused: %v (%q)", err, text)
	}
	if got != "x402:0x00000000000000000000000000000000c0ffee12" {
		t.Fatalf("handler saw %q, want the namespaced wallet", got)
	}
}

// And an anonymous caller must still be refused outright.
func TestAnonymousCallerIsStillRefused(t *testing.T) {
	withPayer(t, "")

	RegisterToolWithAuth(Tool{Name: "identityprobe2", Description: "probe"},
		func(_ map[string]any, accountID string) (string, error) {
			t.Errorf("handler ran for an anonymous caller as %q", accountID)
			return "", nil
		})
	defer func() { tools = tools[:len(tools)-1] }()

	if _, isErr, err := ExecuteTool(httptest.NewRequest("POST", "/mcp", nil), "identityprobe2", nil); err == nil || !isErr {
		t.Fatal("an anonymous caller reached an auth-bound tool")
	}
}

// Every account-scoped tool that is *not* mail_send should accept a paid
// wallet — that is the point of the change.
func TestOtherScopedToolsAcceptAPaidWallet(t *testing.T) {
	withPayer(t, "0xfeed000000000000000000000000000000001234")

	for _, name := range []string{"mail_inbox", "stream_post"} {
		var tool *Tool
		for i := range tools {
			if toolMatches(tools[i], name) {
				tool = &tools[i]
			}
		}
		if tool == nil {
			continue
		}
		if tool.AccountOnly {
			t.Errorf("%s is account-only; only mail_send should be", name)
		}
	}
}
