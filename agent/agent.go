// Package agent provides a conversational AI agent interface that has access
// to all Mu tools via the MCP server, using the user's session token for calls.
package agent

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"mu/agent/micro"
	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/wallet"
)

// Model represents an available LLM model tier for agent queries.
type Model struct {
	ID       string
	Name     string
	WalletOp string
	Provider string // ai provider constant, empty = default
	Model    string // ai model override, empty = provider default
}

// defaultPremiumModel is the Anthropic model used for premium agent queries.
var defaultPremiumModel = func() string {
	if v := os.Getenv("ANTHROPIC_PREMIUM_MODEL"); v != "" {
		return v
	}
	return "claude-opus-4-20250514"
}()

// Models lists the available model tiers.
var Models = []Model{
	{
		ID:       "standard",
		Name:     "Fast",
		WalletOp: wallet.OpAgentQuery,
		Provider: ai.ProviderDefault,
	},
	{
		ID:       "premium",
		Name:     "Best",
		WalletOp: wallet.OpAgentQueryPremium,
		Provider: ai.ProviderAnthropic,
		Model:    defaultPremiumModel,
	},
}

// QuotaCheck is set by main.go to wire in the wallet quota check without an
// import cycle. Signature matches api.QuotaCheck.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// ChargeQuota is set by main.go to deduct credits from the acting user's wallet
// once an agent query is admitted. Charging the user who runs the agent (not any
// app owner) is deliberate. Admins and self-hosted instances are unaffected.
var ChargeQuota func(r *http.Request, op string)

// Load initialises the agent package (no-op for now; reserved for future use).
func Load() {}

// QueryMessage is a single turn in a conversation.
type QueryMessage struct {
	Role string // "user" or "assistant"
	Text string
}

// QueryOpts controls what context is included in agent queries.
type QueryOpts struct {
	History []QueryMessage
	Public  bool   // if true, skip private context (mail, wallet, etc.)
	System  string // optional custom system prompt (user-defined agent)
	// Extra is context for this call only — today, the summary of the cards
	// the reader watches, passed when they ask for it. Per-call rather than a
	// package hook because it is a choice made per message: context costs
	// tokens on every turn and most questions have nothing to do with it.
	Extra string
	Tools []string // optional tool allow-list (user-defined agent); empty = all
	// OnStep is called once per tool the agent runs, if set.
	//
	// The pipeline discarded this: a caller got a final string and no way to
	// know what produced it. That is tolerable for a chat reply, where the
	// answer arrives while you are watching, and useless for a task run, which
	// happens while you are not — "it did something and here is a paragraph"
	// is not something anyone can check.
	//
	// A callback rather than a return value or a package-level hook: it needs
	// to work for the micro path too, and two runs by the same account at the
	// same time must not interleave into one list.
	OnStep func(Step)
}

// microStepper adapts OnStep for the micro path, which cannot import this
// package.
func microStepper(opts QueryOpts) func(string, map[string]any, bool, time.Duration) {
	if opts.OnStep == nil {
		return nil
	}
	return func(tool string, args map[string]any, ok bool, took time.Duration) {
		opts.OnStep(Step{Tool: tool, Args: args, OK: ok, Took: took})
	}
}

// Step is one tool the agent ran.
type Step struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
	OK   bool           `json:"ok"`
	Took time.Duration  `json:"took"`
}

// Query runs the agent pipeline synchronously for MCP and bot callers.
func Query(accountID, prompt string, history ...QueryMessage) (string, error) {
	return QueryWithOpts(accountID, prompt, QueryOpts{History: history})
}

// QueryWithOpts runs the agent with explicit options.
// Routes to specialised micro-agents when possible.
func QueryWithOpts(accountID, prompt string, opts QueryOpts) (string, error) {
	// Check for direct agent addressing
	if agentID := micro.MatchDirectAddress(prompt); agentID != "" {
		cleanPrompt := micro.StripAddress(prompt)
		return normalizeQueryAnswer(micro.Orchestrate(accountID, cleanPrompt, []string{agentID}, opts.Public, microStepper(opts)))
	}

	// Route to specialist agent(s)
	agentIDs := micro.Route(prompt)

	// If router picks specialist(s), use the multi-agent system
	if len(agentIDs) > 0 && agentIDs[0] != "micro" {
		return normalizeQueryAnswer(micro.Orchestrate(accountID, prompt, agentIDs, opts.Public, microStepper(opts)))
	}

	// Native go-micro agent path (default for this agent platform; set
	// AGENT_NATIVE=off to disable): the LLM does native tool-calling over the
	// registered domain services instead of the hand-rolled plan/execute/
	// synthesize below. If the native provider is unconfigured, or a native run
	// errors, fall through to the hand-rolled pipeline so a query never fails.
	if nativeEnabled() {
		if answer, handled, err := queryNative(accountID, prompt, opts); handled {
			if err == nil {
				return app.NormalizeAnswerMarkdown(answer), nil
			}
			app.Log("agent", "native agent failed, falling back to planner: %v", err)
		}
	}

	// Fall through to the existing monolithic agent for "micro" (catch-all)
	model := Models[0] // standard

	// Build conversation context for the planner
	var convContext string
	if len(opts.History) > 0 {
		var cb strings.Builder
		cb.WriteString("Conversation so far:\n")
		for _, m := range opts.History {
			if m.Role == "user" {
				cb.WriteString("User: " + m.Text + "\n")
			} else {
				cb.WriteString("Assistant: " + truncate(m.Text, 300) + "\n")
			}
		}
		cb.WriteString("\nNew message: " + prompt)
		convContext = cb.String()
	}

	// --- Plan ---
	type toolCall struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	var toolCalls []toolCall

	if tc := shortcutToolCalls(prompt); len(tc) > 0 {
		for _, s := range tc {
			toolCalls = append(toolCalls, toolCall{Tool: s.Tool, Args: s.Args})
		}
	} else {
		planQuestion := prompt
		if convContext != "" {
			planQuestion = convContext
		}

		userCtx := ""
		if !opts.Public && UserContextFunc != nil {
			userCtx = UserContextFunc(accountID)
		}
		if opts.Extra != "" {
			userCtx = strings.TrimSpace(userCtx + "\n\n" + opts.Extra)
		}
		toolsDesc := agentToolsDesc
		if opts.Public {
			toolsDesc = guestToolsDesc
		}
		planSystem := "You are an AI agent. Given a user question, output ONLY a JSON array of tool calls (no other text, no markdown).\n\n" +
			toolsDesc +
			"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tool calls. If no tools are needed output []." +
			"\n\nIMPORTANT: For personal questions like 'do I have mail', 'what's the weather', 'news today', 'btc price' — ALWAYS use the appropriate tool. " +
			"If the user says 'weather' without a location, use their location from user context, or default to London (lat:51.5074, lon:-0.1278)."
		if userCtx != "" {
			planSystem += "\n\nUser context:\n" + userCtx
		}

		planPrompt := &ai.Prompt{
			System:   planSystem,
			Question: planQuestion,
			Priority: ai.PriorityHigh,
			Provider: "",
			Model:    ai.BackgroundModel(),
			Caller:   "agent-plan",
		}
		planResult, err := ai.Ask(planPrompt)
		if err != nil {
			return "", fmt.Errorf("planning failed: %w", err)
		}
		planJSON := extractJSONArray(planResult)
		json.Unmarshal([]byte(planJSON), &toolCalls)
	}

	// --- Execute ---
	type toolResult struct {
		Name      string
		Result    string
		Args      map[string]any
		Formatted string
	}
	var results []toolResult
	var unavailableTools []string
	seenToolCalls := map[string]bool{}

	for i := 0; i < len(toolCalls); i++ {
		tc := toolCalls[i]
		if tc.Tool == "" {
			continue
		}
		key := toolCallKey(tc.Tool, tc.Args)
		if seenToolCalls[key] {
			continue
		}
		seenToolCalls[key] = true
		if skipMarketMoverCompanionTool(prompt, tc.Tool) {
			continue
		}
		if opts.Public && !isGuestAllowedTool(tc.Tool) {
			continue
		}
		startedAt := time.Now()
		text, isErr, execErr := api.ExecuteToolAs(accountID, tc.Tool, tc.Args)
		if opts.OnStep != nil {
			opts.OnStep(Step{Tool: tc.Tool, Args: tc.Args, OK: execErr == nil && !isErr, Took: time.Since(startedAt)})
		}
		if execErr != nil || isErr {
			unavailableTools = append(unavailableTools, tc.Tool)
			if fallback, ok := fallbackNewsSearchToolCall(prompt, tc.Tool, tc.Args); ok {
				key := toolCallKey(fallback.Tool, fallback.Args)
				if !seenToolCalls[key] && (!opts.Public || isGuestAllowedTool(fallback.Tool)) {
					toolCalls = append(toolCalls, toolCall{Tool: fallback.Tool, Args: fallback.Args})
				}
			}
			continue
		}
		if len(text) > 8000 {
			text = text[:8000] + "…"
		}
		results = append(results, toolResult{Name: tc.Tool, Result: text, Args: tc.Args})
	}

	// --- Synthesize ---
	var ragParts []string
	for i, res := range results {
		ragText := formatToolResult(res.Name, res.Result, res.Args)
		results[i].Formatted = ragText
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", res.Name, ragText))
	}
	for _, tool := range unavailableTools {
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", tool, unavailableToolMessage(tool)))
	}

	today := currentDateContext(time.Now().UTC())

	// Include conversation history in RAG context
	if len(opts.History) > 0 {
		var hb strings.Builder
		hb.WriteString("### Conversation history\n")
		for _, m := range opts.History {
			if m.Role == "user" {
				hb.WriteString("**User:** " + m.Text + "\n\n")
			} else {
				hb.WriteString("**Assistant:** " + truncate(m.Text, 500) + "\n\n")
			}
		}
		ragParts = append([]string{hb.String()}, ragParts...)
	}

	var synthSystem string
	if len(results) == 0 {
		synthSystem = "You are Micro, the AI assistant on Mu — a personal AI platform. Today is " + today + ". " +
			"Answer conversationally. Be helpful and concise."
	} else {
		synthSystem = "You are Micro, a personal AI assistant. Today is " + today + ". " +
			"Answer using the tool results provided. Be concise. " +
			"For prices, weather, and live data, use the exact values from tool results."
	}

	if !opts.Public {
		synthUserCtx := ""
		if UserContextFunc != nil {
			synthUserCtx = UserContextFunc(accountID)
		}
		if synthUserCtx != "" {
			synthSystem += "\n\nUser context:\n" + synthUserCtx
		}
	}

	// The agent you asked writes the answer, not the house assistant.
	//
	// QueryOpts.System is documented as "optional custom system prompt
	// (user-defined agent)" and was read in exactly one place — the native
	// path — so every caller that came through here composed as Micro no matter
	// which agent it was for. That is every task run and every scheduled run,
	// which are precisely the ones nobody is watching.
	//
	// It goes in front of the composition rules rather than replacing them: the
	// rules are about not inventing prices and dates from stale training data,
	// which is not an agent's to override.
	if sys := strings.TrimSpace(opts.System); sys != "" {
		synthSystem = sys + "\n\n" + synthSystem
	}

	synthPrompt := &ai.Prompt{
		System:   synthSystem,
		Rag:      ragParts,
		Question: prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
		Caller:   "agent-synth",
	}

	answer, err := ai.Ask(synthPrompt)
	if err != nil {
		return "", fmt.Errorf("synthesis failed: %w", err)
	}
	return app.NormalizeAnswerMarkdown(app.StripLatexDollars(answer)), nil
}

func normalizeQueryAnswer(answer string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return app.NormalizeAnswerMarkdown(answer), nil
}

// Handler dispatches GET (page) and POST (query) at /agent and /agent/*.
func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// /agent/flow/<id>  — view or delete a saved flow
	if strings.HasPrefix(path, "/agent/flow/") {
		id := strings.TrimPrefix(path, "/agent/flow/")
		switch r.Method {
		case "GET":
			// Saved flows are conversations now — reopen in the unified chat.
			if id != "" && r.URL.Query().Get("json") != "1" {
				http.Redirect(w, r, "/agent?session="+id, http.StatusFound)
				return
			}
			serveFlowPage(w, r, id) // ?json=1 still returns the flow JSON for polling
		case "DELETE":
			handleDeleteFlow(w, r, id)
		default:
			app.MethodNotAllowed(w, r)
		}
		return
	}
	switch r.Method {
	case "GET":
		servePage(w, r)
	case "POST":
		handleQuery(w, r)
	default:
		app.MethodNotAllowed(w, r)
	}
}

// servePage renders the unified chat surface: a sessions rail (logged-in users)
// beside the shared chat component, in the standard app shell. Reopening a saved
// session loads its turns in place and continues the same conversation. This is
// the single AI surface — the home assistant and guest landing render the same
// chat component.
func servePage(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	guest := acc == nil
	accountID := ""
	if acc != nil {
		accountID = acc.ID
	}

	// Reopen a saved session (?session=, or legacy ?continue=).
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = r.URL.Query().Get("continue")
	}
	cfg := app.ChatConfig{Guest: guest, StorageNS: "agent"}
	activeRoot := "" // stable id of the reopened conversation, for rail highlight
	reopened := false
	reopenAgent := "" // agent id the reopened conversation was using
	if sessionID != "" && !guest {
		// Resolve the (possibly stale) id to the whole chain and its current head
		// so follow-ups continue this conversation instead of branching it.
		turns := sessionChain(accountID, sessionID)
		if len(turns) > 0 && turns[len(turns)-1].AccountID == accountID {
			head := turns[len(turns)-1]
			cfg.ContextID = head.ID // seed to the true head
			cfg.InitialConvHTML = renderSessionTurns(turns)
			activeRoot = turns[0].ID
			reopened = true
			reopenAgent = head.Agent
		}
	}

	// Prefill prompt from ?q / ?prompt (e.g. home card deep-links).
	prefill := r.URL.Query().Get("prompt")
	if prefill == "" {
		prefill = r.URL.Query().Get("q")
	}

	// Which agent this page is for. Clicking an agent's name on /agents means
	// "talk to that one", so it selects the agent, opens its most recent
	// conversation, and shows only that agent's conversations in the rail —
	// which is what a chat with one agent means. Before, it selected the agent
	// and then dropped you into whichever conversation happened to be newest,
	// listing every agent's history beside it.
	selAgent := r.URL.Query().Get("id")
	if selAgent == "" {
		selAgent = r.URL.Query().Get("agent")
	}
	if reopened {
		// A reopened conversation decides its own agent; the rail filters to it.
		selAgent = reopenAgent
	} else if selAgent != "" && !guest && prefill == "" {
		// Land in the last conversation with this agent, if there is one.
		if last := latestSessionFor(accountID, selAgent); last != "" {
			if turns := sessionChain(accountID, last); len(turns) > 0 {
				head := turns[len(turns)-1]
				cfg.ContextID = head.ID
				cfg.InitialConvHTML = renderSessionTurns(turns)
				activeRoot = turns[0].ID
			}
		}
	}

	rail := ""
	if !guest {
		rail = `<div class="chat-side">` + renderAgentsPanel() +
			renderSessionsRail(accountID, activeRoot, selAgent) + `</div>`
	}

	chip := ""
	if !guest {
		// The chip alone said "Agent: Micro" and nothing else, which leaves the
		// obvious question unanswered — what is this, and is it the same thing
		// as the agent I came here to connect? It is not: this one is Mu's,
		// running on the tools. Yours reaches the same tools over MCP.
		//
		// The link beside it used to be a generic "Connect your own agent"
		// pointing at /tools, on a page that already knows which agent you are
		// looking at. When it knows, it points at that one's endpoint, scope and
		// token rather than at the catalogue.
		connect := `<a class="agent-connect" href="/tools">Connect your own agent &rarr;</a>`
		if selAgent != "" {
			connect = `<a class="agent-connect" href="/agent/connect?id=` +
				url.QueryEscape(selAgent) + `">How to reach this one &rarr;</a>`
		}
		chip = `<div class="agent-bar">` +
			`<div id="active-agent-chip" class="agent-chip">Agent: Micro</div>` +
			connect + `</div>`
	}

	// A signed-out visitor arrives here from the landing's "See it working",
	// and used to meet a bare chat box: the same one on any site, with nothing
	// saying the answers come from tools running on this instance rather than
	// from a model's memory. That is the whole claim, and the page was letting
	// it go unsaid. Two sentences and a link close the loop back to /tools.
	if guest {
		chip = `<div class="agent-intro">` +
			`<b>This is the agent, using the tools.</b> Ask it something and it calls them on this ` +
			`instance — the news feeds, the search index, the markets data — the same tools your own ` +
			`agent gets over <a href="/mcp">MCP</a>. ` +
			`<a href="/tools">See what it can reach &rarr;</a></div>`
	}
	// Tabs above the conversation for anyone signed in: talking to an agent and
	// seeing what it has done are the same question asked twice, and runs used
	// to be a top-level page as if they were a peer of the agent itself.
	tabs := ""
	if !guest {
		tabs = agentTabs("chat", selAgent)
	}
	content := `<div class="chat-layout">` + rail + `<div class="chat-main">` + tabs + chip + app.ChatComponent(cfg) + `</div></div>` + chatLayoutCSS

	// Seed the active agent so the panel highlights it and follow-ups continue
	// with it: an explicit ?id= selection (deep link) wins; otherwise a reopened
	// session restores the agent it was using (blank resets to default,
	// overriding any stale in-tab selection).
	//
	// ?id= because that is what /agent/new and /agents already call an agent's
	// id, and the same object should not have two names depending on which page
	// links to it. ?agent= still works: links to it exist, and breaking a URL to
	// tidy a parameter is a bad trade.
	// The id stays in the URL. It used to be stripped straight back out with
	// replaceState, which made the address bar disagree with the page and a
	// reload forget which agent you were talking to — the redirect looked
	// wasteful because it was.
	//
	// Seeded unconditionally, including the empty default. selAgent is what the
	// rail was filtered by and what the conversation was loaded for, so it is
	// the page's answer to "which agent is this" and the chip has to give the
	// same one. Seeding only when it was non-empty left the tab's remembered
	// selection in charge on a bare /agent: the rail listed every agent's
	// conversations while the chip named one of them, and the next message went
	// to the agent the chip named. The URL is the state.
	if !guest {
		content += `<script>window.muSeedAgent(` + app.JSString(selAgent) + `);</script>`
	}
	if prefill != "" {
		content += `<script>(function(){var i=document.getElementById('mu-chat-input');if(i&&window.muChatAsk){i.value=` + app.JSString(prefill) + `;window.muChatAsk(i.value);}history.replaceState(null,'','/agent');})()</script>`
	}

	html := app.RenderHTMLForRequest("Agent", "Ask the Mu agent — news, mail, markets, weather, search and more, with tools", content, r)
	w.Write([]byte(html))
}

// renderSessionTurns renders a conversation's prior turns into the chat log.
func renderSessionTurns(turns []*Flow) string {
	var b strings.Builder
	for _, f := range turns {
		b.WriteString(`<div class="mu-user">` + htmlEsc(f.Prompt) + `</div>`)
		b.WriteString(`<div class="mu-agent"><div class="card" id="agent-response">` + app.RenderString(f.Answer) + `</div></div>`)
	}
	return b.String()
}

// renderSessionsRail renders the list of past conversations.
func renderSessionsRail(accountID, currentID, agentID string) string {
	sessions := ListSessions(accountID)
	// One agent's conversations, when a page is about one agent. The rail listed
	// every conversation on the account regardless, so a brand-new agent opened
	// showing somebody else's history and looked like it had already been used.
	if agentID != "" {
		var mine []Session
		for _, s := range sessions {
			if s.Agent == agentID {
				mine = append(mine, s)
			}
		}
		sessions = mine
	}
	// A new chat with the agent whose rail this is. It used to rewrite the URL
	// to a bare /agent, which dropped the agent out of the address bar while
	// the page went on talking to it — so a reload landed you on the default
	// and the rail silently widened to every conversation on the account.
	newURL := "/agent"
	if agentID != "" {
		newURL += "?id=" + url.QueryEscape(agentID)
	}
	var b strings.Builder
	b.WriteString(`<aside class="chat-rail"><button class="chat-new" onclick="if(window.muChatNew){muChatNew();history.replaceState(null,''` +
		`,` + app.JSString(newURL) + `);document.querySelectorAll('.chat-sess.active').forEach(function(e){e.classList.remove('active')});}">+ New chat</button><div class="chat-sess-list">`)
	if len(sessions) == 0 {
		if agentID != "" {
			b.WriteString(`<div class="chat-sess-empty">No conversations with this agent yet.</div>`)
		} else {
			b.WriteString(`<div class="chat-sess-empty">No conversations yet.</div>`)
		}
	}
	for _, s := range sessions {
		cls := "chat-sess"
		if s.RootID == currentID {
			cls += " active"
		}
		title := s.Title
		if title == "" {
			title = "Untitled"
		}
		if len(title) > 60 {
			title = title[:60] + "…"
		}
		b.WriteString(`<a href="/agent?session=` + s.RootID + `" class="` + cls + `">` + htmlEsc(title) + `</a>`)
	}
	b.WriteString(`</div></aside>`)
	return b.String()
}

const chatLayoutCSS = `<style>
.chat-layout{display:flex;gap:24px;align-items:flex-start}
.chat-side{width:250px;flex-shrink:0;display:flex;flex-direction:column}
.chat-side .chat-rail{width:auto}
.chat-rail{width:250px;flex-shrink:0}
.chat-main{flex:1;min-width:0}
/* The conversation fills the main pane here (the 760px readability cap only
   applies to the chat embedded on the landing/home page). */
.chat-main #mu-chat{max-width:none}
/* Which agent is answering — always visible above the conversation. */
.agent-bar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-bottom:10px}
.agent-connect{margin-left:auto;font-size:13px;color:var(--text-muted,#666);text-decoration:none;white-space:nowrap}
.agent-connect:hover{color:var(--text-primary,#111)}
@media only screen and (max-width:600px){.agent-connect{margin-left:0}}
.agent-chip{display:inline-block;padding:3px 10px;border-radius:999px;background:var(--hover-background,#f5f5f5);color:var(--text-primary,#111);font-size:12px;font-weight:600;font-variant-numeric:tabular-nums}
.agent-intro{margin:0 0 14px;padding:12px 14px;border:1px solid var(--border-color,#e5e5e5);border-radius:8px;font-size:14px;line-height:1.55;color:var(--text-secondary,#555)}
.agent-intro b{color:var(--text-primary,#111)}
.agent-intro a{color:var(--text-primary,#111)}
.chat-new{width:100%;padding:9px 12px;background:#111;color:#fff;border:none;border-radius:6px;cursor:pointer;font-size:14px;font-family:inherit;margin-bottom:12px}
.chat-sess-list{display:flex;flex-direction:column;gap:2px}
.chat-sess{display:block;padding:8px 10px;border-radius:6px;color:#444;text-decoration:none;font-size:13px;line-height:1.35;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.chat-sess:hover{background:#f5f5f5}
.chat-sess.active{background:#eef0ff;color:#111;font-weight:600}
.chat-sess-empty{color:#999;font-size:13px;padding:8px 10px}
/* Stacking the layout turns align-items:flex-start into *horizontal*
   alignment, so children size to their content rather than the screen — a wide
   answer then pushed the column (and the input with it) past the viewport.
   Stretch them back to full width. */
@media(max-width:760px){.chat-layout{flex-direction:column;gap:12px;align-items:stretch}.chat-side,.chat-rail,.chat-main{width:100%;min-width:0;max-width:100%}.chat-sess-list{flex-direction:row;overflow-x:auto;flex-wrap:nowrap}.chat-sess{flex-shrink:0;max-width:160px}}
</style>`

// FormatAge returns a human-friendly string for an elapsed duration.
func FormatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// serveFlowPage renders a saved flow for viewing and sharing.
func serveFlowPage(w http.ResponseWriter, r *http.Request, id string) {
	f := getFlow(id)
	if f == nil {
		http.NotFound(w, r)
		return
	}

	// JSON polling endpoint for mobile recovery
	if app.WantsJSON(r) || r.URL.Query().Get("json") == "1" {
		app.RespondJSON(w, map[string]any{
			"id":     f.ID,
			"status": f.Status,
			"html":   f.HTML,
			"error":  f.Error,
			"prompt": f.Prompt,
		})
		return
	}

	var b strings.Builder

	// Show conversation history if this flow has a parent chain
	if f.ParentID != "" {
		history := getConversationHistory(f.ParentID, 5)
		for _, h := range history {
			b.WriteString(`<div class="card" style="border-left:3px solid #007bff;margin-bottom:8px;opacity:0.8;">`)
			b.WriteString(`<div style="font-size:12px;color:#888;margin-bottom:6px;">Previous question:</div>`)
			b.WriteString(`<div style="font-size:14px;font-weight:600;margin-bottom:10px;">` + htmlEsc(h.Prompt) + `</div>`)
			b.WriteString(`<div style="font-size:14px;">` + app.RenderString(h.Answer) + `</div>`)
			b.WriteString(`</div>`)
		}
	}

	b.WriteString(`<div class="card">`)
	b.WriteString(`<p style="font-size:12px;color:#888;margin:0 0 4px;">Saved query</p>`)
	b.WriteString(`<h3 style="margin:0 0 12px;">` + htmlEsc(f.Prompt) + `</h3>`)
	b.WriteString(`<p style="font-size:12px;color:#888;">` + f.CreatedAt.Format("2 January 2006, 15:04 UTC") + `</p>`)
	b.WriteString(`</div>`)

	// Render the stored answer
	if f.Answer != "" {
		rendered := app.RenderString(f.Answer)
		b.WriteString(`<div class="card" id="agent-response">` + rendered + `</div>`)
	}

	// Append typed cards from stored steps
	for _, step := range f.Steps {
		if card := renderResultCard(step.Tool, step.Result, step.Args); card != "" {
			b.WriteString(card)
		}
	}

	// References
	if len(f.Steps) > 0 {
		b.WriteString(`<div class="card" style="font-size:13px;"><h4 style="margin:0 0 8px;font-size:13px;color:#888;">References</h4>`)
		for _, step := range f.Steps {
			formatted := formatToolResult(step.Tool, step.Result, step.Args)
			b.WriteString(renderToolCallRef(step.Tool, step.Args, formatted))
		}
		b.WriteString(`</div>`)
	}

	// Actions — no card wrapper, just links
	b.WriteString(`<div style="display:flex;gap:12px;align-items:center;margin-top:12px;font-size:13px;">`)
	b.WriteString(`<a href="/agent?continue=` + f.ID + `">Continue →</a>`)
	b.WriteString(`<a href="#" onclick="var u=location.href;if(navigator.share){navigator.share({url:u})}else if(navigator.clipboard){navigator.clipboard.writeText(u).then(function(){this.textContent='Copied!'}.bind(this))}else{prompt('Copy:',u)};return false;">Share</a>`)
	b.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Agent", "Saved agent query: "+htmlEsc(f.Prompt), b.String(), r)
	w.Write([]byte(html))
}

// handleDeleteFlow handles DELETE /agent/flow/<id>.
func handleDeleteFlow(w http.ResponseWriter, r *http.Request, id string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}
	if err := deleteFlow(acc.ID, id); err != nil {
		http.Error(w, `{"error":"failed to delete flow"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sse writes a single Server-Sent Events data line and flushes.
func sse(w http.ResponseWriter, event map[string]any) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// streamNativeSSE drives the native go-micro agent and translates its stream
// into the SSE events the chat UI expects (tool_start/tool_done, stream_start/
// stream_token, response, done). It returns true when it handled the request.
// It returns false only when no native provider is configured, or the agent
// failed before producing any user-visible output — in which case nothing but
// the already-sent flow_id was emitted, so the caller can fall back to the
// hand-rolled pipeline cleanly.
func streamNativeSSE(w http.ResponseWriter, accountID, prompt string, opts QueryOpts, flow *Flow, isGuest bool) bool {
	streaming := false
	emitted := false
	var captured strings.Builder
	var nativeTools []string
	// The tool names behind those labels, for the run record.
	var nativeToolNames []string
	startedTools := map[string]bool{}
	endedTools := map[string]bool{}

	answer, handled, err := streamNative(accountID, prompt, opts, StreamHooks{
		ToolStart: func(label, name string) {
			if startedTools[label] {
				return
			}
			startedTools[label] = true
			emitted = true
			nativeTools = append(nativeTools, label)
			nativeToolNames = append(nativeToolNames, NativeToolName(name))
			sse(w, map[string]any{"type": "tool_start", "name": label, "message": label})
		},
		ToolEnd: func(label, name string) {
			if endedTools[label] {
				return
			}
			endedTools[label] = true
			sse(w, map[string]any{"type": "tool_done", "name": label, "message": label + " — done"})
		},
		Token: func(tok string) {
			if shouldHoldNativeNewsStreamTokens(prompt, nativeTools) {
				captured.WriteString(tok)
				return
			}
			if !streaming {
				streaming = true
				emitted = true
				sse(w, map[string]any{"type": "stream_start"})
			}
			captured.WriteString(tok)
			sse(w, map[string]any{"type": "stream_token", "token": tok})
		},
	})

	if !handled {
		return false // no native provider — fall back to the planner
	}
	if err != nil {
		if !emitted {
			// Nothing user-visible was sent yet (only flow_id); fall back to the
			// hand-rolled pipeline so the query still gets answered.
			app.Log("agent", "native stream failed before output, falling back: %v", err)
			return false
		}
		app.Log("agent", "native stream error mid-answer: %v", err)
		updateFlow(flow.ID, func(f *Flow) { f.Status = "error"; f.Error = err.Error() })
		sse(w, map[string]any{"type": "error", "message": "Could not generate response: " + err.Error()})
		sse(w, map[string]any{"type": "done"})
		return true
	}

	if answer == "" {
		answer = app.StripLatexDollars(captured.String())
	}
	answer = completeNativeToolAnswer(answer, nativeTools)
	answer = app.NormalizeAnswerMarkdown(answer)
	if !streaming && shouldReplayFinalNativeAnswer(prompt, nativeTools, captured.Len()) {
		streaming = true
		emitted = true
		sse(w, map[string]any{"type": "stream_start"})
		sse(w, map[string]any{"type": "stream_token", "token": answer})
	}
	rendered := app.RenderString(answer)
	html := `<div class="card" id="agent-response">` + rendered + `</div>`
	updateFlow(flow.ID, func(f *Flow) {
		f.Answer = answer
		f.HTML = html
		f.Status = "done"
		// What it ran. The native path streamed tool_start and tool_done to the
		// browser and then dropped both on the floor, so a finished run recorded
		// an answer and no account of how it got there — and this is the default
		// path, so that was almost every run. The names are what this path
		// knows; the payloads stay with the planner path, which keeps them.
		for _, name := range nativeToolNames {
			f.Steps = append(f.Steps, FlowStep{Tool: name})
		}
	})
	sse(w, map[string]any{"type": "response", "html": html, "flow_id": flow.ID})
	sse(w, map[string]any{"type": "done"})
	return true
}

// agentToolsDesc is the tool catalogue shown to the AI planner.
const agentToolsDesc = `Available tools (use exact name):
- news_headlines: Get recent news headlines + short summaries balanced across ALL topics (args optional: {"topic":"tech","limit":30}). PREFER this for general news, "what's happening" and morning-briefing requests so coverage isn't dominated by one topic like crypto. Then use news_read for any article worth expanding.
- news_read: Read one news article in full by its id from news_headlines (args: {"id":"<article id>"})
- news: Get the raw latest news feed grouped by category (no args) — only when you specifically need the full per-category feed
- news_search: Search news articles (args: {"query":"search term"})
- recall: Search across everything — indexed news, blog, social, video AND the user's own mail — for 'do you remember', 'what did I get about X', and cross-source lookups (args: {"query":"search term"}). Returns ids you can open with the matching tool.
- blog_list: Get recent blog posts (no args)
- blog_read: Read a specific blog post (args: {"id":"post-id"})
- social: View the social feed (no args)
- social_search: Search social posts (args: {"query":"search term"})
- video: Get the latest videos from curated channels (no args)
- video_search: Search YouTube for videos (args: {"query":"search term"})
- markets: Get live market prices (args: {"category":"crypto|futures|commodities"})
- weather_forecast: Get weather forecast (args: {"lat":number,"lon":number})
- mail_read: Read inbox messages (no args)
- mail_send: Send a message (args: {"to":"username or email","subject":"subject","body":"message"})
- chat: Chat with the AI (args: {"prompt":"message"})
- search: Search all Mu content (args: {"q":"search term"})
- web_search: Search the web for current information (args: {"q":"search term"})
- web_fetch: Fetch a web page and get its cleaned readable content (args: {"url":"https://example.com/page"})
- places_search: Search for places (args: {"q":"search name","near":"location"})
- places_nearby: Find places near a location (args: {"address":"location","radius":number})
- prayer_reflection: Get today's Islamic reflection — a verse of the Quran, a saying of the Prophet, and a name of Allah (no args) — response includes a "date" field, always mention it
- prayer_times: Today's Islamic prayer times for a location, and which prayer is next (args: {"lat":51.5,"lon":-0.12,"tz":"Europe/London"})
- prayer_qibla: The qibla — the compass bearing to face for prayer (args: {"lat":51.5,"lon":-0.12})
- quran: Look up a Quran chapter or verse (args: {"chapter":1,"verse":1} — verse is optional)
- hadith: Look up hadith from Sahih Al Bukhari (args: {"book":1} — optional book number)
- quran_search: Semantic search across the Quran, Hadith, and names of Allah (args: {"q":"what does the quran say about patience"})
- apps_search: Search apps directory (args: {"q":"search term","tag":"productivity"})
- apps_read: Read details of a specific app (args: {"slug":"app-slug"})
- apps_build: build a small app (a tracker, checklist, or counter) from a description (args: {"prompt":"an expense tracker"})
- apps_edit: Edit an existing app (args: {"slug":"app-slug","html":"<new html>","name":"New Name"})
- apps_run: Run JavaScript code and return the result (args: {"code":"return 2+2"})
- wallet_balance: Check your balance — credits, plus your Base address and USDC for topping up (no args). To add credits, point the user at /wallet/topup; there is no tool for it.
- stream: Read the public event stream (no args)`

const guestToolsDesc = `Available tools (use exact name):
- news_headlines: Get recent news headlines + short summaries balanced across ALL topics (args optional: {"topic":"tech","limit":30}). PREFER this for general news and briefing requests so coverage isn't dominated by one topic like crypto. Then use news_read for any article worth expanding.
- news_read: Read one news article in full by its id from news_headlines (args: {"id":"<article id>"})
- news: Get the raw latest news feed grouped by category (no args)
- news_search: Search news articles (args: {"query":"search term"})
- recall: Search across indexed news, blog, social and video for cross-source lookups (args: {"query":"search term"})
- blog_list: Get recent blog posts (no args)
- blog_read: Read a specific blog post (args: {"id":"post-id"})
- social: View the social feed (no args)
- social_search: Search social posts (args: {"query":"search term"})
- video: Get the latest videos from curated channels (no args)
- video_search: Search YouTube for videos (args: {"query":"search term"})
- markets: Get live market prices (args: {"category":"crypto|futures|commodities"})
- weather_forecast: Get weather forecast (args: {"lat":number,"lon":number})
- places_search: Search for places (args: {"q":"search name","near":"location"})
- places_nearby: Find places near a location (args: {"address":"location","radius":number})
- prayer_reflection: Get today's Islamic reflection (no args)
- prayer_times: Today's Islamic prayer times for a location (args: {"lat":51.5,"lon":-0.12})
- quran: Look up a Quran chapter or verse (args: {"chapter":1,"verse":1})
- hadith: Look up hadith from Sahih Al Bukhari (args: {"book":1})
- quran_search: Search the Quran and Hadith (args: {"q":"search term"})
- search: Search all Mu content (args: {"q":"search term"})
- web_search: Search the web (args: {"q":"search term"})
- web_fetch: Fetch a web page (args: {"url":"https://example.com"})
- stream: Read the public event stream (no args)`

// handleQuery processes an agent query request with SSE streaming.
func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt    string `json:"prompt"`
		Model     string `json:"model"`
		Agent     string `json:"agent"`      // optional: user-defined agent id to answer as
		ContextID string `json:"context_id"` // optional: prior flow to continue from
		// Cards asks for the reader's home cards to be included as context, so
		// a question about what they watch is answered from what is already
		// known rather than fetched again.
		Cards bool `json:"cards"`
		// History is an optional client-supplied conversation thread used by
		// the inline chat (landing + assistant). It gives multi-turn context
		// without server-side persistence, so guests get follow-up memory too.
		History []struct {
			Prompt string `json:"prompt"`
			Answer string `json:"answer"`
		} `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, `{"error":"prompt required"}`, http.StatusBadRequest)
		return
	}

	_, acc := auth.TrySession(r)
	isGuest := acc == nil

	if isGuest {
		ip := app.ClientIP(r)
		if !guestQueryAllowed(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Sign up to keep using the AI agent. 3 free queries per day."}`))
			return
		}
		guestQueryRecord(ip)
	}

	// Resolve model
	model := Models[0] // default: standard
	for _, m := range Models {
		if m.ID == req.Model {
			model = m
			break
		}
	}

	// Check wallet quota (authenticated users only), then charge up-front: the
	// agent is our most expensive op, so it's metered like web_fetch and chat.
	if !isGuest && QuotaCheck != nil {
		canProceed, _, err := QuotaCheck(r, model.WalletOp)
		if !canProceed {
			msg := "Insufficient credits for agent query. Top up at /wallet/topup."
			if err != nil {
				msg = err.Error()
			}
			http.Error(w, `{"error":"`+msg+`"}`, http.StatusPaymentRequired)
			return
		}
		if ChargeQuota != nil {
			ChargeQuota(r, model.WalletOp)
		}
	}

	// Load conversation history when continuing a conversation. A saved flow
	// (context_id) takes precedence; otherwise fall back to the client-supplied
	// inline history used by the stateless inline chat.
	var conversationHistory []*Flow
	if req.ContextID != "" {
		conversationHistory = getConversationHistory(req.ContextID, 5)
	}
	if len(conversationHistory) == 0 && len(req.History) > 0 {
		const maxTurns = 6
		hist := req.History
		if len(hist) > maxTurns {
			hist = hist[len(hist)-maxTurns:]
		}
		for _, h := range hist {
			if strings.TrimSpace(h.Prompt) == "" {
				continue
			}
			ans := h.Answer
			if len(ans) > 1500 {
				ans = ans[:1500] + "…"
			}
			conversationHistory = append(conversationHistory, &Flow{Prompt: h.Prompt, Answer: ans})
		}
	}

	// Create flow early so progress is saved server-side even if the
	// SSE connection drops (e.g. mobile browser suspends the tab).
	accountID := ""
	if acc != nil {
		accountID = acc.ID
	}
	flow := &Flow{
		ID:        newFlowID(),
		AccountID: accountID,
		Prompt:    req.Prompt,
		Status:    "running",
		Agent:     req.Agent,
		ParentID:  req.ContextID,
		CreatedAt: time.Now().UTC(),
	}
	if !isGuest {
		if err := saveFlow(flow); err != nil {
			app.Log("agent", "Failed to create flow: %v", err)
		}
		// Notice anything worth remembering. This ran only on /agent/run — the
		// REST and MCP path — so an agent remembered what a program told it and
		// forgot everything a person did, which is backwards: the chat is where
		// somebody says "I'm in London, keep it short". Cheap: one background
		// model call, off the response path, and it stores nothing when the
		// message holds no fact.
		go extractMemory(accountID, req.Prompt)
	}

	// Start SSE stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send flow_id immediately so the client can recover on disconnect.
	sse(w, map[string]any{"type": "flow_id", "flow_id": flow.ID})

	// Native go-micro agent path (opt-in via AGENT_NATIVE_STREAM while the
	// upstream StreamAsk tool-name bug is fixed): the LLM does native
	// tool-calling over the registered services and streams the answer,
	// emitting tool start/end events. Falls through to the hand-rolled
	// pipeline below if disabled, no provider is configured, or it fails
	// before producing any output.
	// Guest starter/live-data prompts with deterministic tool shortcuts should
	// skip the native agent's initial model planning turn. The hand-rolled
	// pipeline below can start the relevant tool immediately, which improves
	// first visible progress for the first-run core loop without changing any
	// public endpoint or tool contract.
	preferPlanner := isGuest && (len(shortcutToolCalls(req.Prompt)) > 0 || isSimpleWeatherPrompt(req.Prompt))
	if nativeStreamEnabled() && !preferPlanner {
		nopts := QueryOpts{Public: isGuest}
		if req.Cards && !isGuest && CardContextFunc != nil {
			nopts.Extra = CardContextFunc(accountID)
		}
		if ua := resolveAgent(accountID, req.Agent, isGuest); ua != nil {
			nopts.System = ua.SystemPrompt
			nopts.Tools = ua.Tools
		}
		for _, f := range conversationHistory {
			if strings.TrimSpace(f.Prompt) == "" {
				continue
			}
			nopts.History = append(nopts.History,
				QueryMessage{Role: "user", Text: f.Prompt},
				QueryMessage{Role: "assistant", Text: f.Answer})
		}
		if streamNativeSSE(w, accountID, req.Prompt, nopts, flow, isGuest) {
			return
		}
	}

	// --- Step 1: plan tool calls ---
	type toolCall struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	var toolCalls []toolCall

	// Shortcut: skip planning LLM for common queries with known tool mappings
	if tc := shortcutToolCalls(req.Prompt); len(tc) > 0 {
		for _, s := range tc {
			toolCalls = append(toolCalls, toolCall{Tool: s.Tool, Args: s.Args})
		}
		sse(w, map[string]any{"type": "thinking", "message": "Fetching data…"})
	} else {
		sse(w, map[string]any{"type": "thinking", "message": "Planning your request…"})

		// Build planning question with conversation context
		planQuestion := req.Prompt
		if len(conversationHistory) > 0 {
			var convCtx strings.Builder
			convCtx.WriteString("Previous conversation:\n")
			for _, f := range conversationHistory {
				convCtx.WriteString(fmt.Sprintf("User: %s\nAssistant: %s\n\n", f.Prompt, truncate(f.Answer, 500)))
			}
			convCtx.WriteString("New question: " + req.Prompt)
			planQuestion = convCtx.String()
		}
		toolsForPlan := agentToolsDesc
		if isGuest {
			toolsForPlan = guestToolsDesc
		}
		planPrompt := &ai.Prompt{
			System: "You are an AI agent. Given a user question, output ONLY a JSON array of tool calls (no other text, no markdown).\n\n" +
				toolsForPlan +
				"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tool calls. When the question asks for cross-source insights or correlations (e.g. news + markets, news + video), call multiple relevant tools. If the question is a follow-up that can be answered from prior conversation context without new tools, output []. If no tools are needed output [].",
			Question: planQuestion,
			Priority: ai.PriorityHigh,
			Provider: "",
			Model:    ai.BackgroundModel(),
			Caller:   "agent-plan",
		}

		planResult, err := ai.Ask(planPrompt)
		if err != nil {
			updateFlow(flow.ID, func(f *Flow) { f.Status = "error"; f.Error = err.Error() })
			sse(w, map[string]any{"type": "error", "message": "Could not plan request: " + err.Error()})
			sse(w, map[string]any{"type": "done"})
			return
		}

		// Parse tool calls (the AI may wrap JSON in markdown fences)
		planJSON := extractJSONArray(planResult)
		json.Unmarshal([]byte(planJSON), &toolCalls) //nolint:errcheck — fallback to empty slice
	}

	// --- Step 2: execute tool calls ---
	type toolResult struct {
		Name      string
		Result    string
		Args      map[string]any
		Formatted string // pre-formatted RAG text, also used for reference rendering
	}
	var results []toolResult
	var unavailableTools []string
	seenToolCalls := map[string]bool{}

	for i := 0; i < len(toolCalls); i++ {
		tc := toolCalls[i]
		if tc.Tool == "" {
			continue
		}
		key := toolCallKey(tc.Tool, tc.Args)
		if seenToolCalls[key] {
			continue
		}
		seenToolCalls[key] = true
		if skipMarketMoverCompanionTool(req.Prompt, tc.Tool) {
			continue
		}
		if isGuest && !isGuestAllowedTool(tc.Tool) {
			continue
		}
		msg := toolLabel(tc.Tool)
		sse(w, map[string]any{"type": "tool_start", "name": tc.Tool, "message": msg})

		text, isErr, execErr := api.ExecuteTool(r, tc.Tool, tc.Args)
		if execErr != nil || isErr {
			app.Log("agent", "Tool %s failed: err=%v isErr=%v response=%.200s", tc.Tool, execErr, isErr, text)
			unavailableTools = append(unavailableTools, tc.Tool)
			sse(w, map[string]any{
				"type":    "tool_done",
				"name":    tc.Tool,
				"message": tc.Tool + " — unavailable",
			})
			if fallback, ok := fallbackNewsSearchToolCall(req.Prompt, tc.Tool, tc.Args); ok {
				key := toolCallKey(fallback.Tool, fallback.Args)
				if !seenToolCalls[key] && (!isGuest || isGuestAllowedTool(fallback.Tool)) {
					toolCalls = append(toolCalls, toolCall{Tool: fallback.Tool, Args: fallback.Args})
				}
			}
			continue
		}

		// Cap context length passed to the synthesiser
		if len(text) > 8000 {
			text = text[:8000] + "…"
		}
		results = append(results, toolResult{Name: tc.Tool, Result: text, Args: tc.Args})

		// Save step to flow incrementally
		updateFlow(flow.ID, func(f *Flow) {
			f.Steps = append(f.Steps, FlowStep{Tool: tc.Tool, Args: tc.Args, Result: text})
		})

		sse(w, map[string]any{
			"type":    "tool_done",
			"name":    tc.Tool,
			"message": msg + " — done",
		})
	}

	// --- Step 3: synthesise response ---
	sse(w, map[string]any{"type": "thinking", "message": "Composing answer…"})

	var ragParts []string

	// Include conversation history when continuing a conversation.
	if len(conversationHistory) > 0 {
		var convCtx strings.Builder
		convCtx.WriteString("### Conversation history\n")
		for i, f := range conversationHistory {
			convCtx.WriteString(fmt.Sprintf("**Turn %d — User asked:** %s\n\n", i+1, f.Prompt))
			convCtx.WriteString(f.Answer + "\n\n")
		}
		ragParts = append(ragParts, convCtx.String())
	}

	for i, res := range results {
		ragText := formatToolResult(res.Name, res.Result, res.Args)
		results[i].Formatted = ragText
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", res.Name, ragText))
	}
	for _, tool := range unavailableTools {
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", tool, unavailableToolMessage(tool)))
	}

	hasMarketsTool := false
	hasWeatherTool := false
	hasWebSearchTool := false
	hasNewsSearchTool := false
	for _, tc := range toolCalls {
		switch tc.Tool {
		case "markets", "markets_list":
			hasMarketsTool = true
		case "weather_forecast":
			hasWeatherTool = true
		case "web_search", "search_web":
			hasWebSearchTool = true
		case "news_search":
			hasNewsSearchTool = true
		}
	}
	hasUnavailableNewsSearch := false
	for _, tool := range unavailableTools {
		if tool == "news_search" {
			hasUnavailableNewsSearch = true
			break
		}
	}
	if useFastToolFallback(req.Prompt, isGuest, hasMarketsTool, hasWeatherTool, hasWebSearchTool, hasNewsSearchTool, hasUnavailableNewsSearch, ragParts) {
		answer := app.NormalizeAnswerMarkdown(app.StripLatexDollars(synthesizeToolFallback(ragParts)))
		rendered := app.RenderString(answer)
		html := `<div class="card" id="agent-response">` + rendered + `</div>`
		for _, res := range results {
			if card := renderResultCard(res.Name, res.Result, res.Args); card != "" {
				html += card
			}
		}
		if len(results) > 0 {
			html += `<div class="card" style="font-size:13px;"><h4 style="margin:0 0 8px;font-size:13px;color:#888;">References</h4>`
			for _, res := range results {
				html += renderToolCallRef(res.Name, res.Args, res.Formatted)
			}
			html += `</div>`
		}
		updateFlow(flow.ID, func(f *Flow) {
			f.Answer = answer
			f.HTML = html
			f.Status = "done"
		})
		sse(w, map[string]any{"type": "stream_start"})
		sse(w, map[string]any{"type": "stream_token", "token": answer})
		sse(w, map[string]any{"type": "response", "html": html, "flow_id": flow.ID})
		sse(w, map[string]any{"type": "done"})
		return
	}

	today := currentDateContext(time.Now().UTC())

	var synthSystem string
	if len(results) == 0 {
		synthSystem = "You are Micro, the agent on Mu at micro.mu. Today's date is " + today + ".\n\n" +
			"Mu is the everyday internet as tools — news, mail, web search, weather, video, markets, storage — that you call on the user's behalf, and that agents can call directly over MCP. " +
			"You check their mail, look up prices, search the web, read the news, and give a personalised answer. " +
			"Everything is behind one account and one balance, instead of a signup, a card and a token for every provider. It is open and self-hostable as a single binary. " +
			"No ads, no tracking, no algorithm. Pay for the tools, not with your attention.\n\n" +
			"Answer the user's question conversationally. Be helpful and concise. Use markdown formatting.\n\n" +
			"IMPORTANT: Use plain dollar signs for currency (e.g. $69,811). Do NOT use LaTeX math delimiters like \\( or \\)."
	} else {
		synthSystem = "You are a helpful assistant. Today's date is " + today + ". " +
			"The tool results below come from live data feeds. For prompts about today, latest, current, or now, anchor the answer to the current date above; do not label today as a different date unless a provider timestamp explicitly proves staleness, and disclose that staleness. Use article publication dates when reasoning about recency. For weather, use the dated forecast rows exactly and never invent calendar dates or weekdays; if the date is ambiguous or absent, say so.\n\n" +
			"Answer the user's question directly, as user-facing prose. Start with the answer, not with process narration such as \"Here's what I found\" or \"Based on the tool results\". Do not use internal tool names or implementation headings like weather_forecast, markets, or Web sources in the primary answer; translate them to natural labels only when needed. Preserve useful freshness, source, provider-unavailable, and link details.\n\n" +
			"For web_search results, preserve the user's query intent exactly, cite the listed source URLs, and if confidence is low say the results do not clearly support the requested answer and ask for a refinement.\n\n" +
			"For news results, include the article URL next to each headline whenever the tool result provides one; if a headline has no URL, do not invent one.\n\n" +
			"IMPORTANT: For any prices, market values, weather conditions, or other real-time data, you MUST use " +
			"the exact values from the tool results. Do NOT use your training knowledge for current prices or live data — " +
			"it will be outdated. If no tool result contains the requested real-time data, say it is unavailable.\n\n" +
			"For market-mover prompts, prioritize the largest 24h movers and their prices first. Keep the answer to a brief bullets-only summary unless the user asks for depth. Only mention news when the tool results include directly explanatory headlines or sources; do not pad a market-mover answer with general market/news commentary.\n\n" +
			"When results come from multiple sources (news, video, markets, weather, etc.), identify and highlight " +
			"connections and correlations between them — for example, how a market move relates to a news story, " +
			"or how videos cover the same topic appearing in the news.\n\n" +
			"Use markdown formatting. Summarise key information from any news articles, weather data, market prices or other structured data. " +
			"Do not answer with progress narration like 'let me check' or 'I'll pull the data'; the tools have already run, so provide the final answer or say exactly what is unavailable.\n\n" +
			"IMPORTANT: Use plain dollar signs for currency (e.g. $69,811). Do NOT use LaTeX math delimiters like \\( or \\)."
	}

	// And the same in the streaming fallback. This branch runs whenever the
	// native path is off or gave up before producing output, and it applied no
	// named agent at all: it recorded the id on the flow and then composed as
	// Micro, so an agent whose whole point is a voice or a standing instruction
	// lost both the moment a question needed a tool.
	customAgent := false
	if ua := resolveAgent(accountID, req.Agent, isGuest); ua != nil &&
		strings.TrimSpace(ua.SystemPrompt) != "" {
		synthSystem = strings.TrimSpace(ua.SystemPrompt) + "\n\n" + synthSystem
		customAgent = true
	}

	synthPrompt := &ai.Prompt{
		System:   synthSystem,
		Rag:      ragParts,
		Question: req.Prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
		Caller:   "agent-synth",
	}

	// Stream tokens to the client as they arrive from the LLM.
	sse(w, map[string]any{"type": "stream_start"})

	answer, err := ai.AskStream(synthPrompt, func(token string) {
		sse(w, map[string]any{"type": "stream_token", "token": token})
	})
	if err != nil {
		updateFlow(flow.ID, func(f *Flow) { f.Status = "error"; f.Error = err.Error() })
		// The tools have already run and their results are in hand. Discarding
		// them because the model could not write the prose around them gets the
		// trade backwards: calling the tools is the expensive, fallible half and
		// it succeeded — the user asked for the weather and we have the weather.
		// So say the summary failed and show what came back.
		var raw strings.Builder
		for _, res := range results {
			if card := renderResultCard(res.Name, res.Result, res.Args); card != "" {
				raw.WriteString(card)
				continue
			}
			text := res.Formatted
			if strings.TrimSpace(text) == "" {
				text = res.Result
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			raw.WriteString(`<div class="card"><h4>` + htmlpkg.EscapeString(strings.ReplaceAll(res.Name, "_", " ")) +
				`</h4><pre style="white-space:pre-wrap;margin:0">` + htmlpkg.EscapeString(text) + `</pre></div>`)
		}
		if raw.Len() > 0 {
			sse(w, map[string]any{"type": "error",
				"message": "Could not write a summary (" + err.Error() + "). Here is what the tools returned."})
			sse(w, map[string]any{"type": "response", "flow_id": flow.ID, "html": raw.String()})
			sse(w, map[string]any{"type": "done"})
			return
		}
		sse(w, map[string]any{"type": "error", "message": "Could not generate response: " + err.Error()})
		sse(w, map[string]any{"type": "done"})
		return
	}
	answer = app.NormalizeAnswerMarkdown(app.StripLatexDollars(answer))
	// Told whether a named agent wrote this, so the freshness guard does not
	// replace its answer with a list of the raw results.
	answer = completeToolAnswerFor(answer, ragParts, customAgent)
	answer = app.NormalizeAnswerMarkdown(answer)

	rendered := app.RenderString(answer)
	html := `<div class="card" id="agent-response">` + rendered + `</div>`

	for _, res := range results {
		if card := renderResultCard(res.Name, res.Result, res.Args); card != "" {
			html += card
		}
	}

	if len(results) > 0 {
		html += `<div class="card" style="font-size:13px;"><h4 style="margin:0 0 8px;font-size:13px;color:#888;">References</h4>`
		for _, res := range results {
			html += renderToolCallRef(res.Name, res.Args, res.Formatted)
		}
		html += `</div>`
	}

	updateFlow(flow.ID, func(f *Flow) {
		f.Answer = answer
		f.HTML = html
		f.Status = "done"
	})

	sse(w, map[string]any{"type": "response", "html": html, "flow_id": flow.ID})
	sse(w, map[string]any{"type": "done"})
}

// shortcutToolCall defines a pre-planned tool call for common queries.
type shortcutToolCall struct {
	Tool string
	Args map[string]any
}

// shortcutToolCalls returns pre-planned tool calls for exact-match aliases,
// skipping the LLM planning step for common one-word queries and starter pills.
func shortcutToolCalls(prompt string) []shortcutToolCall {
	aliases := map[string][]shortcutToolCall{
		// Short aliases
		"news":          {{Tool: "news_headlines", Args: map[string]any{}}},
		"markets":       {{Tool: "markets", Args: map[string]any{}}},
		"market":        {{Tool: "markets", Args: map[string]any{}}},
		"prices":        {{Tool: "markets", Args: map[string]any{}}},
		"video":         {{Tool: "video", Args: map[string]any{}}},
		"videos":        {{Tool: "video", Args: map[string]any{}}},
		"latest videos": {{Tool: "video", Args: map[string]any{}}},
		"latest video":  {{Tool: "video", Args: map[string]any{}}},
		"reminder":      {{Tool: "prayer_reflection", Args: map[string]any{}}},
		"apps":          {{Tool: "apps_search", Args: map[string]any{}}},
		"mail":          {{Tool: "mail_read", Args: map[string]any{}}},
		// Personal queries
		"do i have mail":                   {{Tool: "mail_read", Args: map[string]any{}}},
		"do i have unread mail":            {{Tool: "mail_read", Args: map[string]any{}}},
		"do i have email":                  {{Tool: "mail_read", Args: map[string]any{}}},
		"check my mail":                    {{Tool: "mail_read", Args: map[string]any{}}},
		"check my email":                   {{Tool: "mail_read", Args: map[string]any{}}},
		"any new mail":                     {{Tool: "mail_read", Args: map[string]any{}}},
		"any new email":                    {{Tool: "mail_read", Args: map[string]any{}}},
		"any mail":                         {{Tool: "mail_read", Args: map[string]any{}}},
		"unread mail":                      {{Tool: "mail_read", Args: map[string]any{}}},
		"unread email":                     {{Tool: "mail_read", Args: map[string]any{}}},
		"read my mail":                     {{Tool: "mail_read", Args: map[string]any{}}},
		"read my email":                    {{Tool: "mail_read", Args: map[string]any{}}},
		"read my unread email":             {{Tool: "mail_read", Args: map[string]any{}}},
		"read my unread emails":            {{Tool: "mail_read", Args: map[string]any{}}},
		"my mail":                          {{Tool: "mail_read", Args: map[string]any{}}},
		"my email":                         {{Tool: "mail_read", Args: map[string]any{}}},
		"btc price":                        {{Tool: "markets", Args: map[string]any{"category": "crypto"}}},
		"bitcoin price":                    {{Tool: "markets", Args: map[string]any{"category": "crypto"}}},
		"eth price":                        {{Tool: "markets", Args: map[string]any{"category": "crypto"}}},
		"what's happening":                 {{Tool: "news_headlines", Args: map[string]any{}}},
		"what's happening?":                {{Tool: "news_headlines", Args: map[string]any{}}},
		"today's news":                     {{Tool: "news_headlines", Args: map[string]any{}}},
		"what is moving in markets today":  {{Tool: "markets", Args: map[string]any{}}},
		"what is moving in markets today?": {{Tool: "markets", Args: map[string]any{}}},
		"what's moving in markets today":   {{Tool: "markets", Args: map[string]any{}}},
		"what's moving in markets today?":  {{Tool: "markets", Args: map[string]any{}}},
		"market movers today":              {{Tool: "markets", Args: map[string]any{}}},
		"markets movers today":             {{Tool: "markets", Args: map[string]any{}}},
		// Starter pill phrases
		"give me a summary of today's top news":         {{Tool: "news_headlines", Args: map[string]any{}}},
		"what's in the news?":                           {{Tool: "news_headlines", Args: map[string]any{}}},
		"what are the latest crypto and market prices?": {{Tool: "markets", Args: map[string]any{}}},
		"find me the latest tech videos":                {{Tool: "video_search", Args: map[string]any{"query": "tech"}}},
		"search the web for the latest ai news":         {{Tool: "web_search", Args: map[string]any{"q": "latest AI news"}}},
		"show me today's islamic reminder":              {{Tool: "prayer_reflection", Args: map[string]any{}}},
		// Wallet
		"my wallet":      {{Tool: "wallet_balance", Args: map[string]any{}}},
		"wallet":         {{Tool: "wallet_balance", Args: map[string]any{}}},
		"wallet balance": {{Tool: "wallet_balance", Args: map[string]any{}}},
		"wallet address": {{Tool: "wallet_balance", Args: map[string]any{}}},
	}
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if tc, ok := aliases[lower]; ok {
		return tc
	}

	// Fuzzy matches for prompts with dynamic content. Simple market-mover
	// prompts should reach the markets tool without an LLM planning turn so the
	// first visible tool event is emitted as soon as possible. Explanation or
	// cross-source requests still use the planner so they can add news/search.
	if isMarketMoverPrompt(lower) && !wantsMarketMoverExplanation(lower) {
		return []shortcutToolCall{{Tool: "markets", Args: map[string]any{}}}
	}

	if tc := weatherShortcutToolCalls(lower); len(tc) > 0 {
		return tc
	}

	if isLatestTechnologyNewsPrompt(lower) {
		return []shortcutToolCall{{Tool: "news_search", Args: map[string]any{"query": newsTopicQuery(lower)}}}
	}

	if strings.Contains(lower, "unread email") || strings.Contains(lower, "unread mail") ||
		(strings.Contains(lower, "read") && strings.Contains(lower, "mail")) ||
		(strings.Contains(lower, "read") && strings.Contains(lower, "email")) {
		return []shortcutToolCall{{Tool: "mail_read", Args: map[string]any{}}}
	}

	return nil
}

func weatherShortcutToolCalls(lower string) []shortcutToolCall {
	if !isSimpleWeatherPrompt(lower) {
		return nil
	}
	locationAliases := map[string][2]float64{
		"new york":       {40.7128, -74.0060},
		"nyc":            {40.7128, -74.0060},
		"san francisco":  {37.7749, -122.4194},
		"sf":             {37.7749, -122.4194},
		"london":         {51.5074, -0.1278},
		"paris":          {48.8566, 2.3522},
		"tokyo":          {35.6762, 139.6503},
		"los angeles":    {34.0522, -118.2437},
		"chicago":        {41.8781, -87.6298},
		"miami":          {25.7617, -80.1918},
		"seattle":        {47.6062, -122.3321},
		"washington dc":  {38.9072, -77.0369},
		"washington, dc": {38.9072, -77.0369},
	}
	for alias, coords := range locationAliases {
		if containsLocationAlias(lower, alias) {
			return []shortcutToolCall{{Tool: "weather_forecast", Args: map[string]any{"lat": coords[0], "lon": coords[1]}}}
		}
	}
	return nil
}

func containsLocationAlias(prompt, alias string) bool {
	if !strings.Contains(prompt, alias) {
		return false
	}
	for _, marker := range []string{" in " + alias, " for " + alias, " at " + alias, " near " + alias} {
		if strings.Contains(prompt, marker) {
			return true
		}
	}
	return strings.TrimSpace(prompt) == "weather "+alias || strings.TrimSpace(prompt) == alias+" weather"
}

func fallbackNewsSearchToolCall(prompt, tool string, args map[string]any) (shortcutToolCall, bool) {
	if tool != "news_search" || !isLatestTechnologyNewsPrompt(strings.ToLower(prompt)) {
		return shortcutToolCall{}, false
	}
	query := newsTopicQuery(strings.ToLower(prompt))
	if raw, ok := args["query"].(string); ok && strings.TrimSpace(raw) != "" {
		query = strings.TrimSpace(raw)
	}
	if promptQuery := newsTopicQuery(strings.ToLower(prompt)); promptQuery != "technology news" && query == "technology news" {
		query = promptQuery
	}
	return shortcutToolCall{Tool: "web_search", Args: map[string]any{"q": query}}, true
}

func newsTopicQuery(lower string) string {
	prefix := ""
	switch {
	case strings.Contains(lower, "today"):
		prefix = "today "
	case strings.Contains(lower, "latest"):
		prefix = "latest "
	case strings.Contains(lower, "current") || strings.Contains(lower, "happening"):
		prefix = "current "
	}
	topic := "technology news"
	if strings.Contains(lower, "artificial intelligence") {
		topic = "artificial intelligence news"
	} else {
		for _, token := range strings.FieldsFunc(lower, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		}) {
			if token == "ai" {
				topic = "AI news"
				break
			}
		}
	}
	return prefix + topic
}

func isLatestTechnologyNewsPrompt(lower string) bool {
	hasRecency := strings.Contains(lower, "latest") ||
		strings.Contains(lower, "today") ||
		strings.Contains(lower, "current") ||
		strings.Contains(lower, "happening")
	if !hasRecency || !strings.Contains(lower, "news") {
		return false
	}
	for _, topic := range []string{"tech", "technology", "ai", "artificial intelligence"} {
		if strings.Contains(lower, topic) {
			return true
		}
	}
	return false
}

func useFastToolFallback(prompt string, isGuest bool, hasMarketsTool bool, hasWeatherTool bool, hasWebSearchTool bool, hasNewsSearchTool bool, hasUnavailableNewsSearch bool, ragParts []string) bool {
	if !isGuest || len(ragParts) == 0 {
		return false
	}
	if hasMarketsTool && isMarketMoverPrompt(prompt) && !wantsMarketMoverExplanation(prompt) {
		return true
	}
	if hasWeatherTool && isSimpleWeatherPrompt(prompt) {
		return true
	}
	if isLatestTechnologyNewsPrompt(strings.ToLower(prompt)) {
		return hasNewsSearchTool || (hasWebSearchTool && hasUnavailableNewsSearch)
	}
	return false
}

func shouldReplayFinalNativeAnswer(prompt string, nativeTools []string, capturedLen int) bool {
	if capturedLen > 0 {
		return true
	}
	// streamNative buffers stale-news tokens internally once the tool payload
	// proves a freshness caveat is needed. In that case streamNativeSSE's local
	// capture is empty, so replay the guarded final answer as the first streamed
	// text instead of relying only on the later response replacement event.
	return shouldHoldNativeNewsStreamTokens(prompt, nativeTools)
}

func shouldHoldNativeNewsStreamTokens(prompt string, nativeTools []string) bool {
	if !isLatestTechnologyNewsPrompt(strings.ToLower(prompt)) {
		return false
	}
	// Latest-news prompts are sensitive to recency caveats: the native model can
	// emit answer text before the news.Search/dotted news tool payload has been
	// recorded and guarded. Hold the stream from the first token, then emit the
	// final guarded answer once completeToolAnswer has had a chance to prepend
	// stale/mostly-stale disclosures.
	if len(nativeTools) == 0 {
		return true
	}
	for _, tool := range nativeTools {
		lowerTool := strings.ToLower(strings.TrimSpace(tool))
		if canonicalToolTitle(lowerTool) == "news" || strings.Contains(lowerTool, "news") || strings.Contains(lowerTool, "headline") {
			return true
		}
	}
	return false
}

func isSimpleWeatherPrompt(prompt string) bool {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if lower == "" || !strings.Contains(lower, "weather") {
		return false
	}
	complexTerms := []string{"compare", "versus", " vs ", "and news", "market", "markets", "why", "explain", "impact", "affect"}
	for _, term := range complexTerms {
		if strings.Contains(lower, term) {
			return false
		}
	}
	return true
}

func toolCallKey(tool string, args map[string]any) string {
	if len(args) == 0 {
		return strings.TrimSpace(tool)
	}
	b, err := json.Marshal(args)
	if err != nil {
		return strings.TrimSpace(tool)
	}
	return strings.TrimSpace(tool) + "\x00" + string(b)
}

// extractJSONArray extracts the first JSON array `[…]` from text produced by the AI.
// skipMarketMoverCompanionTool keeps market-mover answers focused on price
// data unless the user explicitly asks for explanatory news or cross-source
// correlation. Planning can otherwise add broad news tools for "today" prompts,
// which lets unrelated headlines bleed into a simple movers answer.
func skipMarketMoverCompanionTool(prompt, tool string) bool {
	if tool == "markets" || tool == "markets_list" || !isMarketMoverPrompt(prompt) || wantsMarketMoverExplanation(prompt) {
		return false
	}
	return tool == "news" || tool == "news_headlines" || tool == "news_list" || tool == "news_search" || tool == "web_search" || tool == "search_web" || tool == "recall" || tool == "index"
}

func isMarketMoverPrompt(prompt string) bool {
	lower := strings.ToLower(prompt)
	hasMoveIntent := strings.Contains(lower, "moving") ||
		strings.Contains(lower, "mover") ||
		strings.Contains(lower, "move") ||
		strings.Contains(lower, "rally") ||
		strings.Contains(lower, "selloff") ||
		strings.Contains(lower, "up today") ||
		strings.Contains(lower, "down today")
	if !hasMoveIntent {
		return false
	}
	for _, term := range []string{"market", "markets", "stock", "stocks", "equity", "equities", "crypto", "bitcoin", "btc", "eth", "ethereum", "index", "indices", "futures"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func wantsMarketMoverExplanation(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, term := range []string{"why", "because", "explain", "reason", "driving", "driver", "catalyst", "news", "headline", "correlat"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func extractJSONArray(text string) string {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start == -1 || end == -1 || end <= start {
		return "[]"
	}
	return text[start : end+1]
}

// toolLabel returns a human-readable progress label for a tool name.
func toolLabel(tool string) string {
	switch tool {
	case "news":
		return "📰 Reading latest news"
	case "news_headlines", "news_list":
		return "📰 Scanning headlines"
	case "news_read":
		return "📖 Reading article"
	case "news_search":
		return "Searching news"
	case "recall", "index":
		return "🧠 Searching your world"
	case "web_search", "search_web":
		return "🌐 Searching the web"
	case "web_fetch", "search_fetch":
		return "Fetching web page"
	case "video_search":
		return "🎬 Searching videos"
	case "markets", "markets_list":
		return "📈 Checking market prices"
	case "weather_forecast":
		return "🌤 Getting weather forecast"
	case "places_search":
		return "📍 Searching places"
	case "places_nearby":
		return "📍 Finding nearby places"
	case "prayer_reflection", "islam_today", "islam":
		return "📿 Getting today's reflection"
	case "search":
		return "Searching Mu"
	case "blog_list":
		return "📝 Reading blog posts"
	case "wallet_balance":
		return "💳 Checking wallet balance"
	case "apps_search":
		return "📱 Searching apps"
	case "apps_read":
		return "📱 Reading app"
	case "apps_build":
		return "🔨 Building app"
	case "apps_edit":
		return "✏️ Editing app"
	case "apps_run":
		return "⚡ Running code"
	default:
		return "⚙ Calling " + tool
	}
}

// renderToolCallRef renders a collapsible <details> element showing the tool
// name with arguments and the formatted result text, for use as a reference
// alongside the agent's synthesised answer.
func renderToolCallRef(name string, args map[string]any, formattedResult string) string {
	label := toolLabel(name)
	if args != nil {
		if q, ok := args["query"].(string); ok && q != "" {
			label += ` — "` + htmlEsc(q) + `"`
		} else if q, ok := args["q"].(string); ok && q != "" {
			label += ` — "` + htmlEsc(q) + `"`
		} else if cat, ok := args["category"].(string); ok && cat != "" {
			label += ` — ` + htmlEsc(cat)
		}
	}
	return `<details style="margin-bottom:4px;">` +
		`<summary style="cursor:pointer;color:#555;font-size:13px;list-style:none;padding:4px 0;">` +
		label + `</summary>` +
		`<pre style="margin:6px 0 0;font-size:12px;color:#444;white-space:pre-wrap;background:#f9f9f9;` +
		`border-radius:4px;padding:8px;max-height:200px;overflow-y:auto;font-family:inherit;">` +
		htmlEsc(formattedResult) + `</pre>` +
		`</details>`
}

// renderResultCard returns an HTML card to attach after the AI answer, or "" if
// the tool has no visual card.
// resolveAgent finds the agent a question is being asked as.
//
// It reads the roster first, which is where every agent made since /agents
// existed lives. The ask path used to read only agent/micro's user store — the
// store the roster replaced — so an agent you built, named and gave a system
// prompt was found by nothing when you asked it something: the lookup returned
// nil, the prompt and the tool scope were dropped, and the default assistant
// answered in its place. Nothing said so. That is the whole of "it is not clear
// whether a new agent is a real thing or a play thing": it was not a real
// thing, because the one place it had to be read was reading somewhere else.
//
// The old store is still consulted second, for anything the one-way import has
// not carried over.
func resolveAgent(accountID, id string, isGuest bool) *micro.Agent {
	if isGuest || accountID == "" || id == "" {
		return nil
	}
	if a := AgentFor(accountID, id); a != nil {
		return a.AsMicro()
	}
	// Somebody else's, published. RunPublic charges its price and counts the
	// run, and returns nil for anything not published — so knowing the id of a
	// private agent gets you the default assistant, the same as knowing
	// nothing. What comes back is the recipe; it still runs here, on this
	// account, against this account's scope and credits.
	if a := RunPublic(accountID, id); a != nil {
		return a.AsMicro()
	}
	return micro.GetUserAgentFor(accountID, id)
}

func renderResultCard(toolName, result string, args map[string]any) string {
	switch toolName {
	case "news", "news_search":
		return renderNewsCard(result)
	case "video_search":
		return renderVideoCard(result)
	case "places_search", "places_nearby":
		return renderPlacesCard(result, args)
	case "apps_search":
		return renderAppsCard(result)
	case "apps_run":
		return renderRunCard(result)
	}
	// Service-sourced dashboard cards (markets, news_headlines, social, …),
	// pulled from the same tool registry, attached via api.SetCard in main.go.
	return api.CardForTool(toolName)
}

// --- typed card renderers ---

func renderNewsCard(result string) string {
	var data struct {
		Query     string     `json:"query"`
		Feed      []newsItem `json:"feed"`
		Results   []newsItem `json:"results"`
		Freshness struct {
			Status string `json:"status"`
			Notice string `json:"notice"`
		} `json:"freshness"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	items := data.Results
	if len(items) == 0 {
		items = data.Feed
	}
	if len(items) == 0 {
		return ""
	}
	if newsCardQueryRequiresAI(data.Query) {
		items = filterNewsCardAIItems(items)
	}
	if len(items) == 0 {
		return ""
	}
	if len(items) > 5 {
		items = items[:5]
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📰 News</h4>`)
	if notice := strings.TrimSpace(data.Freshness.Notice); notice != "" && (data.Freshness.Status == "stale" || data.Freshness.Status == "mostly_stale" || data.Freshness.Status == "no_dated_results") {
		b.WriteString(`<div style="margin:0 0 8px;padding:8px;border-radius:4px;background:#fff7ed;color:#7c2d12;font-size:12px;">`)
		b.WriteString(htmlEsc(newsFreshnessCardNotice(data.Freshness.Status, notice)))
		b.WriteString(`</div>`)
	}
	for _, item := range items {
		link := item.URL
		if item.ID != "" {
			link = "/news?id=" + item.ID
		}
		b.WriteString(`<div style="padding:8px 0;border-bottom:1px solid #f0f0f0;">`)
		if item.Category != "" {
			b.WriteString(`<a href="/news#` + htmlEsc(item.Category) + `" class="category" style="font-size:11px;margin-right:6px;">` + htmlEsc(item.Category) + `</a>`)
		}
		b.WriteString(`<a href="` + htmlEsc(link) + `" style="font-size:14px;font-weight:600;display:block;color:#111;">` + htmlEsc(item.Title) + `</a>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<a href="/news" class="link" style="display:inline-block;margin-top:8px;">More news →</a></div>`)
	return b.String()
}

func newsFreshnessCardNotice(status, notice string) string {
	notice = strings.TrimSpace(notice)
	if status == "mostly_stale" {
		return userFacingNewsFreshnessSummary(notice)
	}
	return userFacingNewsFreshnessSummary(notice)
}

type newsItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	Category    string `json:"category"`
}

func filterNewsCardAIItems(items []newsItem) []newsItem {
	filtered := items[:0]
	for _, item := range items {
		if newsCardAIItemIsBroadChipFinance(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func newsCardQueryRequiresAI(query string) bool {
	clean := strings.NewReplacer("-", " ", "_", " ", "/", " ", "'", "", "’", "").Replace(strings.ToLower(query))
	return newsCardTermMatches(clean, "ai") || strings.Contains(clean, "artificial intelligence") || strings.Contains(clean, "machine learning") || newsCardTermMatches(clean, "llm")
}

func newsCardAIItemIsBroadChipFinance(item newsItem) bool {
	haystack := strings.ToLower(strings.Join([]string{item.Title, item.Description, item.Content, item.Category}, " "))
	if haystack == "" {
		return false
	}
	hasChipFrame := false
	for _, term := range []string{"chip", "chips", "semiconductor", "sk hynix", "nvidia", "data center", "datacenter"} {
		if newsCardTermMatches(haystack, term) || strings.Contains(haystack, term) {
			hasChipFrame = true
			break
		}
	}
	if !hasChipFrame {
		return false
	}
	hasFinanceFrame := false
	for _, term := range []string{"market", "markets", "stock", "stocks", "shares", "trading", "investor", "investors", "nasdaq", "ipo", "valuation", "rally", "climb", "debut"} {
		if newsCardTermMatches(haystack, term) {
			hasFinanceFrame = true
			break
		}
	}
	if !hasFinanceFrame {
		return false
	}
	for _, term := range []string{
		"launch", "launches", "launched", "release", "releases", "released", "unveil", "unveils", "unveiled",
		"deploy", "deploys", "deployed", "deployment", "build", "builds", "built", "capacity", "processor",
		"accelerator", "gpu", "server", "inference", "training", "model serving", "model-serving", "cloud",
		"ai product", "ai model", "ai agent", "ai assistant", "ai safety", "ai governance", "ai infrastructure",
	} {
		if newsCardTermMatches(haystack, term) || strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func newsCardTermMatches(haystack, term string) bool {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return false
	}
	if strings.Contains(term, " ") {
		return strings.Contains(haystack, term)
	}
	for _, token := range strings.FieldsFunc(haystack, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if token == term {
			return true
		}
	}
	return false
}

func renderVideoCard(result string) string {
	var data struct {
		Results []videoItem `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	if len(data.Results) == 0 {
		return ""
	}
	items := data.Results
	if len(items) > 4 {
		items = items[:4]
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>🎬 Videos</h4>`)
	for _, v := range items {
		b.WriteString(`<div style="display:flex;gap:10px;padding:8px 0;border-bottom:1px solid #f0f0f0;align-items:flex-start;">`)
		if v.Thumbnail != "" {
			b.WriteString(`<img src="` + htmlEsc(v.Thumbnail) + `" style="width:80px;height:45px;object-fit:cover;border-radius:3px;flex-shrink:0;" loading="lazy">`)
		}
		b.WriteString(`<div style="min-width:0;"><a href="` + htmlEsc(v.URL) + `" style="font-size:13px;font-weight:600;display:block;color:#111;">` + htmlEsc(v.Title) + `</a>`)
		if v.Channel != "" {
			b.WriteString(`<div style="font-size:11px;color:#888;margin-top:2px;">` + htmlEsc(v.Channel) + `</div>`)
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`<a href="/video" class="link" style="display:inline-block;margin-top:8px;">More videos →</a></div>`)
	return b.String()
}

type videoItem struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	Channel   string `json:"channel"`
}

func renderPlacesCard(result string, args map[string]any) string {
	var data struct {
		Results []placeItem `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	if len(data.Results) == 0 {
		return ""
	}
	items := data.Results
	if len(items) > 5 {
		items = items[:5]
	}

	// Build a deterministic Google Maps search URL from the tool args so the
	// link opens the exact same query without any additional server-side cost.
	mapURL := placesMapURL(args, data.Results)

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📍 Places</h4>`)
	for _, p := range items {
		b.WriteString(`<div style="padding:6px 0;border-bottom:1px solid #f0f0f0;">`)
		b.WriteString(`<div style="font-weight:600;">` + htmlEsc(p.Name) + `</div>`)
		if p.Category != "" || p.Address != "" {
			meta := p.Category
			if p.Address != "" {
				if meta != "" {
					meta += " · "
				}
				meta += p.Address
			}
			b.WriteString(`<div style="font-size:12px;color:#888;">` + htmlEsc(meta) + `</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<a href="` + htmlEsc(mapURL) + `" target="_blank" rel="noopener noreferrer" class="link" style="display:inline-block;margin-top:8px;">Open in Google Maps ↗</a></div>`)
	return b.String()
}

// placesMapURL builds a deterministic Google Maps search URL for the places
// results.  It prefers using the query/near tool args when available, falling
// back to a coordinate-based search centred on the first place result.
func placesMapURL(args map[string]any, items []placeItem) string {
	q := ""
	near := ""
	if args != nil {
		if v, ok := args["q"]; ok {
			q = fmt.Sprintf("%v", v)
		}
		if v, ok := args["near"]; ok {
			near = fmt.Sprintf("%v", v)
		}
		if near == "" {
			if v, ok := args["address"]; ok {
				near = fmt.Sprintf("%v", v)
			}
		}
	}

	if q != "" && near != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q+" "+near)
	}
	if q != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q)
	}

	// Fall back: centre on the first result with known coordinates.
	for _, p := range items {
		if p.Lat != 0 || p.Lon != 0 {
			return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", p.Lat, p.Lon)
		}
	}

	return "/places"
}

// formatToolResult converts a raw tool result into a human-readable text
// summary suitable for inclusion in the AI synthesis RAG context.
func formatToolResult(toolName, result string, args map[string]any) string {
	switch toolName {
	case "news", "news_search":
		return withCurrentDateContext(formatNewsResult(result))
	case "news_headlines", "news_list", "news_read":
		return withCurrentDateContext(result)
	case "video_search":
		return formatVideoResult(result)
	case "prayer_reflection", "islam_today", "islam":
		return formatReminderResult(result)
	case "search":
		return withCurrentDateContext(formatSearchResult(result))
	case "web_search", "search_web", "weather_forecast":
		return withCurrentDateContext(result)
	case "markets", "markets_list":
		return withCurrentDateContext(formatMarketsResult(result))
	case "web_fetch", "search_fetch":
		return formatWebFetchResult(result)
	case "places_search", "places_nearby":
		return formatPlacesResult(result, args)
	case "wallet_balance":
		return formatWalletBalanceResult(result)
	case "apps_search":
		return formatAppsSearchResult(result)
	case "apps_read":
		return formatAppsReadResult(result)
	case "apps_build":
		return formatAppsBuildResult(result)
	case "apps_edit":
		return formatAppsBuildResult(result) // same format: returns app details
	case "apps_run":
		return formatAppsRunResult(result)
	}
	return result
}

// formatMarketsResult converts raw markets tool JSON into readable price context
// for synthesis. The go-micro markets tool usually returns {"text":"..."},
// while the REST endpoint returns {"category":"...","data":[...]}; handle both so
// market-moving prompts never expose raw JSON to users or the model fallback.
func formatMarketsResult(result string) string {
	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &textData); err == nil && strings.TrimSpace(textData.Text) != "" {
		return strings.TrimSpace(textData.Text)
	}

	var data struct {
		Category  string `json:"category"`
		UpdatedAt string `json:"updated_at"`
		Stale     bool   `json:"stale"`
		Partial   bool   `json:"partial"`
		Freshness string `json:"freshness"`
		Data      []struct {
			Symbol    string  `json:"symbol"`
			Price     float64 `json:"price"`
			Change24h float64 `json:"change_24h"`
			Type      string  `json:"type"`
			UpdatedAt string  `json:"updated_at"`
			Source    string  `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Data) == 0 {
		return "No market prices available right now."
	}

	category := strings.TrimSpace(data.Category)
	if category == "" {
		category = "market"
	}

	var b strings.Builder
	if strings.TrimSpace(data.Freshness) != "" {
		fmt.Fprintf(&b, "%s.\n", strings.TrimSuffix(strings.TrimSpace(data.Freshness), "."))
	} else if strings.TrimSpace(data.UpdatedAt) != "" {
		fmt.Fprintf(&b, "Last refresh: %s.\n", strings.TrimSpace(data.UpdatedAt))
	}
	if data.Stale {
		b.WriteString("Disclosure: market data may be stale.\n")
	}
	if data.Partial {
		b.WriteString("Disclosure: some requested symbols are unavailable from the current source.\n")
	}
	fmt.Fprintf(&b, "Live %s prices:\n", category)
	count := 0
	for _, item := range data.Data {
		symbol := strings.TrimSpace(item.Symbol)
		if symbol == "" || item.Price == 0 {
			continue
		}
		if item.Change24h != 0 {
			fmt.Fprintf(&b, "%s: $%s (%+.2f%% 24h)\n", symbol, formatMarketPrice(item.Price), item.Change24h)
		} else {
			fmt.Fprintf(&b, "%s: $%s\n", symbol, formatMarketPrice(item.Price))
		}
		count++
	}
	if count == 0 {
		return fmt.Sprintf("No %s prices available right now.", category)
	}
	return strings.TrimSpace(b.String())
}

func formatMarketPrice(price float64) string {
	switch {
	case price >= 100:
		return fmt.Sprintf("%.2f", price)
	case price >= 1:
		return fmt.Sprintf("%.3f", price)
	default:
		return fmt.Sprintf("%.6f", price)
	}
}

// formatNewsResult converts a raw JSON news feed or search result into
// human-readable text for the AI synthesis RAG context.
func formatNewsResult(result string) string {
	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &textData); err == nil && strings.TrimSpace(textData.Text) != "" {
		result = strings.TrimSpace(textData.Text)
	}

	var data struct {
		Feed []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
			Published   string `json:"published"`
			PostedAt    string `json:"posted_at"`
		} `json:"feed"`
		Results []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
			PostedAt    string `json:"posted_at"`
		} `json:"results"`
		Freshness struct {
			Status           string `json:"status"`
			Notice           string `json:"notice"`
			RequestedDate    string `json:"requested_date"`
			FreshestPostedAt string `json:"freshest_posted_at"`
		} `json:"freshness"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	var items []formattedNewsItem
	for _, a := range data.Results {
		items = append(items, formattedNewsItem{a.Title, a.Description, a.Category, a.URL, a.PostedAt, ""})
	}
	if len(items) == 0 {
		for _, a := range data.Feed {
			items = append(items, formattedNewsItem{a.Title, a.Description, a.Category, a.URL, a.PostedAt, a.Published})
		}
	}
	if len(items) == 0 {
		return "No news available."
	}

	// Freshness-sensitive searches feed both the fast guest fallback and the
	// native streaming/final synthesis paths. Keep dated current items ahead of
	// older replayed context before any model sees the result list.
	if data.Query != "" && (data.Freshness.Status == "mostly_stale" || data.Freshness.Status == "stale" || strings.TrimSpace(data.Freshness.Notice) != "") {
		sort.SliceStable(items, func(i, j int) bool {
			left := newsResultItemTime(items[i])
			right := newsResultItemTime(items[j])
			if left.IsZero() || right.IsZero() {
				return false
			}
			return left.After(right)
		})
	}

	// Interleave items across categories round-robin to ensure diversity.
	// The raw feed groups items by category, so naively slicing gives only
	// the first category. Instead, pick up to 2 items per category in
	// round-robin order.
	if data.Query == "" {
		catOrder := []string{}
		catItems := map[string][]formattedNewsItem{}
		for _, a := range items {
			cat := a.Category
			if cat == "" {
				cat = "_"
			}
			if _, ok := catItems[cat]; !ok {
				catOrder = append(catOrder, cat)
			}
			catItems[cat] = append(catItems[cat], a)
		}
		var mixed []formattedNewsItem
		maxPerCat := 3
		for round := 0; round < maxPerCat; round++ {
			for _, cat := range catOrder {
				if round < len(catItems[cat]) {
					mixed = append(mixed, catItems[cat][round])
				}
			}
		}
		items = mixed
	}

	if len(items) > 20 {
		items = items[:20]
	}
	var sb strings.Builder
	if data.Query != "" {
		sb.WriteString(fmt.Sprintf("News results for %q:\n", data.Query))
	} else {
		sb.WriteString("Latest news:\n")
	}
	if freshnessNotice := strings.TrimSpace(data.Freshness.Notice); freshnessNotice != "" {
		sb.WriteString("Freshness caveat: " + freshnessNotice + "\n")
	} else if data.Freshness.Status == "stale" || data.Freshness.Status == "no_dated_results" {
		requestedDate := strings.TrimSpace(data.Freshness.RequestedDate)
		if requestedDate == "" {
			requestedDate = "the requested date"
		}
		if data.Freshness.Status == "no_dated_results" {
			sb.WriteString("Freshness caveat: No dated news results were available for " + requestedDate + "; do not present these results as today's news without that caveat.\n")
		} else {
			freshest := strings.TrimSpace(data.Freshness.FreshestPostedAt)
			if freshest != "" {
				if t, err := time.Parse(time.RFC3339, freshest); err == nil {
					freshest = t.UTC().Format("2006-01-02")
				}
			}
			if freshest == "" {
				sb.WriteString("Freshness caveat: No same-day news results were available for " + requestedDate + "; lead with a freshness caveat before older items.\n")
			} else {
				sb.WriteString("Freshness caveat: No same-day news results were available for " + requestedDate + "; the freshest result is from " + freshest + ", so lead with a freshness caveat before older items.\n")
			}
		}
	}
	for i, a := range items {
		line := fmt.Sprintf("%d. %s", i+1, a.Title)
		if desc := cleanNewsDescription(a.Description); desc != "" {
			line += " — " + desc
		}
		if label := conciseNewsSourceDateLabel(a); label != "" {
			line += " (" + label + ")"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func cleanNewsDescription(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	desc = strings.TrimRight(desc, " \t\n\r")
	for strings.HasSuffix(desc, "...") || strings.HasSuffix(desc, "…") {
		desc = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(desc, "..."), "…"))
	}
	return desc
}

func conciseNewsSourceDateLabel(item formattedNewsItem) string {
	var parts []string
	if source := newsSourceLabel(item.URL); source != "" {
		parts = append(parts, source)
	}
	if date := newsDateLabel(item); date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, ", ")
}

func newsSourceLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	return host
}

func newsDateLabel(item formattedNewsItem) string {
	if t := newsResultItemTime(item); !t.IsZero() {
		return t.Format("2 Jan 2006")
	}
	return ""
}

type formattedNewsItem struct {
	Title       string
	Description string
	Category    string
	URL         string
	PostedAt    string
	Published   string
}

func newsResultItemTime(item formattedNewsItem) time.Time {
	for _, value := range []string{item.PostedAt, item.Published} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2 Jan 2006 15:04 MST", "2 Jan 2006", "2006-01-02", "Jan 2, 2006"} {
			if t, err := time.Parse(layout, value); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

// formatVideoResult converts a raw JSON video search result into
// human-readable text for the AI synthesis RAG context.
func formatVideoResult(result string) string {
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			Channel string `json:"channel"`
			URL     string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No videos found."
	}
	items := data.Results
	if len(items) > 10 {
		items = items[:10]
	}
	var sb strings.Builder
	sb.WriteString("Video results:\n")
	for i, v := range items {
		line := fmt.Sprintf("%d. %s", i+1, v.Title)
		if v.Channel != "" {
			line += fmt.Sprintf(" (channel: %s)", v.Channel)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// formatReminderResult converts a raw JSON reflection payload into
// human-readable text for the AI synthesis RAG context.
func formatReminderResult(result string) string {
	var data struct {
		Verse   string `json:"verse"`
		Name    string `json:"name"`
		Hadith  string `json:"hadith"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if data.Verse == "" && data.Hadith == "" && data.Message == "" {
		return "No reflection is available right now."
	}
	// Verse, Saying, Name, Reflection — the same four labels the /prayer page
	// and the prayer_reflection tool use.
	var sb strings.Builder
	sb.WriteString("Today's Islamic reflection:\n")
	if data.Verse != "" {
		sb.WriteString(fmt.Sprintf("Verse: %s\n", data.Verse))
	}
	if data.Hadith != "" {
		sb.WriteString(fmt.Sprintf("Saying: %s\n", data.Hadith))
	}
	if data.Name != "" {
		sb.WriteString(fmt.Sprintf("Name: %s\n", data.Name))
	}
	if data.Message != "" {
		sb.WriteString(fmt.Sprintf("Reflection: %s\n", data.Message))
	}
	return sb.String()
}

// formatSearchResult converts a raw search result (which may be an HTML page)
// into human-readable text for the AI synthesis RAG context.
func formatSearchResult(result string) string {
	// Try to parse as JSON first (structured response)
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Type    string `json:"type"`
		} `json:"results"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err == nil && len(data.Results) > 0 {
		var sb strings.Builder
		if data.Query != "" {
			sb.WriteString(fmt.Sprintf("Search results for %q:\n", data.Query))
		} else {
			sb.WriteString("Search results:\n")
		}
		for i, r := range data.Results {
			line := fmt.Sprintf("%d. %s", i+1, r.Title)
			if r.Type != "" {
				line += fmt.Sprintf(" [%s]", r.Type)
			}
			if r.Content != "" {
				snippet := r.Content
				if len(snippet) > 120 {
					snippet = snippet[:120] + "…"
				}
				line += " — " + snippet
			}
			sb.WriteString(line + "\n")
		}
		return sb.String()
	}
	// Fall back: strip HTML tags to extract plain text
	return stripHTMLTags(result)
}

// formatWebFetchResult converts a raw JSON web fetch result into
// human-readable text for the AI synthesis RAG context.
func formatWebFetchResult(result string) string {
	var data struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	var sb strings.Builder
	if data.Title != "" {
		sb.WriteString(fmt.Sprintf("Page: %s\n", data.Title))
	}
	if data.URL != "" {
		sb.WriteString(fmt.Sprintf("URL: %s\n", data.URL))
	}
	sb.WriteString("\n")
	content := data.Content
	// Truncate for AI context — keep it reasonable
	if len(content) > 8000 {
		content = content[:8000] + "\n\n[Content truncated for brevity]"
	}
	sb.WriteString(content)
	return sb.String()
}

// stripHTMLTags removes HTML tags from s and collapses whitespace.
func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '<':
			inTag = true
		case s[i] == '>':
			inTag = false
			sb.WriteByte(' ')
		case !inTag:
			sb.WriteByte(s[i])
		}
	}
	// Collapse runs of whitespace
	out := strings.Join(strings.Fields(sb.String()), " ")
	if len(out) > 2000 {
		out = out[:2000] + "…"
	}
	return out
}

// formatPlacesResult converts a raw JSON places result into a human-readable
// text summary suitable for inclusion in the AI synthesis RAG context.
func formatPlacesResult(result string, args map[string]any) string {
	var data struct {
		Results []placeItem `json:"results"`
		Count   int         `json:"count"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No places found."
	}

	q := ""
	near := ""
	if args != nil {
		if v, ok := args["q"]; ok {
			q = fmt.Sprintf("%v", v)
		}
		if v, ok := args["near"]; ok {
			near = fmt.Sprintf("%v", v)
		}
		if near == "" {
			if v, ok := args["address"]; ok {
				near = fmt.Sprintf("%v", v)
			}
		}
	}

	var sb strings.Builder
	header := fmt.Sprintf("Found %d place(s)", len(data.Results))
	if q != "" && near != "" {
		header += fmt.Sprintf(" matching %q near %s", q, near)
	} else if q != "" {
		header += fmt.Sprintf(" matching %q", q)
	} else if near != "" {
		header += fmt.Sprintf(" near %s", near)
	}
	sb.WriteString(header + ":\n")
	for i, p := range data.Results {
		line := fmt.Sprintf("%d. %s", i+1, p.Name)
		if p.Category != "" {
			line += " (" + p.Category + ")"
		}
		if p.Address != "" {
			line += " — " + p.Address
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

type placeItem struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Address  string  `json:"address"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

// formatWalletBalanceResult converts a raw JSON wallet balance result into
// human-readable text for the AI synthesis RAG context.
//
// One tool answers the whole question now, so this renders credits and the
// deposit address together. "balance" is the key the old path-backed tool
// returned; it is still read so an answer built before this change formats.
func formatWalletBalanceResult(result string) string {
	var data struct {
		Credits *int   `json:"credits"`
		Balance *int   `json:"balance"`
		Address string `json:"address"`
		USDC    string `json:"usdc"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	credits := 0
	switch {
	case data.Credits != nil:
		credits = *data.Credits
	case data.Balance != nil:
		credits = *data.Balance
	default:
		return result
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Wallet balance: %d credits (£%d.%02d). Top up at /wallet/topup.\n", credits, credits/100, credits%100)
	if data.Address != "" {
		fmt.Fprintf(&sb, "Base address for USDC top-ups: %s (holding $%s USDC).\n", data.Address, data.USDC)
	}
	return sb.String()
}

// formatAppsSearchResult converts a raw JSON apps search result into
// human-readable text for the AI synthesis RAG context.
func formatAppsSearchResult(result string) string {
	var apps []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &apps); err != nil {
		return result
	}
	if len(apps) == 0 {
		return "No apps found."
	}
	var sb strings.Builder
	sb.WriteString("Apps:\n")
	for i, a := range apps {
		tagInfo := ""
		if a.Tags != "" {
			tagInfo = " [" + a.Tags + "]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s) — %s%s %d installs /apps/%s\n",
			i+1, a.Name, a.Slug, a.Description, tagInfo, a.Installs, a.Slug))
	}
	return sb.String()
}

// formatAppsReadResult converts a raw JSON app detail result into
// human-readable text for the AI synthesis RAG context.
func formatAppsReadResult(result string) string {
	var a struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &a); err != nil {
		return result
	}
	if a.Name == "" {
		return result
	}
	tagLine := ""
	if a.Tags != "" {
		tagLine = fmt.Sprintf("Tags: %s\n", a.Tags)
	}
	return fmt.Sprintf("App: %s\nID: %s\nDescription: %s\nAuthor: %s\n%sInstalls: %d\nURL: /apps/%s\nRun: /apps/%s/run\n",
		a.Name, a.Slug, a.Description, a.Author, tagLine, a.Installs, a.Slug, a.Slug)
}

// formatAppsBuildResult converts a raw JSON build result into
// human-readable text for the AI synthesis RAG context.
func formatAppsBuildResult(result string) string {
	var data struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return "App HTML generated successfully."
	}
	if data.HTML == "" {
		return result
	}
	snippet := data.HTML
	if len(snippet) > 200 {
		snippet = snippet[:200] + "…"
	}
	return fmt.Sprintf("Generated app HTML (%d bytes). Preview:\n%s", len(data.HTML), snippet)
}

// formatAppsRunResult converts a raw JSON run result into
// human-readable text for the AI synthesis RAG context.
func formatAppsRunResult(result string) string {
	var data struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	return fmt.Sprintf("Code sandbox created. URL: %s\nThe code will execute in the user's browser and display results.", data.URL)
}

// renderRunCard renders a live code execution iframe for apps_run results.
func renderRunCard(result string) string {
	var data struct {
		ID  string `json:"id"`
		Run string `json:"run"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil || data.Run == "" {
		return ""
	}
	return `<div class="card"><h4>⚡ Result</h4>` +
		`<iframe src="` + htmlEsc(data.Run) + `" sandbox="allow-scripts" allow="geolocation" ` +
		`style="width:100%;min-height:120px;border:1px solid #eee;border-radius:6px;background:#fff;"></iframe>` +
		`</div>`
}

// renderAppsCard renders an HTML card for apps search results.
func renderAppsCard(result string) string {
	var apps []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &apps); err != nil {
		return ""
	}
	if len(apps) == 0 {
		return ""
	}
	if len(apps) > 5 {
		apps = apps[:5]
	}
	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📱 Apps</h4>`)
	for _, a := range apps {
		b.WriteString(`<div style="padding:6px 0;border-bottom:1px solid #f0f0f0;">`)
		b.WriteString(`<a href="/apps/` + htmlEsc(a.Slug) + `" style="font-weight:600;">` + htmlEsc(a.Name) + `</a>`)
		b.WriteString(`<div style="font-size:12px;color:#888;">` + htmlEsc(a.Description) + `</div>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<a href="/apps" class="link" style="display:inline-block;margin-top:8px;">Browse all apps →</a></div>`)
	return b.String()
}

// htmlEsc escapes a string for safe HTML attribute/text inclusion.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func formatPrice(price float64) string {
	if price >= 1000 {
		return fmt.Sprintf("$%s", formatLargeNum(price))
	}
	if price >= 1 {
		return fmt.Sprintf("$%.2f", price)
	}
	return fmt.Sprintf("$%.4f", price)
}

func formatLargeNum(n float64) string {
	// Simple comma-formatted integer
	i := int64(n)
	s := fmt.Sprintf("%d", i)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if s != "" {
		parts = append([]string{s}, parts...)
	}
	return strings.Join(parts, ",")
}
