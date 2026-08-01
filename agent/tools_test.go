package agent

import (
	"context"
	"testing"

	gmai "go-micro.dev/v6/ai"
)

// Every registered service becomes a tool. The guard is per-method: what is
// withheld is an irreversible side effect nobody asked for, not a capability.
func TestDestructiveMethodsAreBlocked(t *testing.T) {
	blocked := []string{
		"wallet.charge", "Wallet.Charge", "wallet_charge", "WALLET_CHARGE",
		"db.delete", "db_delete", "DB.Delete",
	}
	for _, n := range blocked {
		if !toolBlocked(n) {
			t.Errorf("toolBlocked(%q) = false; the model must not invoke it", n)
		}
	}
}

// Reading is not destructive. Blocking a whole service to protect one method
// would take these with it.
func TestReadsAndOrdinaryToolsAreAllowed(t *testing.T) {
	allowed := []string{
		"wallet.balance", "wallet_balance", "wallet.check",
		"db.create", "db.list", "db.get", "db_update",
		"news.headlines", "web.fetch", "mail.inbox", "images.generate",
	}
	for _, n := range allowed {
		if toolBlocked(n) {
			t.Errorf("toolBlocked(%q) = true; it should be available", n)
		}
	}
}

// The refusal has to reach the model as a result, not an error, so it can
// explain rather than retry.
func TestBlockedCallIsRefusedNotExecuted(t *testing.T) {
	ran := false
	h := blockDestructiveTools()(func(_ context.Context, _ gmai.ToolCall) gmai.ToolResult {
		ran = true
		return gmai.ToolResult{Content: "executed"}
	})
	res := h(context.Background(), gmai.ToolCall{ID: "1", Name: "wallet.charge"})
	if ran {
		t.Fatal("the blocked tool executed")
	}
	if res.Refused == "" {
		t.Error("result should be marked refused")
	}
}
