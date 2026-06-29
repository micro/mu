package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	gmagent "go-micro.dev/v6/agent"
	gmai "go-micro.dev/v6/ai"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/service"
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

// nativeStreamEnabled reports whether the STREAMING /agent path uses the native
// go-micro agent (StreamAsk). Default OFF: StreamAsk currently mis-resolves tool
// names mid-run on some providers, producing degraded answers ("the tool name
// wasn't quite right"), so the streaming UI stays on the hand-rolled pipeline
// until that's fixed upstream. Opt in with AGENT_NATIVE_STREAM=on. The
// non-streaming agent.Query path uses the native agent regardless (it works).
func nativeStreamEnabled() bool {
	if !nativeEnabled() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(settings.Get("AGENT_NATIVE_STREAM"))) {
	case "on", "true", "1", "yes":
		return true
	}
	return false
}

// Mode reports the active agent engine: "native" (go-micro agent) or "planner"
// (the hand-rolled pipeline). Surfaced on /version and /status.
func Mode() string {
	if nativeEnabled() {
		return "native"
	}
	return "planner"
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

// buildNativeAgent constructs the go-micro agent and the question (history +
// prompt) shared by queryNative and streamNative. ok is false when no native
// provider is configured, signalling the caller to fall back.
func buildNativeAgent(accountID, prompt string, opts QueryOpts) (a gmagent.Agent, question string, ok bool) {
	key := settings.Get("ATLAS_API_KEY")
	if key == "" {
		return nil, "", false
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

	question = prompt
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

	a = service.NewAgent("assistant", sys, "atlascloud", key, nativeServices(opts.Public),
		gmagent.Model(ai.ModelDeepSeekPro),
		gmagent.MaxSteps(6),
		gmagent.WrapTool(injectAccount(accountID)))
	return a, question, true
}

// queryNative answers using a go-micro agent wired to the registered domain
// services: the LLM does native tool-calling over those services (with the
// built-in plan/guardrails), replacing the hand-rolled planner+synthesizer.
//
// It returns (answer, true) when it handled the request, or ("", false) to
// signal the caller to fall back to the hand-rolled path (e.g. no Atlas key).
func queryNative(accountID, prompt string, opts QueryOpts) (string, bool, error) {
	a, question, ok := buildNativeAgent(accountID, prompt, opts)
	if !ok {
		return "", false, nil
	}
	defer a.Stop()

	resp, err := a.Ask(context.Background(), question)
	if err != nil {
		return "", true, fmt.Errorf("native agent: %w", err)
	}
	return app.StripLatexDollars(resp.Reply), true, nil
}

// StreamHooks receives streaming events from the native agent: tool lifecycle
// (with a friendly label) and answer tokens as they arrive.
type StreamHooks struct {
	ToolStart func(label string)
	ToolEnd   func(label string)
	Token     func(tok string)
}

// streamNative runs the native go-micro agent with StreamAsk, emitting tool
// start/end events and answer tokens via hooks, and returns the final answer.
// Returns (answer, true, err) when it handled the request, or ("", false, nil)
// to signal the caller to fall back (no provider).
func streamNative(accountID, prompt string, opts QueryOpts, hooks StreamHooks) (string, bool, error) {
	a, question, ok := buildNativeAgent(accountID, prompt, opts)
	if !ok {
		return "", false, nil
	}
	defer a.Stop()

	stream, err := gmagent.StreamAsk(context.Background(), a, question)
	if err != nil {
		return "", true, fmt.Errorf("native agent stream: %w", err)
	}

	var reply strings.Builder
	var final string
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", true, fmt.Errorf("native agent stream: %w", err)
		}
		if ev == nil {
			continue
		}
		switch ev.Type {
		case gmagent.StreamEventToolStart:
			if label, show := nativeToolLabel(ev.ToolCall.Name); show && hooks.ToolStart != nil {
				hooks.ToolStart(label)
			}
		case gmagent.StreamEventToolEnd:
			if label, show := nativeToolLabel(ev.ToolCall.Name); show && hooks.ToolEnd != nil {
				hooks.ToolEnd(label)
			}
		case gmagent.StreamEventToken:
			reply.WriteString(ev.Token)
			if hooks.Token != nil {
				hooks.Token(ev.Token)
			}
		case gmagent.StreamEventDone:
			if ev.Response != nil && ev.Response.Reply != "" {
				final = ev.Response.Reply
			}
		}
	}
	if final == "" {
		final = reply.String()
	}
	return app.StripLatexDollars(final), true, nil
}

// nativeToolLabel maps a go-micro tool name (service_Method) to a friendly
// progress label. show is false for the agent's internal/builtin tools (plan,
// delegate, …), which shouldn't surface as user-facing tool steps.
func nativeToolLabel(name string) (label string, show bool) {
	switch name {
	case "plan", "delegate", "human_input", "":
		return "", false
	}
	svc := name
	if i := strings.IndexByte(name, '_'); i > 0 {
		svc = name[:i]
	}
	switch svc {
	case "weather":
		return "🌤 Checking the weather", true
	case "news":
		return "📰 Scanning headlines", true
	case "markets":
		return "📈 Checking market prices", true
	case "social":
		return "💬 Reading social", true
	case "video":
		return "📺 Finding videos", true
	case "blog":
		return "📝 Reading the blog", true
	case "search":
		return "🔎 Searching the web", true
	case "trade":
		return "💱 Checking trade", true
	case "recall":
		return "🧠 Recalling your data", true
	case "apps":
		return "🧩 Browsing apps", true
	case "mail":
		return "📬 Checking your mail", true
	}
	return "⚙️ Working", true
}
