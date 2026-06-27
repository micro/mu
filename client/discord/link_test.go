package discord

import (
	"testing"
	"time"
)

func TestGenerateLinkCodeReplacesExistingAccountCode(t *testing.T) {
	codeMu.Lock()
	codes = map[string]*linkCode{}
	codeMu.Unlock()

	first := GenerateLinkCode("alice")
	second := GenerateLinkCode("alice")

	if first == second {
		t.Fatalf("expected replacement code to differ from first code")
	}
	if account, ok := redeemCode(first); ok || account != "" {
		t.Fatalf("old code redeemed as account %q, ok=%v; want expired replacement", account, ok)
	}
	if account, ok := redeemCode(second); !ok || account != "alice" {
		t.Fatalf("new code redeemed as account %q, ok=%v; want alice, true", account, ok)
	}
	if account, ok := redeemCode(second); ok || account != "" {
		t.Fatalf("code redeemed twice as account %q, ok=%v; want consumed", account, ok)
	}
}

func TestRedeemCodeRejectsExpiredCode(t *testing.T) {
	codeMu.Lock()
	codes = map[string]*linkCode{
		"expired": {Account: "alice", ExpiresAt: time.Now().Add(-time.Minute)},
	}
	codeMu.Unlock()

	if account, ok := redeemCode("expired"); ok || account != "" {
		t.Fatalf("expired code redeemed as account %q, ok=%v; want rejected", account, ok)
	}
	codeMu.Lock()
	_, stillPresent := codes["expired"]
	codeMu.Unlock()
	if stillPresent {
		t.Fatal("expired code was not removed")
	}
}
