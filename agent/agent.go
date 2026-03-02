// Package agent provides a conversational AI agent interface that has access
// to all Mu tools via the MCP server, using the user's session token for calls.
package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mu/ai"
	"mu/api"
	"mu/app"
	"mu/auth"
)

// Model represents an available LLM model tier for agent queries.
type Model struct {
	ID       string
	Name     string
	Desc     string
	WalletOp string
	Provider string // ai provider constant, empty = default
}

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
	},
}

// QuotaCheck is set by main.go to wire in the wallet quota check without an
// import cycle. Signature matches api.QuotaCheck.
var QuotaCheck func(r *http.Request, op string) (bool, int, error)

// Load initialises the agent package (no-op for now; reserved for future use).
func Load() {}

// Handler dispatches GET (page) and POST (query) at /agent.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		servePage(w, r)
	case "POST":
		handleQuery(w, r)
	default:
		app.MethodNotAllowed(w, r)
	}
}

// servePage renders the static agent chat page.
func servePage(w http.ResponseWriter, r *http.Request) {
	var modelOpts strings.Builder
	for _, m := range Models {
		modelOpts.WriteString(fmt.Sprintf(
			`<option value="%s">%s — %s</option>`, m.ID, m.Name, m.Desc,
		))
	}

	content := `<div class="card">
<h2>Agent</h2>
<p class="card-desc">Ask a question and the agent will search news, weather, places, markets, video and more to answer it.</p>
<form id="agent-form">
<textarea id="agent-prompt" name="prompt" rows="3"
  placeholder="What would you like to know?"
  style="width:100%;box-sizing:border-box;padding:8px;font-family:inherit;font-size:15px;resize:vertical;border:1px solid #ddd;border-radius:4px;"></textarea>
<div style="display:flex;gap:8px;margin-top:8px;align-items:center;flex-wrap:wrap;">
<select id="agent-model"
  style="padding:6px 10px;font-family:inherit;font-size:13px;border:1px solid #ddd;border-radius:4px;">` +
		modelOpts.String() + `</select>
<button type="submit" id="agent-submit">Ask Agent</button>
</div>
</form>
</div>

<div id="agent-progress" style="display:none;">
<div class="card">
<h4 style="margin:0 0 12px;">Working…</h4>
<div id="agent-steps"></div>
</div>
</div>

<div id="agent-result"></div>

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
  if(!prompt)return;

  var btn=document.getElementById('agent-submit');
  btn.disabled=true;btn.textContent='Working…';

  var prog=document.getElementById('agent-progress');
  var steps=document.getElementById('agent-steps');
  var result=document.getElementById('agent-result');
  prog.style.display='block';steps.innerHTML='';result.innerHTML='';

  fetch('/agent',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({prompt:prompt,model:model})
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
- web_search: Search the web for current information (args: {"query":"search term"})
- video_search: Search for videos (args: {"query":"search term"})
- markets: Get live market prices (args: {"category":"crypto|futures|commodities"})
- weather_forecast: Get weather forecast (args: {"lat":number,"lon":number})
- places_search: Search for places (args: {"q":"search name","near":"location"})
- places_nearby: Find places near a location (args: {"address":"location","radius":number})
- reminder: Get Islamic daily reminder (no args)
- search: Search all Mu content (args: {"q":"search term"})
- blog_list: Get recent blog posts (no args)`

// handleQuery processes an agent query request with SSE streaming.
func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
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
	_ = acc

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

	// Start SSE stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	// --- Step 1: plan tool calls ---
	sse(w, map[string]any{"type": "thinking", "message": "Planning your request…"})

	planPrompt := &ai.Prompt{
		System: "You are an AI agent. Given a user question, output ONLY a JSON array of tool calls (no other text, no markdown).\n\n" +
			agentToolsDesc +
			"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 3 tool calls. If no tools are needed output [].",
		Question: req.Prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
	}

	planResult, err := ai.Ask(planPrompt)
	if err != nil {
		sse(w, map[string]any{"type": "error", "message": "Could not plan request: " + err.Error()})
		sse(w, map[string]any{"type": "done"})
		return
	}

	// Parse tool calls (the AI may wrap JSON in markdown fences)
	type toolCall struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	planJSON := extractJSONArray(planResult)
	var toolCalls []toolCall
	json.Unmarshal([]byte(planJSON), &toolCalls) //nolint:errcheck — fallback to empty slice

	// --- Step 2: execute tool calls ---
	type toolResult struct {
		Name   string
		Result string
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
			sse(w, map[string]any{
				"type":    "tool_done",
				"name":    tc.Tool,
				"message": tc.Tool + " — unavailable",
			})
			continue
		}

		// Cap context length passed to the synthesiser
		if len(text) > 4000 {
			text = text[:4000] + "…"
		}
		results = append(results, toolResult{Name: tc.Tool, Result: text})
		sse(w, map[string]any{
			"type":    "tool_done",
			"name":    tc.Tool,
			"message": msg + " — done",
		})
	}

	// --- Step 3: synthesise response ---
	sse(w, map[string]any{"type": "thinking", "message": "Composing answer…"})

	var ragParts []string
	for _, res := range results {
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", res.Name, res.Result))
	}

	synthPrompt := &ai.Prompt{
		System: "You are a helpful assistant. Using the tool results provided (if any), answer the user's question clearly and concisely. " +
			"Use markdown formatting. Summarise key information from any news articles, weather data, market prices or other structured data.",
		Rag:      ragParts,
		Question: req.Prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
	}

	answer, err := ai.Ask(synthPrompt)
	if err != nil {
		sse(w, map[string]any{"type": "error", "message": "Could not generate response: " + err.Error()})
		sse(w, map[string]any{"type": "done"})
		return
	}

	// Render markdown answer wrapped in a card, then append typed result cards
	rendered := app.RenderString(answer)
	html := `<div class="card" id="agent-response">` + rendered + `</div>`

	// Append typed cards for structured tool results
	for _, res := range results {
		if card := renderResultCard(res.Name, res.Result); card != "" {
			html += card
		}
	}

	sse(w, map[string]any{"type": "response", "html": html})
	sse(w, map[string]any{"type": "done"})
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
	default:
		return "⚙ Calling " + tool
	}
}

// renderResultCard parses a tool's JSON result and returns an HTML card, or "" if
// the result type is not handled (the AI summary card is always shown).
func renderResultCard(toolName, result string) string {
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
		return renderPlacesCard(result)
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
				TempC       float64 `json:"temp_c"`
				FeelsLikeC  float64 `json:"feels_like_c"`
				Description string  `json:"description"`
				Humidity    int     `json:"humidity"`
				WindKph     float64 `json:"wind_kph"`
			} `json:"current"`
			Location string `json:"location"`
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

func renderPlacesCard(result string) string {
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
	b.WriteString(`<a href="/places" class="link" style="display:inline-block;margin-top:8px;">Open map →</a></div>`)
	return b.String()
}

type placeItem struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Address  string `json:"address"`
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
