package agent

import (
	"context"
	"testing"

	"mu/internal/service"

	gmai "go-micro.dev/v6/ai"
)

// TestInjectAccountBindsCallerToContext is a security regression test. The
// identity account-scoped tools act on must always be the authenticated
// caller's, and it must travel on the call context — never in the arguments,
// where prompt injection in tool content (an email body the model just read)
// could steer it at another user's data.
func TestInjectAccountBindsCallerToContext(t *testing.T) {
	capture := func() (gmai.ToolHandler, *map[string]any, *string) {
		var gotInput map[string]any
		var gotAccount string
		h := func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
			gotInput = call.Input
			gotAccount = service.AccountFrom(ctx)
			return gmai.ToolResult{}
		}
		return h, &gotInput, &gotAccount
	}

	cases := []struct {
		name   string
		caller string
		input  map[string]any
		want   string
	}{
		{"binds the caller", "acct-me", map[string]any{"query": "x"}, "acct-me"},
		{"ignores a model-supplied account", "acct-me", map[string]any{"account_id": "acct-victim"}, "acct-me"},
		{"binds with no input at all", "acct-me", nil, "acct-me"},
		{"a guest claims nobody", "", map[string]any{"account_id": "acct-victim"}, ""},
		{"a guest stays a guest", "", map[string]any{"query": "x"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, gotInput, gotAccount := capture()
			wrapped := injectAccount(tc.caller)(h)
			wrapped(context.Background(), gmai.ToolCall{Name: "mail.Inbox", Input: tc.input})

			if *gotAccount != tc.want {
				t.Fatalf("handler saw account %q, want %q", *gotAccount, tc.want)
			}
			// The arguments must never carry identity, whatever the model sent.
			if _, ok := (*gotInput)["account_id"]; ok {
				t.Fatalf("account_id reached the handler arguments: %v", *gotInput)
			}
		})
	}
}

// TestInjectAccountClearsInheritedIdentity guards the case where a wrapped call
// runs under a context that already carries someone else's account: a guest
// must not inherit it.
func TestInjectAccountClearsInheritedIdentity(t *testing.T) {
	var got string
	h := func(ctx context.Context, _ gmai.ToolCall) gmai.ToolResult {
		got = service.AccountFrom(ctx)
		return gmai.ToolResult{}
	}
	inherited := service.WithAccount(context.Background(), "acct-someone-else")
	injectAccount("")(h)(inherited, gmai.ToolCall{Name: "mail.Inbox"})
	if got != "" {
		t.Fatalf("guest inherited account %q", got)
	}
}
