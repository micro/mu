package news

import (
	"context"
	"testing"

	"mu/internal/service"
)

// TestNewsViaMesh verifies the go-micro RPC round-trip for the news service.
func TestNewsViaMesh(t *testing.T) {
	if err := service.Register("news", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp HeadlinesResponse
	if err := service.Call(context.Background(), "news", "Server.Headlines",
		&HeadlinesRequest{Limit: 5}, &rsp); err != nil {
		t.Fatalf("call: %v", err)
	}
	// Text may be empty without feeds loaded; the round-trip is what matters.
}
