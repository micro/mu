package trade

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// TestTradeViaMesh verifies the go-micro RPC round-trip for the trade service.
// An unknown account has no wallet, so the service returns a clean error — which
// confirms the request reached the handler and the response routed back.
func TestTradeViaMesh(t *testing.T) {
	if err := service.Register("trade", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var info WalletInfo
	err := service.Call(context.Background(), "trade", "Server.Wallet",
		&WalletRequest{AccountID: "nonexistent-account"}, &info)
	if err == nil {
		t.Fatal("expected an error for an unknown account")
	}
	if !strings.Contains(err.Error(), "no trading wallet") {
		t.Fatalf("unexpected error (transport failure?): %v", err)
	}
}
