package apps

import (
	"context"
	"strings"
	"testing"

	"mu/internal/mesh"
)

func TestAppsSearchReadViaMesh(t *testing.T) {
	if err := mesh.Register("apps", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var sr AppSearchResponse
	if err := mesh.Call(context.Background(), "apps", "Server.Search",
		&AppSearchRequest{Query: "nothing-xyz"}, &sr); err != nil {
		t.Fatalf("search call: %v", err)
	}
	if !strings.Contains(sr.Text, "nothing-xyz") {
		t.Fatalf("search resp: %q", sr.Text)
	}
	var rr AppReadResponse
	err := mesh.Call(context.Background(), "apps", "Server.Read",
		&AppReadRequest{Slug: "definitely-missing"}, &rr)
	if err == nil {
		t.Fatal("expected error for missing app")
	}
}
