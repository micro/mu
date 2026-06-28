package news

import (
	"context"
	"testing"

	"mu/internal/mesh"
)

// TestNewsViaMesh verifies the go-micro RPC round-trip for the news service.
func TestNewsViaMesh(t *testing.T) {
	if err := mesh.Register("news", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp HeadlinesResponse
	if err := mesh.Call(context.Background(), "news", "Server.Headlines",
		&HeadlinesRequest{Limit: 5}, &rsp); err != nil {
		t.Fatalf("call: %v", err)
	}
	// Text may be empty without feeds loaded; the round-trip is what matters.
}
