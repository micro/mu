package recall

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// TestRecallViaMesh verifies the recall service RPC round-trip and endpoint name.
func TestRecallViaMesh(t *testing.T) {
	if err := service.Register("recall", Server{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp Response
	if err := service.Call(context.Background(), "recall", "Server.Search",
		&Request{Query: "anything", Limit: 5}, &rsp); err != nil {
		t.Fatalf("call (wrong endpoint or transport?): %v", err)
	}
	if !strings.Contains(rsp.Text, "anything") {
		t.Fatalf("unexpected response: %q", rsp.Text)
	}
}

func TestRecallFormattingHelpers(t *testing.T) {
	if got := stripTags("<p>Hello <strong>world</strong></p>"); got != "Hello world" {
		t.Fatalf("stripTags() = %q", got)
	}
	if got := snippet("<p>Hello\n\tworld</p>", 20); got != "Hello world" {
		t.Fatalf("snippet() = %q", got)
	}
	if got := snippet("abcdef", 3); got != "abc…" {
		t.Fatalf("truncated snippet() = %q", got)
	}
	if got := firstLine(" first line \n second line", 20); got != "first line" {
		t.Fatalf("firstLine() = %q", got)
	}
	if got := firstLine("abcdef", 3); got != "abc…" {
		t.Fatalf("truncated firstLine() = %q", got)
	}
}
