package mail

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// TestMailSearchViaMesh verifies the mail service RPC round-trip and endpoint.
func TestMailSearchViaMesh(t *testing.T) {
	if err := service.Register(service.Spec{Name: "mail", Handler: new(Server)}); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp SearchResponse
	if err := service.Call(service.WithAccount(context.Background(), "nobody"), "mail", "Server.Search",
		&SearchRequest{Query: "invoice", Limit: 5}, &rsp); err != nil {
		t.Fatalf("call (endpoint/transport?): %v", err)
	}
	// No mail for an unknown account; the round-trip + formatting is what matters.
	if !strings.Contains(rsp.Text, "invoice") {
		t.Fatalf("unexpected response: %q", rsp.Text)
	}
}
