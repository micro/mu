package agent

import (
	"context"
	"testing"

	gmai "go-micro.dev/v6/ai"
)

// TestInjectAccountForcesCallerAccount is a security regression test: the
// account id passed to account-scoped tools must always be the authenticated
// caller's, never a value the model supplied. A model-supplied account_id can
// be steered by prompt injection in tool content (e.g. an email body), so it
// must be overwritten, and stripped entirely for guests.
func TestInjectAccountForcesCallerAccount(t *testing.T) {
	capture := func() (gmai.ToolHandler, *map[string]any) {
		var got map[string]any
		h := func(_ context.Context, call gmai.ToolCall) gmai.ToolResult {
			got = call.Input
			return gmai.ToolResult{}
		}
		return h, &got
	}

	cases := []struct {
		name    string
		caller  string
		input   map[string]any
		wantAcc any  // expected account_id value
		present bool // whether account_id should be present at all
	}{
		{"injects when absent", "acct-me", map[string]any{"query": "x"}, "acct-me", true},
		{"overrides model-supplied", "acct-me", map[string]any{"account_id": "acct-victim"}, "acct-me", true},
		{"overrides nil input", "acct-me", nil, "acct-me", true},
		{"guest strips model-supplied", "", map[string]any{"account_id": "acct-victim"}, nil, false},
		{"guest leaves none", "", map[string]any{"query": "x"}, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, got := capture()
			wrapped := injectAccount(tc.caller)(h)
			wrapped(context.Background(), gmai.ToolCall{Name: "mail.Inbox", Input: tc.input})

			v, ok := (*got)["account_id"]
			if ok != tc.present {
				t.Fatalf("account_id present=%v, want %v (input=%v)", ok, tc.present, *got)
			}
			if tc.present && v != tc.wantAcc {
				t.Fatalf("account_id=%v, want %v", v, tc.wantAcc)
			}
		})
	}
}
