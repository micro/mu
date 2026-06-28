package markets

import (
	"context"
	"testing"

	"mu/internal/mesh"
)

// TestMarketsViaMesh verifies the go-micro RPC round-trip for the markets service.
func TestMarketsViaMesh(t *testing.T) {
	if err := mesh.Register("markets", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp PricesResponse
	if err := mesh.Call(context.Background(), "markets", "Server.Prices",
		&PricesRequest{Category: "crypto"}, &rsp); err != nil {
		t.Fatalf("call: %v", err)
	}
}
