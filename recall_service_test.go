package main

import (
	"context"
	"strings"
	"testing"

	"mu/internal/mesh"
)

// TestRecallViaMesh verifies the recall service RPC round-trip and endpoint name.
func TestRecallViaMesh(t *testing.T) {
	if err := mesh.Register("recall", RecallServer{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp RecallResponse
	if err := mesh.Call(context.Background(), "recall", "RecallServer.Search",
		&RecallRequest{Query: "anything", Limit: 5}, &rsp); err != nil {
		t.Fatalf("call (wrong endpoint or transport?): %v", err)
	}
	if !strings.Contains(rsp.Text, "anything") {
		t.Fatalf("unexpected response: %q", rsp.Text)
	}
}
