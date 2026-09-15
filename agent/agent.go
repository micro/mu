package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mu/internal/ai"
	"mu/web"
	"net/http"
	"net/url"

	"strings"
	"sync"
	"time"

	"mu/agent/micro"
	"mu/inbox"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/bookmarks"
	"mu/internal/result"

	"mu/internal/thread"
)

var QuotaCheck func(r *http.Request, op string) (bool, int, error)

var ChargeQuota func(r *http.Request, op string)

func Load() {
	adoptAll()

	inbox.Tools = runTools
}

func runTools(workflow string) string {
	f := getFlow(workflow)
	if f == nil {
		return ""
	}
	var chips strings.Builder
	seen := map[string]bool{}
	for _, s := range f.Steps {
		if s.Tool == "" || seen[s.Tool] {
			continue
		}
		seen[s.Tool] = true
		chips.WriteString(app.Pill(s.Tool))
	}
	if f.Status == "error" && f.Error != "" {
		chips.WriteString(`<span class="ib-failed">` + htmlEsc(f.Error) + `</span>`)
	}
	if chips.Len() == 0 {
		return ""
	}
	return `<div class="ib-ran">` + chips.String() + `</div>`
}

type QueryMessage struct {
	Role string // "user" or "assistant"
	Text string
}

type QueryOpts struct {
	RunContext        context.Context
	OnStepStart       func(Step)
	Context           ClientContext
	RawReply          bool
	OutputInstruction string
	Thread            string // server-resolved conversation; ownership checked before retrieval
	History           []QueryMessage
	Public            bool   // if true, skip private context (mail, wallet, etc.)
	System            string // optional custom system prompt (user-defined agent)
	Extra             string
	Tools             []string // optional tool allow-list (user-defined agent); empty = all
	Model             string
	Stream            StreamHooks
	OnStep            func(Step)
}

func microStepper(opts QueryOpts) func(string, map[string]any, bool, time.Duration) {
	if opts.OnStep == nil {
		return nil
	}
	return func(tool string, args map[string]any, ok bool, took time.Duration) {
		opts.OnStep(Step{Tool: tool, Args: args, OK: ok, Took: took})
	}
}

type Step struct {
	ID       string
	Started  time.Time
	Finished time.Time
	Output   string
	Error    string
	Tool     string         `json:"tool"`
	Args     map[string]any `json:"args,omitempty"`
	OK       bool           `json:"ok"`
	Took     time.Duration  `json:"took"`
}

func Query(accountID, prompt string, history ...QueryMessage) (string, error) {
	return QueryWithOpts(accountID, prompt, QueryOpts{History: history})
}

func Routed(prompt string, opts QueryOpts) (string, QueryOpts) {
	if strings.TrimSpace(opts.System) != "" {
		return prompt, opts
	}

	id := micro.MatchDirectAddress(prompt)
	if id == "" {
		return prompt, opts
	}
	prompt = micro.StripAddress(prompt)
	if id == DefaultPlatformAgent {
		return prompt, opts
	}
	if o := PlatformOpts(Platform(id)); o.System != "" {
		opts.System, opts.Tools = o.System, o.Tools
	}
	return prompt, opts
}

func QueryWithOpts(accountID, prompt string, opts QueryOpts) (string, error) {
	prompt, opts = Routed(prompt, opts)

	answer, err := queryWithFallback(accountID, prompt, opts)
	if err != nil {
		return "", err
	}
	if opts.RawReply {
		return answer, nil
	}
	return app.NormalizeAnswerMarkdown(answer), nil
}

func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/agent/flow/") {
		id := strings.TrimPrefix(path, "/agent/flow/")
		switch r.Method {
		case "GET":
			if id != "" && r.URL.Query().Get("json") != "1" {
				http.Redirect(w, r, "/agent/"+DefaultSlug+"?session="+url.QueryEscape(id),
					http.StatusFound)
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
		if strings.TrimPrefix(path, "/agent/") != "" && path != "/agent" {
			APIHandler(w, r)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			handleQuery(w, r)
			return
		}
		APIHandler(w, r)
	default:
		app.MethodNotAllowed(w, r)
	}
}

func servePage(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	accountID := acc.ID
	assistant := r.URL.Path == "/assistant" || r.URL.Path == "/"
	w.Header().Set("Cache-Control", "private, no-store")

	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = r.URL.Query().Get("continue")
	}
	cfg := app.ChatConfig{StorageNS: "agent"}
	cfg.AgentName = agentTitle(accountID, "")
	selected := ""
	if sessionID == "" {
		var item *bookmarks.Item
		var err error
		id := r.URL.Query().Get("bookmark")
		if id == "" {
			id = r.URL.Query().Get("saved")
		}
		if id != "" {
			item, err = bookmarks.Get(accountID, id)
			cfg.Attachment = "bookmark:" + id
		} else if ref := r.URL.Query().Get("item"); ref != "" {
			item, err = bookmarks.Source(ref)
			cfg.Attachment = "archive:" + ref
		}
		if err != nil {
			app.NotFound(w, r, "Reading material not found")
			return
		}
		if item != nil {
			cfg.StorageNS = "reading-" + accountID + "-" + item.ID + "-" + item.Ref
			cfg.Placeholder = "What would you like to know about this?"
			selected = `<div class="card"><strong>` + html.EscapeString(item.Title) + `</strong><p>This material will accompany your question in this private conversation.</p></div>`
		}
	}
	reopened := false
	reopenAgent := "" // agent the reopened conversation is with
	elsewhere := ""
	if sessionID != "" {
		if id := openThread(accountID, sessionID); id != "" {
			th := thread.Get(accountID, id)
			reopened = true
			if th != nil {
				reopenAgent = th.Agent
			}
			if th != nil && th.Client != thread.WebClient {
				elsewhere = inbox.ConversationView(accountID, th)
			} else {
				cfg.ContextID = id
				cfg.InitialConvHTML = renderThreadTurns(accountID, id)
				cfg.Pending = Pending(accountID, id)
			}
		}
	}

	if sessionID != "" && !reopened {
		app.NotFound(w, r, "Conversation not found")
		return
	}

	if r.Header.Get("X-Mu-Transcript") == "1" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "X-Mu-Transcript")
		if !reopened || elsewhere != "" {
			app.RespondError(w, 404, "Chat not found")
			return
		}
		app.RespondJSON(w, map[string]any{"id": cfg.ContextID, "html": cfg.InitialConvHTML, "pending": cfg.Pending, "agent": reopenAgent, "agentName": agentTitle(accountID, reopenAgent), "storageNS": "agent-" + accountID + "-" + reopenAgent})
		return
	}

	prefill := r.URL.Query().Get("prompt")
	if prefill == "" {
		prefill = r.URL.Query().Get("q")
	}

	selAgent := r.URL.Query().Get("id")
	if selAgent == "" {
		selAgent = r.URL.Query().Get("agent")
	}
	named := r.URL.Path == "/" || selAgent != ""
	if slug := strings.TrimPrefix(r.URL.Path, "/agent/"); strings.HasPrefix(r.URL.Path, "/agent/") &&
		slug != "" && !strings.Contains(slug, "/") {
		id, ok := agentSlugTarget(accountID, slug)
		if !ok {
			app.NotFound(w, r, "no agent called "+slug)
			return
		}
		selAgent, named = id, true
	}
	if reopened {
		selAgent = reopenAgent
	} else if (selAgent != "" || named || assistant) && prefill == "" && cfg.Attachment == "" && r.URL.Query().Get("new") != "1" {
		if last := latestThreadFor(accountID, selAgent, named); last != "" {
			cfg.ContextID = last
			cfg.InitialConvHTML = renderThreadTurns(accountID, last)
			cfg.Pending = Pending(accountID, last)
			if th := thread.Get(accountID, last); th != nil {
				selAgent = th.Agent
			}
		}
	}

	cfg.SelectionScope = accountID + ":" + chatPath(accountID, selAgent)
	if assistant {
		cfg.SelectionScope = accountID + ":/"
	}
	cfg.AgentName = agentTitle(accountID, selAgent)
	chatBase := chatPath(accountID, selAgent)
	if assistant {
		chatBase = "/"
		cfg.Location = true
	}

	if elsewhere != "" && !app.WantsJSON(r) {
		http.Redirect(w, r, "/inbox?id="+url.QueryEscape(sessionID), http.StatusSeeOther)
		return
	}
	conversation := ClientConversation{Messages: []ClientMessage{}}
	if cfg.ContextID != "" {
		conversation = ClientHistory(accountID, cfg.ContextID)
	}
	conversation.Agent, conversation.AgentName = selAgent, agentTitle(accountID, selAgent)
	conversation.Attachment = cfg.Attachment
	if selected != "" {
		conversation.AttachmentTitle = "Reading material"
	}
	state := map[string]any{"account": map[string]any{"id": acc.ID, "name": acc.Name, "admin": acc.Admin}, "csrf": auth.CSRFToken(r), "conversation": conversation}
	if web.Page(w, r, conversation.AgentName, state) {
		return
	}
	if app.WantsJSON(r) {
		app.RespondJSON(w, state)
		return
	}
	chip := `<div class="conversation-toolbar"><strong>` + html.EscapeString(agentTitle(accountID, selAgent)) + `</strong></div>`
	if selAgent == "" {
		chip = `<div class="conversation-toolbar" hidden><strong></strong></div>`
	}
	cfg.Placeholder = "What do you need?"
	cfg.StorageNS = "agent-" + accountID + "-" + selAgent
	cfg.ServerOwned = true
	cfg.Ask = true
	main := selected + app.ChatComponent(cfg)
	if elsewhere != "" {
		main = elsewhere
	}
	content := `<div class="chat-layout"><div class="chat-main">` + chip + main +
		`</div></div>` + chatPageJS

	content += `<script>history.replaceState(window.history.state,'',` + app.JSString(chatBase) + `);window.addEventListener('mu-chat-thread',function(e){history.replaceState(window.history.state,'',` + app.JSString(chatBase) + `);});</script>`
	if r.URL.Path == "/" {
		content += HandoffHTML(r)
	}
	content += `<script>window.muSeedAgent(` + app.JSString(selAgent) + `);</script>`
	if prefill != "" {
		content += `<script>(function(){var i=document.getElementById('mu-chat-input');if(i&&window.muChatAsk){i.value=` + app.JSString(prefill) + `;window.muChatAsk(i.value);}history.replaceState(window.history.state,'',` + app.JSString(chatBase) + `);})()</script>`
	}

	title := agentTitle(accountID, selAgent)
	if assistant && selAgent == "" {
		title = "Home"
	}
	desc := "Talk to " + agentTitle(accountID, selAgent)
	if assistant && selAgent == "" {
		desc = "A personal AI agent"
	}
	app.Respond(w, r, app.Response{Title: title, Description: desc, HTML: content})
}

func openThread(accountID, id string) string {
	if th := thread.Get(accountID, id); th != nil {
		return th.ID
	}
	chain := sessionChain(accountID, id)
	if len(chain) == 0 || chain[len(chain)-1].AccountID != accountID {
		return ""
	}
	return adopt(accountID, chain)
}

func renderThreadTurns(accountID, threadID string) string {
	who := ""
	if th := thread.Get(accountID, threadID); th != nil {
		who = agentTitle(accountID, th.Agent)
	}
	var b strings.Builder
	for _, m := range thread.Messages(accountID, threadID, inbox.MessagesShown) {
		b.WriteString(renderTurn(m, who))
	}
	return b.String()
}

func latestThreadFor(accountID, agentID string, named bool) string {
	for _, t := range chatThreads(accountID, agentID, named) { // newest first
		return t.ID
	}
	return ""
}

const railShown = 40

func chatThreads(accountID, agentID string, named bool) []thread.Thread {
	var out []thread.Thread
	for _, t := range thread.List(accountID, 0) {
		if thread.Arrived(t) {
			continue
		}
		if named && t.Agent != agentID {
			continue
		}
		out = append(out, t)
		if len(out) >= railShown {
			break
		}
	}
	return out
}

const chatPageJS = `<script>
window.muActiveAgent='';
window.muSeedAgent=function(id){window.muActiveAgent=id||'';};
window.muAgentCsrf=function(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';};
</script>`

func serveFlowPage(w http.ResponseWriter, r *http.Request, id string) {
	f := getFlow(id)
	if f == nil {
		http.NotFound(w, r)
		return
	}

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

	app.NotFound(w, r, "Run not found")
}

func handleDeleteFlow(w http.ResponseWriter, r *http.Request, id string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}
	if err := deleteFlow(acc.ID, id); err != nil {
		http.Error(w, `{"error":"failed to delete run"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const sseHeartbeat = 10 * time.Second

func sse(w http.ResponseWriter, event map[string]any) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func streamNativeSSE(w http.ResponseWriter, accountID, prompt string, opts QueryOpts, flow *Flow, threadID string, streamText bool) {
	var wmu sync.Mutex
	send := func(ev map[string]any) {
		wmu.Lock()
		defer wmu.Unlock()
		sse(w, ev)
	}
	beating := make(chan struct{})
	defer close(beating)
	go func() {
		t := time.NewTicker(sseHeartbeat)
		defer t.Stop()
		for {
			select {
			case <-beating:
				return
			case <-t.C:
				wmu.Lock()
				fmt.Fprint(w, ": still here\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				wmu.Unlock()
			}
		}
	}()

	emitted := false
	var nativeTools []string
	startedTools := map[string]bool{}
	endedTools := map[string]bool{}
	var unnamed int
	keyOf := func(run ToolRun) string {
		if run.ID != "" {
			return run.ID
		}
		unnamed++
		return fmt.Sprintf("%s#%d", run.Name, unnamed)
	}

	sopts := opts
	var results []result.Item
	sopts.OnStep = func(step Step) {
		if opts.OnStep != nil {
			opts.OnStep(step)
		}
		wmu.Lock()
		defer wmu.Unlock()
		results = mergeResults(results, resultItems(step))
	}
	sopts.Stream = StreamHooks{
		ToolStart: func(run ToolRun) {
			wmu.Lock()
			key := keyOf(run)
			if startedTools[key] {
				wmu.Unlock()
				return
			}
			startedTools[key] = true
			emitted = true
			nativeTools = append(nativeTools, run.Label)
			wmu.Unlock()
			if accountID != "" {
				updateFlow(flow.ID, func(f *Flow) {
					f.Steps = append(f.Steps, FlowStep{ID: key, Tool: NativeToolName(run.Name),
						Label: run.Label, Status: "running", Started: time.Now().UTC()})
				})
			}
			send(map[string]any{"type": "tool_start", "name": run.Label, "message": run.Label})
		},
		ToolEnd: func(run ToolRun) {
			wmu.Lock()
			key := keyOf(run)
			if endedTools[key] {
				wmu.Unlock()
				return
			}
			endedTools[key] = true
			wmu.Unlock()
			if accountID != "" {
				updateFlow(flow.ID, func(f *Flow) {
					name := NativeToolName(run.Name)
					for i := len(f.Steps) - 1; i >= 0; i-- {
						if f.Steps[i].ID == key || (run.ID == "" &&
							f.Steps[i].Status == "running" && f.Steps[i].Tool == name) {
							f.Steps[i].Status = "done"
							f.Steps[i].Finished = time.Now().UTC()
							break
						}
					}
				})
			}
			send(map[string]any{"type": "tool_done", "name": run.Label, "message": run.Label + " — done"})
		},
	}
	if streamText && !shouldHoldNativeNewsStreamTokens(prompt, nil) {
		sopts.Stream.Token = func(text string) {
			wmu.Lock()
			defer wmu.Unlock()
			emitted = true
			sse(w, map[string]any{"type": "stream_token", "text": text})
		}
	}
	answer, err := queryWithFallback(accountID, prompt, sopts)
	if err != nil {
		if emitted {
			app.Log("agent", "stream error mid-answer: %v", err)
		} else {
			app.Log("agent", "stream failed before output: %v", err)
		}
		updateFlow(flow.ID, func(f *Flow) {
			f.Status = "error"
			f.Error = err.Error()
			for i := range f.Steps {
				if f.Steps[i].Status == "running" {
					f.Steps[i].Status = "error"
					f.Steps[i].Finished = time.Now().UTC()
				}
			}
		})
		send(map[string]any{"type": "error", "message": agentErrorMessage(err)})
		send(map[string]any{"type": "done"})
		return
	}

	answer = completeNativeToolAnswer(answer, nativeTools)
	answer = app.NormalizeAnswerMarkdown(answer)
	rendered := app.RenderString(answer)
	html := `<div class="card" id="agent-response">` + rendered + `</div>` + app.Results(results)
	if accountID != "" {
		Answered(accountID, threadID, answer, flow.ID, results...)
		updateFlow(flow.ID, func(f *Flow) {
			f.Answer = answer
			f.HTML = html
			f.Status = "done"
		})
	}
	send(map[string]any{"type": "response", "html": html, "text": answer, "results": results, "flow_id": flow.ID})
	send(map[string]any{"type": "done"})
}

func agentErrorMessage(err error) string {
	if errors.Is(err, ErrNoProvider) {
		return "The agent has no AI provider configured. Set a provider key in /admin/config."
	}
	return "Could not generate response: " + ai.FailureMessage(err)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Context    ClientContext `json:"context"`
		Prompt     string        `json:"prompt"`
		Attachment string        `json:"attachment"`
		Model      string        `json:"model"`
		Agent      string        `json:"agent"`       // optional: user-defined agent id to answer as
		ContextID  string        `json:"context_id"`  // optional: prior flow to continue from
		StreamText bool          `json:"stream_text"` // opt-in answer deltas, followed by the final response
		History    []struct {
			Prompt string `json:"prompt"`
			Answer string `json:"answer"`
		} `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, `{"error":"prompt required"}`, http.StatusBadRequest)
		return
	}

	_, acc := auth.TrySession(r)
	if acc == nil {
		w.Header().Set("Content-Type", "application/json")
		if !app.GuestAllowed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"This instance is not answering strangers right now."}`)) //nolint:errcheck
			return
		}
	}

	accountID := ""
	guest := acc == nil
	if acc != nil {
		accountID = acc.ID
	}

	threadID := ""
	if !guest && req.ContextID != "" {
		threadID = openThread(accountID, req.ContextID)
	}

	attachment := req.Attachment
	if threadID != "" {
		attachment = thread.Attachment(accountID, threadID)
	}
	reading := ""
	if attachment != "" {
		if guest {
			app.RespondError(w, http.StatusUnauthorized, "Sign in to use reading material")
			return
		}
		var err error
		reading, err = readingContext(accountID, attachment)
		if err != nil {
			if threadID == "" {
				app.RespondError(w, http.StatusNotFound, "Reading material not found")
				return
			}
			reading = "The material previously attached to this conversation is no longer available."
		}
	}

	var conversationHistory []*Flow
	if threadID != "" {
		conversationHistory = pastTurns(accountID, threadID, historyTurns)
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

	flow := &Flow{
		ID:        newFlowID(),
		AccountID: accountID,
		Prompt:    req.Prompt,
		Status:    "running",
		Agent:     req.Agent,
		CreatedAt: time.Now().UTC(),
	}
	if !guest {
		if threadID == "" {
			threadID = Opened(accountID, thread.WebClient, flow.ID, "", req.Agent)
			thread.SetAttachment(accountID, threadID, attachment)
		}
		if req.Agent == "" {
			RememberClientConversation(w, r, threadID)
		}
		flow.ThreadID = threadID
		if err := saveFlow(flow); err != nil {
			app.Log("agent", "Failed to create flow: %v", err)
		}
		Said(accountID, threadID, req.Prompt, "", "")

	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	sse(w, map[string]any{"type": "flow_id", "flow_id": flow.ID, "thread": threadID})

	sse(w, map[string]any{"type": "working", "message": "Working"})

	nopts := QueryOpts{Public: guest}
	nopts.Context = req.Context
	nopts.Extra = reading
	if ua := resolveAgent(accountID, req.Agent); ua != nil && !guest {
		nopts.System = ua.SystemPrompt
		nopts.Tools = ua.Tools
	}

	if h := History(accountID, threadID, historyTurns); len(h) > 0 {
		nopts.History = h
	} else {
		for _, f := range conversationHistory {
			if strings.TrimSpace(f.Prompt) == "" {
				continue
			}
			nopts.History = append(nopts.History,
				QueryMessage{Role: "user", Text: f.Prompt},
				QueryMessage{Role: "assistant", Text: f.Answer})
		}
	}
	nopts.Thread = threadID
	routedPrompt, nopts := Routed(req.Prompt, nopts)
	streamNativeSSE(w, accountID, routedPrompt, nopts, flow, threadID, req.StreamText)
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

func shouldReplayFinalNativeAnswer(prompt string, nativeTools []string, capturedLen int) bool {
	if capturedLen > 0 {
		return true
	}
	return shouldHoldNativeNewsStreamTokens(prompt, nativeTools)
}

func shouldHoldNativeNewsStreamTokens(prompt string, nativeTools []string) bool {
	if !isLatestTechnologyNewsPrompt(strings.ToLower(prompt)) {
		return false
	}
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
	case "apps_embed":
		return "📱 Getting the embed code"
	default:
		return "⚙ Calling " + tool
	}
}

func resolveAgent(accountID, id string) *micro.Agent {
	if accountID == "" || id == "" {
		return nil
	}
	if a := For(accountID, id); a != nil {
		return a.AsMicro()
	}
	if a := micro.UserAgentFor(accountID, id); a != nil {
		return a
	}
	return micro.Get(id)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

func htmlEsc(s string) string { return html.EscapeString(s) }
