package news

import (
	"context"
	"strings"
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

func TestNewsSearchServiceReturnsFreshnessPayload(t *testing.T) {
	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "Find today's AI news"}, &rsp); err != nil {
		t.Fatalf("search: %v", err)
	}
	if rsp.Text == "" {
		t.Fatal("expected search payload text")
	}
	if !strings.Contains(rsp.Text, `"query":"Find today`) || !strings.Contains(rsp.Text, `"results"`) {
		t.Fatalf("expected JSON news_search payload, got %q", rsp.Text)
	}
}
