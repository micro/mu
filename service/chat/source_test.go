package chat

import (
	"context"
	"mu/internal/service"
	"testing"
)

func TestSourceIsAccountScoped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id, err := KeepSaved("source-owner", Said{Conv: "conv", From: "external@example.com", To: "owner@example.com", Text: "private"})
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"source-owner", "other"} {
		var rsp SourceResponse
		if err := (Server{}).Source(service.WithAccount(context.Background(), owner), &SourceRequest{Id: id}, &rsp); err != nil {
			t.Fatal(err)
		}
		if (rsp.Item != nil) != (owner == "source-owner") {
			t.Fatalf("wrong scope for %s", owner)
		}
	}
}
