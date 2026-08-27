package apps

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

func TestAppsSearchReadViaMesh(t *testing.T) {
	if err := service.Register(service.Spec{Name: "apps", Handler: new(Server)}); err != nil {
		t.Fatalf("register: %v", err)
	}
	var sr AppSearchResponse
	if err := service.Call(context.Background(), "apps", "Server.Search",
		&AppSearchRequest{Query: "nothing_xyz"}, &sr); err != nil {
		t.Fatalf("search call: %v", err)
	}
	if !strings.Contains(sr.Text, "nothing_xyz") {
		t.Fatalf("search resp: %q", sr.Text)
	}
	var rr AppReadResponse
	err := service.Call(context.Background(), "apps", "Server.Read",
		&AppReadRequest{Slug: "definitely_missing"}, &rr)
	if err == nil {
		t.Fatal("expected error for missing app")
	}
}
