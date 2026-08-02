package api

import (
	"strings"
	"testing"
)

// No tool moves money to somebody else.
//
// wallet_transfer used to. It sent credits to another user by username,
// irreversibly, in one call with no confirmation — and the same agent holds
// mail_inbox, news_read, web_fetch and db_list, four ways to read text a
// stranger wrote. Nothing downstream could tell "the user asked" from "the
// agent read it in an email", and the web form's CSRF token does not apply to a
// caller holding a bearer token.
//
// The `pay` tool is the deliberate exception and is named here rather than
// pattern-matched: it settles a call to another MCP server against the caller's
// own wallet, which is buying something rather than giving it away, and it is
// bounded by the spend limit.
func TestNoToolTransfersCreditsToAnotherAccount(t *testing.T) {
	allowed := map[string]bool{"pay": true}

	for _, tool := range sortedTools() {
		if allowed[tool.Name] {
			continue
		}
		name := strings.ToLower(tool.Name)
		for _, verb := range []string{"transfer", "send_credits", "withdraw", "gift"} {
			if strings.Contains(name, verb) {
				t.Errorf("%q looks like it moves money out of an account; that is a form on a page, not an agent tool", tool.Name)
			}
		}
		if strings.Contains(strings.ToLower(tool.Description), "transfer credits") {
			t.Errorf("%q transfers credits; that is a form on a page, not an agent tool", tool.Name)
		}
	}
}

// The wallet surface is one read-only tool. Two tools answering "what is in my
// wallet" gave the planner a coin to flip, and a tool returning card tiers gave
// an agent a purchase flow it cannot complete.
func TestWalletSurfaceIsOneTool(t *testing.T) {
	var found []string
	for _, tool := range sortedTools() {
		if strings.HasPrefix(tool.Name, "wallet") {
			found = append(found, tool.Name)
		}
	}
	// wallet_balance is registered by main.go, so in this package's own tool
	// list the expected count is zero. Either way it must never be more than one.
	if len(found) > 1 {
		t.Errorf("the wallet surface is %v; it should be wallet_balance alone", found)
	}
}
