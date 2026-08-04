package agent

import (
	"context"
	"testing"

	gmai "go-micro.dev/v6/ai"

	"mu/internal/service"
	"mu/service/tasks"
	"mu/service/wallet"
)

// registerGuarded stands up the services whose Specs declare the destructive
// methods. The block is derived from those declarations now, not from a table
// here, so the test has to exercise the real ones.
func registerGuarded(t *testing.T) {
	t.Helper()
	for _, s := range []service.Spec{wallet.Spec, tasks.Spec} {
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
}

// Every registered service becomes a tool. The guard is per-method: what is
// withheld is an irreversible side effect nobody asked for, not a capability.
func TestDestructiveMethodsAreBlocked(t *testing.T) {
	registerGuarded(t)
	blocked := []string{
		"wallet.charge", "Wallet.Charge", "wallet_charge", "WALLET_CHARGE",
		"tasks.delete", "tasks_delete", "TASKS.Delete",
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
	registerGuarded(t)
	allowed := []string{
		"wallet.balance", "wallet_balance", "wallet.check",
		"tasks.create", "tasks.list", "tasks.next", "tasks_update",
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
	registerGuarded(t)
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
