package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	gmagent "go-micro.dev/v6/agent"
	gmai "go-micro.dev/v6/ai"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/mesh"
	"mu/internal/settings"
)

// nativeEnabled reports whether the native go-micro agent path is on. mu is an
// agent platform, so the go-micro agent is the default. Set AGENT_NATIVE to a
// falsey value (off/false/0/no) to fall back to the hand-rolled
// plan/execute/synthesize pipeline. If no LLM provider is configured the native
// path no-ops and the hand-rolled path runs regardless.
func nativeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(settings.Get("AGENT_NATIVE"))) {
	case "off", "false", "0", "no":
		return false
	}
	return true
}

// nativeServices are the registered go-micro domain services the native agent
// may use as tools. Guests get the public subset (no account-scoped data).
func nativeServices(public bool) []string {
	pub := []string{"weather", "news", "markets", "social", "video", "blog", "search"}
	if public {
		return pub
	}
	return append(pub, "trade", "recall", "apps", "mail")
}

// injectAccount is a tool wrapper that supplies the caller's account id to
// account-scoped service methods (recall, trade, …) — the LLM never sees it,
// so this preserves the auth scoping the hand-rolled tools enforced.
func injectAccount(accountID string) gmai.ToolWrapper {
	return func(next gmai.ToolHandler) gmai.ToolHandler {
		return func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
			if accountID != "" {
				if call.Input == nil {
					call.Input = map[string]any{}
				}
				if _, ok := call.Input["account_id"]; !ok {
					call.Input["account_id"] = accountID
				}
			}
			return next(ctx, call)
		}
	}
}

// queryNative answers using a go-micro agent wired to the registered domain
// services: the LLM does native tool-calling over those services (with the
// built-in plan/guardrails), replacing the hand-rolled planner+synthesizer.
//
// It returns (answer, true) when it handled the request, or ("", false) to
// signal the caller to fall back to the hand-rolled path (e.g. no Atlas key).
func queryNative(accountID, prompt string, opts QueryOpts) (string, bool, error) {
	key := settings.Get("ATLAS_API_KEY")
	if key == "" {
		return "", false, nil // no native provider configured — fall back
	}

	today := time.Now().UTC().Format("Monday, 2 January 2006 (UTC)")
	sys := "You are Micro, a personal AI assistant on Mu. Today is " + today + ". " +
		"Use the available tools for live or personal data (weather, news, market prices, " +
		"social, video, blog, web search, trading, and recall of the user's own news/mail). " +
		"Quote exact values from tool results. Be concise and conversational. " +
		"If the user asks about weather without a location, default to London (lat 51.5074, lon -0.1278)."
	if !opts.Public && UserContextFunc != nil {
		if uc := UserContextFunc(accountID); uc != "" {
			sys += "\n\nUser context:\n" + uc
		}
	}

	question := prompt
	if len(opts.History) > 0 {
		var hb strings.Builder
		hb.WriteString("Conversation so far:\n")
		for _, m := range opts.History {
			if m.Role == "user" {
				hb.WriteString("User: " + m.Text + "\n")
			} else {
				hb.WriteString("Assistant: " + truncate(m.Text, 300) + "\n")
			}
		}
		hb.WriteString("\nNew message: " + prompt)
		question = hb.String()
	}

	a := mesh.NewAgent("assistant", sys, "atlascloud", key, nativeServices(opts.Public),
		gmagent.Model(ai.ModelDeepSeekPro),
		gmagent.MaxSteps(6),
		gmagent.WrapTool(injectAccount(accountID)))
	defer a.Stop()

	resp, err := a.Ask(context.Background(), question)
	if err != nil {
		return "", true, fmt.Errorf("native agent: %w", err)
	}
	return app.StripLatexDollars(resp.Reply), true, nil
}
