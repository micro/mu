package mail

import (
	"context"
	"strings"
	"testing"

	"mu/internal/mesh"
)

// TestMailSearchViaMesh verifies the mail service RPC round-trip and endpoint.
func TestMailSearchViaMesh(t *testing.T) {
	if err := mesh.Register("mail", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp SearchResponse
	if err := mesh.Call(context.Background(), "mail", "Server.Search",
		&SearchRequest{AccountID: "nobody", Query: "invoice", Limit: 5}, &rsp); err != nil {
		t.Fatalf("call (endpoint/transport?): %v", err)
	}
	// No mail for an unknown account; the round-trip + formatting is what matters.
	if !strings.Contains(rsp.Text, "invoice") {
		t.Fatalf("unexpected response: %q", rsp.Text)
	}
}
