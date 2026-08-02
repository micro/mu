package markets

import (
	"context"
	"testing"

	"mu/internal/service"
)

// TestMarketsViaMesh verifies the go-micro RPC round-trip for the markets service.
func TestMarketsViaMesh(t *testing.T) {
	if err := service.Register("markets", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp PricesResponse
	if err := service.Call(context.Background(), "markets", "Server.Prices",
		&PricesRequest{Category: "crypto"}, &rsp); err != nil {
		t.Fatalf("call: %v", err)
	}
}
