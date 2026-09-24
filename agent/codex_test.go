package agent

import (
	"context"
	gmai "go-micro.dev/v6/model"
	"mu/internal/auth"
	"mu/internal/service"
	"testing"
)

func TestCodexPreviewCannotBeSelectedByAnotherAccountOrBackground(t *testing.T) {
	auth.SetAccountForTest(&auth.Account{ID: "codex-routing-admin", Admin: true, CodexPreview: true})
	auth.SetAccountForTest(&auth.Account{ID: "codex-routing-user", CodexPreview: true})
	defer auth.RemoveAccountForTest("codex-routing-admin")
	defer auth.RemoveAccountForTest("codex-routing-user")
	for _, tc := range []struct {
		id   string
		opts QueryOpts
		want bool
	}{{"codex-routing-admin", QueryOpts{interactive: true}, true}, {"codex-routing-user", QueryOpts{interactive: true}, false}, {"codex-routing-admin", QueryOpts{}, false}, {"codex-routing-admin", QueryOpts{interactive: true, Public: true}, false}, {"", QueryOpts{interactive: true}, false}} {
		p, _, _, _, _ := nativeModelForAccount(tc.id, tc.opts)
		if (p == "codex") != tc.want {
			t.Fatalf("id=%s opts=%+v provider=%s", tc.id, tc.opts, p)
		}
	}
}

func TestCodexToolHandlerCannotChooseAccount(t *testing.T) {
	called := false
	h := injectAccount("codex-owner")(func(ctx context.Context, c gmai.ToolCall) gmai.ToolResult {
		called = true
		if _, ok := c.Input["account_id"]; ok {
			t.Fatal("model-supplied identity survived")
		}
		if service.AccountFrom(ctx) != "codex-owner" {
			t.Fatal("wrong authenticated account")
		}
		return gmai.ToolResult{}
	})
	h(context.Background(), gmai.ToolCall{Input: map[string]any{"account_id": "victim"}})
	if !called {
		t.Fatal("handler did not run")
	}
}
