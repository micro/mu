package apps

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// TestAppsBuildViaMesh verifies the apps service RPC round-trip and endpoint
// name. Without an AI provider configured, Build returns an AI error — which
// still proves the request reached the handler (not a transport/endpoint error).
func TestAppsBuildViaMesh(t *testing.T) {
	if err := service.Register("apps", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp BuildResponse
	err := service.Call(context.Background(), "apps", "Server.Build",
		&BuildRequest{Prompt: "a water counter", AccountID: "u1"}, &rsp)
	if err == nil {
		return // an AI provider was configured and it built — also fine
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") ||
		strings.Contains(strings.ToLower(err.Error()), "connection") {
		t.Fatalf("transport/endpoint error (routing broken): %v", err)
	}
}
