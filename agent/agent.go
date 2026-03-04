// Package agent provides a conversational AI agent interface that has access
// to all Mu tools via the MCP server, using the user's session token for calls.
package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"mu/ai"
	"mu/api"
	"mu/app"
	"mu/auth"
)

// historyLimit is the maximum number of history items shown on the agent page.
const historyLimit = 20

// Model represents an available LLM model tier for agent queries.
type Model struct {
	ID       string
	Name     string
	Desc     string
	WalletOp string
	Provider string // ai provider constant, empty = default
	Model    string // ai model override, empty = provider default
}

// defaultPremiumModel is the Anthropic model used for premium agent queries.
var defaultPremiumModel = func() string {
	if v := os.Getenv("ANTHROPIC_PREMIUM_MODEL"); v != "" {
		return v
	}
	return "claude-sonnet-4-5-20250514"
}()

// Models lists the available model tiers.
var Models = []Model{
	{
		ID:       "standard",
		Name:     "Standard",
		Desc:     "Fast and efficient",
		WalletOp: "agent_query",
		Provider: ai.ProviderDefault,
	},
	{
		ID:       "premium",
		Name:     "Premium",
		Desc:     "Best quality",
		WalletOp: "agent_query_premium",
		Provider: ai.ProviderAnthropic,
		Model:    defaultPremiumModel,
	},
}

// QuotaCheck is set by main.go to wire in the wallet quota check without an
// import cycle. Signature matches api.QuotaCheck.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// Load initialises the agent package (no-op for now; reserved for future use).
func Load() {}

// Handler dispatches GET (page) and POST (query) at /agent and /agent/*.
func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// /agent/flow/<id>  — view or delete a saved flow
	if strings.HasPrefix(path, "/agent/flow/") {
		id := strings.TrimPrefix(path, "/agent/flow/")
		switch r.Method {
		case "GET":
			serveFlowPage(w, r, id)
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

// servePage renders the agent chat page, including query history for logged-in users.
func servePage(w http.ResponseWriter, r *http.Request) {
	var modelOpts strings.Builder
	for _, m := range Models {
		modelOpts.WriteString(fmt.Sprintf(
			`<option value="%s">%s — %s</option>`, m.ID, m.Name, m.Desc,
		))
	}

	// Pre-fill prompt and context when continuing a saved flow.
	contextID := r.URL.Query().Get("continue")
	preFillPrompt := ""
	if contextID != "" {
		if f := getFlow(contextID); f != nil {
			preFillPrompt = htmlEsc(f.Prompt)
		}
	}

	content := `<div class="card">
<form id="agent-form">
<textarea id="agent-prompt" name="prompt" rows="3"
  placeholder="What would you like to know?"
  style="width:100%;box-sizing:border-box;padding:8px;font-family:inherit;font-size:15px;resize:vertical;border:1px solid #ddd;border-radius:4px;">` + preFillPrompt + `</textarea>
<div style="display:flex;gap:8px;margin-top:8px;align-items:center;flex-wrap:wrap;">
<select id="agent-model"
  style="padding:6px 10px;font-family:inherit;font-size:13px;border:1px solid #ddd;border-radius:4px;">` +
		modelOpts.String() + `</select>
<button type="submit" id="agent-submit">Ask Agent</button>
</div>
<input type="hidden" id="agent-context" value="` + htmlEsc(contextID) + `">
</form>
</div>

<div id="agent-progress" style="display:none;">
<div class="card">
<h4 style="margin:0 0 12px;">Working…</h4>
<div id="agent-steps"></div>
</div>
</div>

<div id="agent-result"></div>
` + renderHistorySection(r) + `
<style>
.agent-step{display:flex;align-items:center;gap:8px;padding:5px 0;font-size:14px;
  color:#555;border-bottom:1px solid #f5f5f5;}
.agent-step:last-child{border-bottom:none;}
.agent-step.done{color:#28a745;}
.agent-step.error{color:#dc3545;}
.step-icon{font-size:16px;flex-shrink:0;}
</style>

<script>
(function(){
var form=document.getElementById('agent-form');
if(!form)return;

function esc(s){
  return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

form.addEventListener('submit',function(e){
  e.preventDefault();
  var prompt=document.getElementById('agent-prompt').value.trim();
  var model=document.getElementById('agent-model').value;
  var contextId=document.getElementById('agent-context').value;
  if(!prompt)return;

  var btn=document.getElementById('agent-submit');
  btn.disabled=true;btn.textContent='Working…';

  var prog=document.getElementById('agent-progress');
  var steps=document.getElementById('agent-steps');
  var result=document.getElementById('agent-result');
  prog.style.display='block';steps.innerHTML='';result.innerHTML='';

  var body={prompt:prompt,model:model};
  if(contextId){body.context_id=contextId;}

  fetch('/agent',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(body)
  })
  .then(function(resp){
    if(!resp.ok&&resp.status===401){
      prog.style.display='none';
      result.innerHTML='<div class="card"><p>Please <a href="/login?redirect=/agent">login</a> to use the agent.</p></div>';
      btn.disabled=false;btn.textContent='Ask Agent';
      return;
    }
    var reader=resp.body.getReader();
    var decoder=new TextDecoder();
    var buf='';
    function read(){
      return reader.read().then(function(chunk){
        if(chunk.done)return;
        buf+=decoder.decode(chunk.value,{stream:true});
        var lines=buf.split('\n');
        buf=lines.pop();
        lines.forEach(function(line){
          if(!line.startsWith('data: '))return;
          try{
            var ev=JSON.parse(line.slice(6));
            if(ev.type==='thinking'){
              var d=document.createElement('div');
              d.className='agent-step';
              d.innerHTML='<span class="step-icon">🤔</span><span>'+esc(ev.message)+'</span>';
              steps.appendChild(d);
            } else if(ev.type==='tool_start'){
              var d=document.createElement('div');
              d.id='step-'+ev.name;d.className='agent-step';
              d.innerHTML='<span class="step-icon">⚙️</span><span>'+esc(ev.message)+'</span>';
              steps.appendChild(d);
            } else if(ev.type==='tool_done'){
              var d=document.getElementById('step-'+ev.name);
              if(d){
                d.className='agent-step done';
                d.innerHTML='<span class="step-icon">✓</span><span>'+esc(ev.message)+'</span>';
              }
            } else if(ev.type==='response'){
              prog.style.display='none';
              result.innerHTML=ev.html;
              // Update context for potential follow-up
              if(ev.flow_id){document.getElementById('agent-context').value=ev.flow_id;}
            } else if(ev.type==='error'){
              prog.style.display='none';
              result.innerHTML='<div class="card"><p style="color:#dc3545;">'+esc(ev.message)+'</p></div>';
            } else if(ev.type==='done'){
              btn.disabled=false;btn.textContent='Ask Agent';
            }
          }catch(ex){console.error('agent event parse error',ex,line);}
        });
        return read();
      });
    }
    return read();
  })
  .catch(function(err){
    prog.style.display='none';
    result.innerHTML='<div class="card"><p style="color:#dc3545;">Error: '+esc(err.message)+'</p></div>';
    btn.disabled=false;btn.textContent='Ask Agent';
  });
});
})();
</script>`

	html := app.RenderHTMLForRequest("Agent", "AI agent with access to all Mu tools", content, r)
	w.Write([]byte(html))
}

// renderHistorySection returns an HTML block showing the user's recent query history,
// or an empty string if the user is not authenticated.
func renderHistorySection(r *http.Request) string {
	_, acc := auth.TrySession(r)
	if acc == nil {
		return ""
	}

	flows := ListFlows(acc.ID)
	if len(flows) == 0 {
		return ""
	}
	if len(flows) > historyLimit {
		flows = flows[:historyLimit]
	}

	var b strings.Builder
	b.WriteString(`<div class="card" id="agent-history">`)
	b.WriteString(`<h4 style="margin:0 0 12px;">Recent queries</h4>`)
	for _, f := range flows {
		age := time.Since(f.CreatedAt)
		ageStr := FormatAge(age)
		b.WriteString(`<div style="display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px solid #f0f0f0;">`)
		b.WriteString(`<div style="min-width:0;flex:1;">`)
		b.WriteString(`<a href="/agent/flow/` + f.ID + `" style="font-size:14px;font-weight:600;display:block;color:#111;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">` + htmlEsc(f.Prompt) + `</a>`)
		b.WriteString(`<div style="font-size:12px;color:#888;margin-top:2px;">` + ageStr)
		if len(f.Steps) > 0 {
			tools := make([]string, 0, len(f.Steps))
			for _, s := range f.Steps {
				tools = append(tools, s.Tool)
			}
			b.WriteString(` · ` + strings.Join(tools, ", "))
		}
		b.WriteString(`</div></div>`)
		b.WriteString(`<div style="display:flex;gap:8px;flex-shrink:0;margin-left:12px;">`)
		b.WriteString(`<a href="/agent?continue=` + f.ID + `" class="link" style="font-size:12px;">Continue</a>`)
		b.WriteString(`<a href="/agent/flow/` + f.ID + `" class="link" style="font-size:12px;">View</a>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

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

	var b strings.Builder
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

	// Action buttons
	b.WriteString(`<div class="card" style="display:flex;gap:12px;align-items:center;">`)
	b.WriteString(`<a href="/agent?continue=` + f.ID + `" class="link">Continue this query →</a>`)
	b.WriteString(`<span style="color:#888;font-size:13px;">Share this URL to let others view this result</span>`)
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

// agentToolsDesc is the tool catalogue shown to the AI planner.
const agentToolsDesc = `Available tools (use exact name):
- news: Get latest news feed (no args)
- news_search: Search news articles (args: {"query":"search term"})
- web_search: Search the web for current information (args: {"q":"search term"})
- video_search: Search for videos (args: {"query":"search term"})
- markets: Get live market prices (args: {"category":"crypto|futures|commodities"})
- weather_forecast: Get weather forecast (args: {"lat":number,"lon":number})
- places_search: Search for places (args: {"q":"search name","near":"location"})
- places_nearby: Find places near a location (args: {"address":"location","radius":number})
- reminder: Get Islamic daily reminder (no args)
- search: Search all Mu content (args: {"q":"search term"})
- blog_list: Get recent blog posts (no args)
- wallet_balance: Check your wallet credit balance (no args)
- wallet_topup: Get available topup options to add credits to your wallet (no args)`

// handleQuery processes an agent query request with SSE streaming.
func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt    string `json:"prompt"`
		Model     string `json:"model"`
		ContextID string `json:"context_id"` // optional: prior flow to continue from
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, `{"error":"prompt required"}`, http.StatusBadRequest)
		return
	}

	// Require authentication
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	// Resolve model
	model := Models[0] // default: standard
	for _, m := range Models {
		if m.ID == req.Model {
			model = m
			break
		}
	}

	// Check wallet quota before starting
	if QuotaCheck != nil {
		canProceed, _, err := QuotaCheck(r, model.WalletOp)
		if !canProceed {
			msg := "Insufficient credits for agent query. Top up at /wallet."
			if err != nil {
				msg = err.Error()
			}
			http.Error(w, `{"error":"`+msg+`"}`, http.StatusPaymentRequired)
			return
		}
	}

	// Load prior flow context if continuing a conversation.
	var priorFlow *Flow
	if req.ContextID != "" {
		priorFlow = getFlow(req.ContextID)
	}

	// Start SSE stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

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

		planPrompt := &ai.Prompt{
			System: "You are an AI agent. Given a user question, output ONLY a JSON array of tool calls (no other text, no markdown).\n\n" +
				agentToolsDesc +
				"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tool calls. When the question asks for cross-source insights or correlations (e.g. news + markets, news + video), call multiple relevant tools. If no tools are needed output [].",
			Question: req.Prompt,
			Priority: ai.PriorityHigh,
			Provider: model.Provider,
			Model:    model.Model,
		}

		planResult, err := ai.Ask(planPrompt)
		if err != nil {
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

	for _, tc := range toolCalls {
		if tc.Tool == "" {
			continue
		}
		msg := toolLabel(tc.Tool)
		sse(w, map[string]any{"type": "tool_start", "name": tc.Tool, "message": msg})

		text, isErr, execErr := api.ExecuteTool(r, tc.Tool, tc.Args)
		if execErr != nil || isErr {
			app.Log("agent", "Tool %s failed: err=%v isErr=%v response=%.200s", tc.Tool, execErr, isErr, text)
			sse(w, map[string]any{
				"type":    "tool_done",
				"name":    tc.Tool,
				"message": tc.Tool + " — unavailable",
			})
			continue
		}

		// Cap context length passed to the synthesiser
		if len(text) > 8000 {
			text = text[:8000] + "…"
		}
		results = append(results, toolResult{Name: tc.Tool, Result: text, Args: tc.Args})
		sse(w, map[string]any{
			"type":    "tool_done",
			"name":    tc.Tool,
			"message": msg + " — done",
		})
	}

	// --- Step 3: synthesise response ---
	sse(w, map[string]any{"type": "thinking", "message": "Composing answer…"})

	var ragParts []string

	// Include prior flow context when continuing a conversation.
	if priorFlow != nil {
		ragParts = append(ragParts, fmt.Sprintf(
			"### Previous query\nUser asked: %s\n\nPrevious answer:\n%s",
			priorFlow.Prompt, priorFlow.Answer,
		))
	}

	for i, res := range results {
		ragText := formatToolResult(res.Name, res.Result, res.Args)
		results[i].Formatted = ragText
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", res.Name, ragText))
	}

	today := time.Now().UTC().Format("Monday, 2 January 2006 (UTC)")
	synthPrompt := &ai.Prompt{
		System: "You are a helpful assistant. Today's date is " + today + ". " +
			"The tool results below come from live data feeds — treat them as current information and use the article publication dates when reasoning about recency.\n\n" +
			"Answer the user's question using ONLY the tool results provided below.\n\n" +
			"IMPORTANT: For any prices, market values, weather conditions, or other real-time data, you MUST use " +
			"the exact values from the tool results. Do NOT use your training knowledge for current prices or live data — " +
			"it will be outdated. If no tool result contains the requested real-time data, say it is unavailable.\n\n" +
			"When results come from multiple sources (news, video, markets, weather, etc.), identify and highlight " +
			"connections and correlations between them — for example, how a market move relates to a news story, " +
			"or how videos cover the same topic appearing in the news.\n\n" +
			"Use markdown formatting. Summarise key information from any news articles, weather data, market prices or other structured data.",
		Rag:      ragParts,
		Question: req.Prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
	}

	answer, err := ai.Ask(synthPrompt)
	if err != nil {
		sse(w, map[string]any{"type": "error", "message": "Could not generate response: " + err.Error()})
		sse(w, map[string]any{"type": "done"})
		return
	}

	// Auto-save this query as a flow for history and sharing.
	flow := &Flow{
		ID:        newFlowID(),
		AccountID: acc.ID,
		Prompt:    req.Prompt,
		Answer:    answer,
		CreatedAt: time.Now().UTC(),
	}
	for _, res := range results {
		flow.Steps = append(flow.Steps, FlowStep{
			Tool:   res.Name,
			Args:   res.Args,
			Result: res.Result,
		})
	}
	if err := saveFlow(flow); err != nil {
		app.Log("agent", "Failed to save flow: %v", err)
	}

	// Render markdown answer wrapped in a card, then append typed result cards
	rendered := app.RenderString(answer)
	html := `<div class="card" id="agent-response">` + rendered + `</div>`

	// Append typed cards for structured tool results
	for _, res := range results {
		if card := renderResultCard(res.Name, res.Result, res.Args); card != "" {
			html += card
		}
	}

	// Append expandable tool call references
	if len(results) > 0 {
		html += `<div class="card" style="font-size:13px;"><h4 style="margin:0 0 8px;font-size:13px;color:#888;">References</h4>`
		for _, res := range results {
			html += renderToolCallRef(res.Name, res.Args, res.Formatted)
		}
		html += `</div>`
	}

	// Append a "Save & share" link for the saved flow.
	html += `<div class="card" style="font-size:13px;display:flex;gap:16px;align-items:center;">` +
		`<a href="/agent/flow/` + flow.ID + `" class="link">View saved flow ↗</a>` +
		`<a href="/agent?continue=` + flow.ID + `" class="link">Continue this query →</a>` +
		`</div>`

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
		"news":      {{Tool: "news", Args: map[string]any{}}},
		"markets":   {{Tool: "markets", Args: map[string]any{}}},
		"market":    {{Tool: "markets", Args: map[string]any{}}},
		"prices":    {{Tool: "markets", Args: map[string]any{}}},
		"video":     {{Tool: "video_search", Args: map[string]any{"query": "latest"}}},
		"videos":    {{Tool: "video_search", Args: map[string]any{"query": "latest"}}},
		"weather":   {{Tool: "weather_forecast", Args: map[string]any{"lat": 51.5074, "lon": -0.1278}}},
		"reminder":  {{Tool: "reminder", Args: map[string]any{}}},
		// Starter pill phrases
		"give me a summary of today's top news":         {{Tool: "news", Args: map[string]any{}}},
		"what's in the news?":                           {{Tool: "news", Args: map[string]any{}}},
		"what are the latest crypto and market prices?": {{Tool: "markets", Args: map[string]any{}}},
		"find me the latest tech videos":                {{Tool: "video_search", Args: map[string]any{"query": "tech"}}},
		"what's the weather like in london today?":      {{Tool: "weather_forecast", Args: map[string]any{"lat": 51.5074, "lon": -0.1278}}},
		"search the web for the latest ai news":        {{Tool: "web_search", Args: map[string]any{"q": "latest AI news"}}},
		"show me today's islamic reminder":             {{Tool: "reminder", Args: map[string]any{}}},
	}
	if tc, ok := aliases[strings.ToLower(strings.TrimSpace(prompt))]; ok {
		return tc
	}
	return nil
}

// extractJSONArray extracts the first JSON array `[…]` from text produced by the AI.
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
	case "news_search":
		return "🔍 Searching news"
	case "web_search":
		return "🌐 Searching the web"
	case "video_search":
		return "🎬 Searching videos"
	case "markets":
		return "📈 Checking market prices"
	case "weather_forecast":
		return "🌤 Getting weather forecast"
	case "places_search":
		return "📍 Searching places"
	case "places_nearby":
		return "📍 Finding nearby places"
	case "reminder":
		return "📿 Getting daily reminder"
	case "search":
		return "🔍 Searching Mu"
	case "blog_list":
		return "📝 Reading blog posts"
	case "wallet_balance":
		return "💳 Checking wallet balance"
	case "wallet_topup":
		return "💳 Getting topup options"
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

// renderResultCard parses a tool's JSON result and returns an HTML card, or "" if
// the result type is not handled (the AI summary card is always shown).
func renderResultCard(toolName, result string, args map[string]any) string {
	switch toolName {
	case "news", "news_search":
		return renderNewsCard(result)
	case "video_search":
		return renderVideoCard(result)
	case "markets":
		return renderMarketsCard(result)
	case "weather_forecast":
		return renderWeatherCard(result)
	case "places_search", "places_nearby":
		return renderPlacesCard(result, args)
	}
	return ""
}

// --- typed card renderers ---

func renderNewsCard(result string) string {
	var data struct {
		Feed    []newsItem `json:"feed"`
		Results []newsItem `json:"results"`
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
	if len(items) > 5 {
		items = items[:5]
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📰 News</h4>`)
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

type newsItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Category string `json:"category"`
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

func renderMarketsCard(result string) string {
	var data struct {
		Data []marketItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	if len(data.Data) == 0 {
		return ""
	}
	items := data.Data
	if len(items) > 6 {
		items = items[:6]
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>📈 Markets</h4>`)
	b.WriteString(`<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:8px;">`)
	for _, item := range items {
		chg := ""
		if item.Change24h != 0 {
			sign := "+"
			color := "#28a745"
			if item.Change24h < 0 {
				sign = ""
				color = "#dc3545"
			}
			chg = fmt.Sprintf(`<span style="font-size:11px;color:%s;">%s%.1f%%</span>`, color, sign, item.Change24h)
		}
		price := formatPrice(item.Price)
		b.WriteString(fmt.Sprintf(
			`<div style="background:#f9f9f9;border-radius:6px;padding:8px;text-align:center;">
<div style="font-size:11px;font-weight:700;color:#555;">%s</div>
<div style="font-size:15px;font-weight:800;">%s %s</div>
</div>`,
			htmlEsc(item.Symbol), price, chg,
		))
	}
	b.WriteString(`</div><a href="/markets" class="link" style="display:inline-block;margin-top:8px;">More →</a></div>`)
	return b.String()
}

type marketItem struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h"`
}

func renderWeatherCard(result string) string {
	var data struct {
		Forecast struct {
			Current struct {
				TempC       float64 `json:"TempC"`
				FeelsLikeC  float64 `json:"FeelsLikeC"`
				Description string  `json:"Description"`
				Humidity    int     `json:"Humidity"`
				WindKph     float64 `json:"WindKph"`
			} `json:"Current"`
			Location string `json:"Location"`
		} `json:"forecast"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return ""
	}
	cur := data.Forecast.Current
	if cur.Description == "" {
		return ""
	}

	loc := data.Forecast.Location
	var b strings.Builder
	b.WriteString(`<div class="card"><h4>🌤 Weather`)
	if loc != "" {
		b.WriteString(` — ` + htmlEsc(loc))
	}
	b.WriteString(`</h4>`)
	b.WriteString(fmt.Sprintf(
		`<p style="font-size:28px;font-weight:800;margin:4px 0;">%.0f°C</p>`,
		cur.TempC,
	))
	b.WriteString(`<p style="color:#555;">` + htmlEsc(cur.Description) + `</p>`)
	b.WriteString(fmt.Sprintf(
		`<p style="font-size:13px;color:#888;">Feels like %.0f°C · Humidity %d%% · Wind %.0f km/h</p>`,
		cur.FeelsLikeC, cur.Humidity, cur.WindKph,
	))
	b.WriteString(`<a href="/weather" class="link" style="display:inline-block;margin-top:8px;">Full forecast →</a></div>`)
	return b.String()
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
		return formatNewsResult(result)
	case "video_search":
		return formatVideoResult(result)
	case "weather_forecast":
		return formatWeatherResult(result)
	case "reminder":
		return formatReminderResult(result)
	case "search":
		return formatSearchResult(result)
	case "blog_list":
		return formatBlogResult(result)
	case "web_search":
		return formatWebSearchResult(result)
	case "markets":
		return formatMarketsResult(result)
	case "places_search", "places_nearby":
		return formatPlacesResult(result, args)
	case "wallet_balance":
		return formatWalletBalanceResult(result)
	case "wallet_topup":
		return formatWalletTopupResult(result)
	}
	return result
}

// formatNewsResult converts a raw JSON news feed or search result into
// human-readable text for the AI synthesis RAG context.
func formatNewsResult(result string) string {
	var data struct {
		Feed    []struct {
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
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	type item struct {
		Title       string
		Description string
		Category    string
		URL         string
		PostedAt    string
		Published   string
	}
	var items []item
	for _, a := range data.Results {
		items = append(items, item{a.Title, a.Description, a.Category, a.URL, a.PostedAt, ""})
	}
	if len(items) == 0 {
		for _, a := range data.Feed {
			items = append(items, item{a.Title, a.Description, a.Category, a.URL, a.PostedAt, a.Published})
		}
	}
	if len(items) == 0 {
		return "No news available."
	}

	// Interleave items across categories round-robin to ensure diversity.
	// The raw feed groups items by category, so naively slicing gives only
	// the first category. Instead, pick up to 2 items per category in
	// round-robin order.
	if data.Query == "" {
		catOrder := []string{}
		catItems := map[string][]item{}
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
		var mixed []item
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
	for i, a := range items {
		line := fmt.Sprintf("%d. %s", i+1, a.Title)
		if a.Category != "" {
			line += fmt.Sprintf(" [%s]", a.Category)
		}
		if a.PostedAt != "" {
			if t, err := time.Parse(time.RFC3339, a.PostedAt); err == nil {
				line += fmt.Sprintf(" (%s)", t.Format("2 Jan 2006 15:04 UTC"))
			}
		} else if a.Published != "" {
			line += fmt.Sprintf(" (%s)", a.Published)
		}
		if a.URL != "" {
			line += " " + a.URL
		}
		if a.Description != "" {
			desc := a.Description
			if len(desc) > 150 {
				desc = desc[:150] + "…"
			}
			line += " — " + desc
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
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

// formatWeatherResult converts a raw JSON weather forecast result into
// human-readable text for the AI synthesis RAG context.
func formatWeatherResult(result string) string {
	var data struct {
		Forecast struct {
			Location string `json:"Location"`
			Current  struct {
				TempC       float64 `json:"TempC"`
				FeelsLikeC  float64 `json:"FeelsLikeC"`
				Description string  `json:"Description"`
				Humidity    int     `json:"Humidity"`
				WindKph     float64 `json:"WindKph"`
			} `json:"Current"`
			DailyItems []struct {
				Date        string  `json:"Date"`
				MaxTempC    float64 `json:"MaxTempC"`
				MinTempC    float64 `json:"MinTempC"`
				Description string  `json:"Description"`
				WillRain    bool    `json:"WillRain"`
				RainMM      float64 `json:"RainMM"`
			} `json:"DailyItems"`
		} `json:"forecast"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	cur := data.Forecast.Current
	if cur.Description == "" && cur.TempC == 0 {
		return "Weather data unavailable."
	}
	var sb strings.Builder
	if data.Forecast.Location != "" {
		sb.WriteString(fmt.Sprintf("Weather for %s:\n", data.Forecast.Location))
	} else {
		sb.WriteString("Current weather:\n")
	}
	sb.WriteString(fmt.Sprintf("- Temperature: %.0f°C (feels like %.0f°C)\n", cur.TempC, cur.FeelsLikeC))
	if cur.Description != "" {
		sb.WriteString(fmt.Sprintf("- Conditions: %s\n", cur.Description))
	}
	if cur.Humidity > 0 {
		sb.WriteString(fmt.Sprintf("- Humidity: %d%%\n", cur.Humidity))
	}
	if cur.WindKph > 0 {
		sb.WriteString(fmt.Sprintf("- Wind: %.0f km/h\n", cur.WindKph))
	}
	if len(data.Forecast.DailyItems) > 0 {
		sb.WriteString("Forecast:\n")
		days := data.Forecast.DailyItems
		if len(days) > 5 {
			days = days[:5]
		}
		for _, d := range days {
			line := fmt.Sprintf("- %.0f°C / %.0f°C, %s", d.MaxTempC, d.MinTempC, d.Description)
			if d.WillRain {
				line += fmt.Sprintf(" (rain: %.1fmm)", d.RainMM)
			}
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

// formatReminderResult converts a raw JSON reminder result into
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
		return "Reminder data unavailable."
	}
	var sb strings.Builder
	sb.WriteString("Daily Islamic reminder:\n")
	if data.Name != "" {
		sb.WriteString(fmt.Sprintf("Name of Allah: %s\n", data.Name))
	}
	if data.Verse != "" {
		sb.WriteString(fmt.Sprintf("Verse: %s\n", data.Verse))
	}
	if data.Hadith != "" {
		sb.WriteString(fmt.Sprintf("Hadith: %s\n", data.Hadith))
	}
	if data.Message != "" {
		sb.WriteString(fmt.Sprintf("Message: %s\n", data.Message))
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

// formatBlogResult converts a raw JSON blog list result into
// human-readable text for the AI synthesis RAG context.
func formatBlogResult(result string) string {
	var posts []struct {
		Title     string `json:"title"`
		Author    string `json:"author"`
		Tags      string `json:"tags"`
		CreatedAt string `json:"created_at"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &posts); err != nil {
		return result
	}
	if len(posts) == 0 {
		return "No blog posts available."
	}
	if len(posts) > 10 {
		posts = posts[:10]
	}
	var sb strings.Builder
	sb.WriteString("Recent blog posts:\n")
	for i, p := range posts {
		line := fmt.Sprintf("%d. %s", i+1, p.Title)
		if p.Author != "" {
			line += fmt.Sprintf(" by %s", p.Author)
		}
		if p.Tags != "" {
			line += fmt.Sprintf(" [%s]", p.Tags)
		}
		if p.Content != "" {
			snippet := p.Content
			if len(snippet) > 120 {
				snippet = snippet[:120] + "…"
			}
			line += " — " + snippet
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// formatWebSearchResult converts a raw JSON web search result into
// human-readable text for the AI synthesis RAG context.
func formatWebSearchResult(result string) string {
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"results"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No web results found."
	}
	items := data.Results
	if len(items) > 10 {
		items = items[:10]
	}
	var sb strings.Builder
	if data.Query != "" {
		sb.WriteString(fmt.Sprintf("Web search results for %q:\n", data.Query))
	} else {
		sb.WriteString("Web search results:\n")
	}
	for i, r := range items {
		line := fmt.Sprintf("%d. %s", i+1, r.Title)
		if r.Snippet != "" {
			line += " — " + r.Snippet
		}
		sb.WriteString(line + "\n")
	}
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

// formatMarketsResult converts a raw JSON markets result into a human-readable
// text summary suitable for inclusion in the AI synthesis RAG context.
func formatMarketsResult(result string) string {
	var data struct {
		Category string `json:"category"`
		Data     []struct {
			Symbol    string  `json:"symbol"`
			Price     float64 `json:"price"`
			Change24h float64 `json:"change_24h"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Data) == 0 {
		return "No market data available."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Live %s market prices:\n", data.Category))
	for _, item := range data.Data {
		line := fmt.Sprintf("- %s: $%.2f", item.Symbol, item.Price)
		if item.Change24h != 0 {
			sign := "+"
			if item.Change24h < 0 {
				sign = ""
			}
			line += fmt.Sprintf(" (24h change: %s%.2f%%)", sign, item.Change24h)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// formatWalletBalanceResult converts a raw JSON wallet balance result into
// human-readable text for the AI synthesis RAG context.
func formatWalletBalanceResult(result string) string {
	var data struct {
		Balance int `json:"balance"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	pounds := data.Balance / 100
	pence := data.Balance % 100
	return fmt.Sprintf("Wallet balance: %d credits (£%d.%02d). Top up at /wallet/topup.\n", data.Balance, pounds, pence)
}

// formatWalletTopupResult converts a raw JSON wallet topup methods result into
// human-readable text for the AI synthesis RAG context.
func formatWalletTopupResult(result string) string {
	var data struct {
		Methods []struct {
			Type  string `json:"type"`
			Tiers []struct {
				Amount  int    `json:"amount"`
				Credits int    `json:"credits"`
				Label   string `json:"label"`
			} `json:"tiers"`
		} `json:"methods"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Methods) == 0 {
		return "No topup methods available. Visit /wallet/topup to add credits."
	}
	var sb strings.Builder
	sb.WriteString("Wallet topup options (visit /wallet/topup to add credits):\n")
	for _, m := range data.Methods {
		sb.WriteString(fmt.Sprintf("%s payment:\n", m.Type))
		for _, t := range m.Tiers {
			sb.WriteString(fmt.Sprintf("- %s: %d credits\n", t.Label, t.Credits))
		}
	}
	return sb.String()
}

// htmlEsc escapes a string for safe HTML attribute/text inclusion.
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
