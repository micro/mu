package apps

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"

	"github.com/google/uuid"
)

// builderSystemPrompt is a generic HTML app builder — no platform-specific knowledge.
// SDK docs are passed as context by the caller when needed.
const builderSystemPrompt = `You are an app builder. You generate complete, self-contained HTML apps.

Output format:
- Output ONLY valid JSON: {"name":"Short Name","icon":"<svg>...</svg>","html":"<!DOCTYPE html>..."}
- The name should be 2-4 words (max 50 chars)
- The icon: SVG, viewBox="0 0 32 32", stroke="#555", fill="none", stroke-width 1.2-2.5
- The html: complete document with <!DOCTYPE html><html><head><style>...</style></head><body>...<script>...</script></body></html>

Style:
- Font: 'Nunito Sans', sans-serif
- Clean minimal design: #fff background, #333 text, #e0e0e0 borders, 6px radius
- Buttons: padding 8-10px 20-24px, radius 6px, primary: background #000 color #fff
- No external dependencies, CDN links, or images
- Responsive and mobile-friendly

Rules:
- The app MUST have working JavaScript — not just a UI shell
- Always check for errors and null-check nested properties
- If using geolocation, provide a manual fallback input
- When modifying an existing app, return the complete updated JSON (not a diff)`

// SDKDocs returns the Mu platform SDK reference documentation.
// This is the single source of truth — used by the mu_sdk tool and
// passed to the builder as context when the app needs platform APIs.
func SDKDocs() string {
	return `# Mu SDK Reference

Apps run as full pages on the same origin. The SDK is auto-injected as window.mu — do NOT add script tags for it.

## Platform APIs (all return Promises with JSON)

mu.weather({lat, lon})        — weather forecast (requires lat/lon numbers, not city name)
mu.news()                     — latest news feed
mu.markets({category})        — market prices (category: 'crypto'|'futures'|'commodities')
mu.video()                    — latest videos
mu.blog.list()                — blog posts
mu.blog.read(id)              — single post
mu.blog.create({title, content}) — create post
mu.social()                   — social threads
mu.places.search({q, near})   — search places (e.g. {q:'cafe', near:'London'})
mu.places.nearby({address, radius}) — nearby places
mu.chat(prompt)               — AI chat
mu.search(query)              — search all content
mu.apps.list()                — list apps
mu.ai(prompt)                 — ask AI, returns response text
mu.agent(prompt)              — full agent: plans tools, executes, returns answer
mu.user()                     — current user info

## Storage (persistent, namespaced per app)

mu.store.set(key, value)
mu.store.get(key)
mu.store.del(key)
mu.store.keys()

## Raw fetch (for any endpoint on the platform)

mu.get(path)          — GET, returns JSON
mu.post(path, body)   — POST, returns JSON

## Response shapes (EXACT — use these field names)

Weather:
  var data = await mu.weather({lat: 51.5, lon: -0.12})
  data.forecast.Current.TempC          // number, e.g. 15.0
  data.forecast.Current.FeelsLikeC     // number
  data.forecast.Current.Description    // string, e.g. "Partly cloudy"
  data.forecast.Current.Humidity       // number, e.g. 65
  data.forecast.Current.WindKph        // number
  data.forecast.DailyItems             // array of {MaxTempC, MinTempC, Description, WillRain, RainMM}
  data.forecast.HourlyItems            // array of {TempC, Description}

Markets:
  var data = await mu.markets({category: 'crypto'})
  data.category                        // string, e.g. "crypto"
  data.data                            // array of items:
  data.data[0].symbol                  // string, e.g. "BTC"
  data.data[0].price                   // number, e.g. 66556.03
  data.data[0].change_24h              // number, e.g. -0.68 (percentage)
  data.data[0].type                    // string, e.g. "crypto"

News:
  var data = await mu.news()
  data.feed                            // array of items:
  data.feed[0].title                   // string
  data.feed[0].description             // string
  data.feed[0].url                     // string
  data.feed[0].category                // string
  data.feed[0].published               // string (date)
  data.feed[0].image                   // string (URL, may be empty)

## Important notes

- mu.weather() requires lat/lon — use geolocation or mu.places.search() to geocode city names
- Do NOT add <script src="/apps/sdk.js"> — the SDK is already injected
- Do NOT load external scripts or CDN links
- Always handle errors: if(!data || data.error){showError(data.error||'Failed to load');return}
- Markets data is in data.data (array), news is in data.feed (array) — always check the wrapper`
}

// handleBuilder serves the app builder page.
func handleBuilder(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	// Check for template parameter
	templateID := r.URL.Query().Get("template")
	initialCode := ""
	if t := GetTemplate(templateID); t != nil {
		initialCode = t.HTML
	}

	var sb strings.Builder
	sb.WriteString(builderPageHTML(initialCode))

	app.Respond(w, r, app.Response{
		Title:       "Build",
		Description: "Build an app with AI or code",
		HTML:        sb.String(),
	})
}

// handleGenerate processes AI generation requests.
func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}

	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
		Code   string `json:"code"` // Existing code for follow-on prompts
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		app.RespondError(w, http.StatusBadRequest, "Prompt is required")
		return
	}

	// Build the AI prompt
	question := req.Prompt
	var rag []string
	if req.Code != "" {
		rag = append(rag, "Current app HTML that the user wants to modify:\n```html\n"+req.Code+"\n```")
		question = "Modify this existing app: " + req.Prompt
	}

	prompt := &ai.Prompt{
		System:   builderSystemPrompt + "\n\n" + SDKDocs(),
		Rag:      rag,
		Question: question,
		Model:    "claude-opus-4-20250514",
		Priority: ai.PriorityHigh,
		Caller:   "app-builder",
	}

	result, err := ai.Ask(prompt)
	if err != nil {
		app.Log("apps", "AI generation error: %v", err)
		app.RespondError(w, http.StatusInternalServerError, "Failed to generate app. Please try again.")
		return
	}

	// Parse the JSON response from AI
	result = cleanGeneratedJSON(result)

	var generated struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(result), &generated); err != nil {
		// Fallback: treat entire response as HTML (backward compat)
		generated.HTML = cleanGeneratedHTML(result)
	}
	if generated.HTML == "" {
		generated.HTML = cleanGeneratedHTML(result)
	}

	resp := map[string]string{"html": generated.HTML}
	if generated.Name != "" {
		resp["name"] = generated.Name
	}
	if generated.Icon != "" {
		resp["icon"] = generated.Icon
	}
	app.RespondJSON(w, resp)
}

// BuildAndSave generates an app from a prompt, saves it, and returns the app.
// Used by the MCP apps_build tool so the agent can create apps in one step.
func BuildAndSave(prompt, authorID, authorName string) (*App, error) {
	aiPrompt := &ai.Prompt{
		System:   builderSystemPrompt + "\n\n" + SDKDocs(),
		Question: prompt,
		Model:    "claude-opus-4-20250514",
		Priority: ai.PriorityHigh,
		Caller:   "app-builder",
	}
	result, err := ai.Ask(aiPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI generation failed: %v", err)
	}

	// Parse JSON response from AI
	result = cleanGeneratedJSON(result)
	var generated struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(result), &generated); err != nil {
		generated.HTML = cleanGeneratedHTML(result)
	}
	if generated.HTML == "" {
		generated.HTML = cleanGeneratedHTML(result)
	}
	if generated.HTML == "" {
		return nil, fmt.Errorf("AI returned empty HTML")
	}

	// Use AI-generated name or derive from prompt
	name := generated.Name
	if name == "" {
		name = prompt
		if len(name) > 50 {
			name = name[:50]
		}
	}
	slug := slugify(name)
	if len(slug) < 3 {
		slug = "app-" + slug
	}

	// Ensure unique slug
	mutex.RLock()
	base := slug
	for i := 2; apps[slug] != nil; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	mutex.RUnlock()

	now := time.Now()
	a := &App{
		ID:          uuid.New().String(),
		Slug:        slug,
		Name:        name,
		Description: prompt,
		AuthorID:    authorID,
		Author:      authorName,
		Icon:        generated.Icon,
		HTML:        generated.HTML,
		Public:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutex.Lock()
	snapshotVersion(a, "Initial version")
	apps[slug] = a
	mutex.Unlock()
	save()

	app.Log("apps", "Agent built app %q for %s", name, authorID)
	event.Publish(event.Event{Type: "apps_updated"})
	return a, nil
}

// EditApp uses AI to fix/modify an existing app based on a prompt.
// Returns the updated app or an error.
func EditApp(slug, prompt, accountID string) (*App, error) {
	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		return nil, fmt.Errorf("app not found: %s", slug)
	}

	// Ask AI to fix the app with current HTML as context
	question := fmt.Sprintf("Here is the current app HTML:\n\n%s\n\nModification requested: %s", a.HTML, prompt)
	aiPrompt := &ai.Prompt{
		System:   builderSystemPrompt + "\n\n" + SDKDocs(),
		Question: question,
		Model:    "claude-opus-4-20250514",
		Priority: ai.PriorityHigh,
		Caller:   "app-edit",
	}
	result, err := ai.Ask(aiPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI edit failed: %v", err)
	}

	result = cleanGeneratedJSON(result)
	var generated struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(result), &generated); err != nil {
		generated.HTML = cleanGeneratedHTML(result)
	}
	if generated.HTML == "" {
		generated.HTML = cleanGeneratedHTML(result)
	}
	if generated.HTML == "" {
		return nil, fmt.Errorf("AI returned empty HTML")
	}

	mutex.Lock()
	a.HTML = generated.HTML
	if generated.Name != "" {
		a.Name = generated.Name
	}
	if generated.Icon != "" {
		a.Icon = generated.Icon
	}
	a.UpdatedAt = time.Now()
	snapshotVersion(a, prompt)
	mutex.Unlock()
	save()

	app.Log("apps", "Agent edited app %q for %s", a.Name, accountID)
	return a, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	// Replace non-alphanumeric with hyphens
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if c == ' ' || c == '-' || c == '_' {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 50 {
		result = result[:50]
	}
	return strings.Trim(result, "-")
}

// handleTemplateList returns available templates as JSON.
func handleTemplateList(w http.ResponseWriter, r *http.Request) {
	type templateSummary struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	summaries := make([]templateSummary, len(Templates))
	for i, t := range Templates {
		summaries[i] = templateSummary{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Category:    t.Category,
		}
	}
	app.RespondJSON(w, summaries)
}

// handleTemplateGet returns a specific template's HTML.
func handleTemplateGet(w http.ResponseWriter, r *http.Request, id string) {
	t := GetTemplate(id)
	if t == nil {
		app.RespondError(w, http.StatusNotFound, "Template not found")
		return
	}
	app.RespondJSON(w, t)
}

// BuilderSystemPrompt returns the system prompt used for app generation.
func BuilderSystemPrompt() string {
	return builderSystemPrompt
}

// CleanGeneratedHTML extracts HTML from AI output, stripping code fences.
func CleanGeneratedHTML(s string) string {
	return cleanGeneratedHTML(s)
}

// cleanGeneratedJSON strips markdown code fences from AI JSON output.
func cleanGeneratedJSON(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

// cleanGeneratedHTML strips markdown code fences and whitespace from AI output.
func cleanGeneratedHTML(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```html ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

// builderPageHTML returns the HTML for the app builder interface.
func builderPageHTML(initialCode string) string {
	escapedCode, _ := json.Marshal(initialCode)
	escapedSDK, _ := json.Marshal(NativeSDK("_preview"))

	// Build template buttons
	var templateButtons strings.Builder
	for _, t := range Templates {
		templateButtons.WriteString(fmt.Sprintf(
			`<button onclick="loadTemplate('%s')" title="%s">%s</button>`,
			htmlpkg.EscapeString(t.ID),
			htmlpkg.EscapeString(t.Description),
			htmlpkg.EscapeString(t.Name),
		))
	}

	return fmt.Sprintf(`
<style>
.builder { display: flex; flex-direction: column; gap: 12px; }
.prompt-bar { display: flex; gap: 8px; }
.prompt-bar input { flex: 1; padding: 10px 14px; border: 1px solid #e0e0e0; border-radius: 6px; font-size: 15px; font-family: inherit; }
.prompt-bar button { padding: 10px 24px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; font-size: 15px; white-space: nowrap; }
.prompt-bar button:disabled { background: #ccc; cursor: not-allowed; }
.templates { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 4px; }
.templates button { padding: 4px 12px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; color: #333; cursor: pointer; font-size: 12px; font-family: inherit; }
.templates button:hover { background: #f5f5f5; color: #111; }
.preview-area { display: flex; flex-direction: column; min-height: 60vh; }
.preview-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.preview-header h3 { font-size: 14px; font-weight: 600; margin: 0; }
.preview-frame { flex: 1; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; min-height: 50vh; }
.code-toggle { padding: 4px 12px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; color: #333; cursor: pointer; font-size: 12px; font-family: inherit; }
.code-toggle:hover { background: #f5f5f5; color: #111; }
.code-section { display: none; margin-top: 12px; }
.code-section.visible { display: block; }
.code-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.code-header h3 { font-size: 14px; font-weight: 600; margin: 0; }
.code-header .actions { display: flex; gap: 6px; }
.code-header .actions button { padding: 4px 12px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; color: #333; cursor: pointer; font-size: 12px; font-family: inherit; }
.code-header .actions button:hover { background: #f5f5f5; color: #111; }
.code-editor { width: 100%%; min-height: 300px; padding: 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace; font-size: 13px; line-height: 1.5; resize: vertical; tab-size: 2; background: #fafafa; }
.save-bar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.save-bar input { padding: 8px 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; font-size: 14px; color: #333; box-sizing: border-box; }
.save-bar input.name { flex: 1; min-width: 150px; }
.save-bar button { padding: 8px 20px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; white-space: nowrap; }
.status-msg { font-size: 13px; color: #999; margin-left: 8px; }
.empty-state { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 50vh; color: #999; font-size: 15px; }
.empty-state p { margin: 4px 0; }
@media (max-width: 768px) {
  .save-bar { flex-direction: column; align-items: stretch; }
  .save-bar input.name { width: 100%%; min-width: auto; }
  .save-bar input { width: 100%%; }
  .prompt-bar { flex-direction: column; }
  .prompt-bar input, .prompt-bar button { width: 100%%; box-sizing: border-box; }
}
</style>

<div class="builder">
  <p class="card-desc">Describe what you want to build, or pick a template to start from.</p>

  <div class="templates">
    %s
  </div>

  <div class="prompt-bar">
    <input type="text" id="prompt" placeholder="Build me a pomodoro timer with sound alerts..." onkeydown="if(event.key==='Enter')generate()">
    <button id="genBtn" onclick="generate()">Generate</button>
  </div>

  <div class="preview-area">
    <div class="preview-header">
      <h3>Preview</h3>
      <button class="code-toggle" id="codeToggle" onclick="toggleCode()">Show Code</button>
    </div>
    <div id="emptyState" class="empty-state">
      <p>Your app preview will appear here.</p>
      <p>Type a prompt above or pick a template.</p>
    </div>
    <iframe id="preview" class="preview-frame" allow="geolocation" style="display:none;"></iframe>
    <div id="errorBar" style="display:none;padding:8px 12px;background:#fff0f0;border:1px solid #fcc;border-radius:6px;margin-top:6px;font-size:13px;color:#c00;cursor:pointer" onclick="fixErrors()" title="Click to auto-fix"></div>
  </div>

  <div class="code-section" id="codeSection">
    <div class="code-header">
      <h3>Code</h3>
      <div class="actions">
        <button onclick="copyCode()">Copy</button>
        <button onclick="updatePreview()">Refresh Preview</button>
      </div>
    </div>
    <textarea class="code-editor" id="code" spellcheck="false" placeholder="Your app's HTML will appear here..."></textarea>
  </div>

  <div class="save-bar">
    <input class="name" type="text" id="appName" placeholder="App name">
    <input type="text" id="appTags" placeholder="Tags (optional)" style="width:140px;">
    <button onclick="saveApp()">Save & Launch</button>
    <span class="status-msg" id="statusMsg"></span>
  </div>
</div>

<script>
var codeEl = document.getElementById('code');
var preview = document.getElementById('preview');
var emptyState = document.getElementById('emptyState');
var appIcon = '';
var previewSDK = %s;
var initialCode = %s;
if (initialCode) { codeEl.value = initialCode; showPreview(); }

// Tab key inserts spaces in the editor
codeEl.addEventListener('keydown', function(e) {
  if (e.key === 'Tab') {
    e.preventDefault();
    var start = this.selectionStart, end = this.selectionEnd;
    this.value = this.value.substring(0, start) + '  ' + this.value.substring(end);
    this.selectionStart = this.selectionEnd = start + 2;
  }
});

var lastErrors = [];

function showPreview() {
  var html = codeEl.value;
  if (!html.trim()) return;
  emptyState.style.display = 'none';
  preview.style.display = '';
  document.getElementById('errorBar').style.display = 'none';
  lastErrors = [];
  // Inject SDK and write HTML directly (same origin, no sandbox)
  var pdoc = preview.contentDocument || preview.contentWindow.document;
  pdoc.open(); pdoc.write(previewSDK + html); pdoc.close();
  // Check for errors after app loads
  setTimeout(function() {
    try {
      var iwin = preview.contentWindow;
      if (iwin && iwin.mu && iwin.mu.errors && iwin.mu.errors.length > 0) {
        lastErrors = iwin.mu.errors;
        var msgs = lastErrors.map(function(e){ return e.message; }).join('; ');
        var bar = document.getElementById('errorBar');
        bar.textContent = msgs.slice(0, 200) + ' — click to fix';
        bar.style.display = 'block';
      }
    } catch(e) {}
  }, 2500);
}

function updatePreview() { showPreview(); }

function fixErrors() {
  if (lastErrors.length === 0) return;
  var msgs = lastErrors.map(function(e){ return e.message; }).join('; ');
  document.getElementById('prompt').value = 'Fix these errors: ' + msgs;
  generate();
}

function toggleCode() {
  var section = document.getElementById('codeSection');
  var btn = document.getElementById('codeToggle');
  if (section.classList.contains('visible')) {
    section.classList.remove('visible');
    btn.textContent = 'Show Code';
  } else {
    section.classList.add('visible');
    btn.textContent = 'Hide Code';
  }
}

function generate() {
  var promptEl = document.getElementById('prompt');
  var p = promptEl.value.trim();
  if (!p) return;
  var btn = document.getElementById('genBtn');
  btn.disabled = true;
  btn.textContent = 'Generating...';
  document.getElementById('statusMsg').textContent = '';

  fetch('/apps/build/generate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt: p, code: codeEl.value })
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.error) { document.getElementById('statusMsg').textContent = data.error; return; }
    codeEl.value = data.html;
    showPreview();
    // Auto-fill name from AI response
    if (data.name) {
      document.getElementById('appName').value = data.name;
    } else if (!document.getElementById('appName').value) {
      var name = p.length > 50 ? p.substring(0, 50) : p;
      document.getElementById('appName').value = name.charAt(0).toUpperCase() + name.slice(1);
    }
    if (data.icon) { appIcon = data.icon; }
  })
  .catch(function(e) { document.getElementById('statusMsg').textContent = 'Error: ' + e.message; })
  .finally(function() { btn.disabled = false; btn.textContent = 'Generate'; });
}

function loadTemplate(id) {
  fetch('/apps/build/templates/' + id)
  .then(function(r) { return r.json(); })
  .then(function(t) {
    codeEl.value = t.html;
    showPreview();
    if (!document.getElementById('appName').value) document.getElementById('appName').value = t.name;
  });
}

function saveApp() {
  var name = document.getElementById('appName').value.trim();
  var tags = (document.getElementById('appTags').value || '').trim();
  var html = codeEl.value.trim();
  if (!name) { document.getElementById('statusMsg').textContent = 'App name is required'; return; }
  var slug = slugify(name);
  if (!html) { document.getElementById('statusMsg').textContent = 'Generate or write some code first'; return; }

  document.getElementById('statusMsg').textContent = 'Saving...';
  fetch('/apps/new', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, slug: slug, icon: appIcon, description: name, tags: tags, html: html, public: true })
  })
  .then(function(r) {
    if (!r.ok) {
      return r.text().then(function(t) {
        try { var j = JSON.parse(t); throw new Error(j.error || 'Save failed'); }
        catch(e) { if (e.message) throw e; throw new Error('Save failed (status ' + r.status + ')'); }
      });
    }
    return r.json();
  })
  .then(function(data) {
    if (data.error) { document.getElementById('statusMsg').textContent = data.error; return; }
    document.getElementById('statusMsg').textContent = 'Saved!';
    window.location.href = '/apps/' + (data.slug || slug);
  })
  .catch(function(e) { document.getElementById('statusMsg').textContent = e.message || 'Save failed'; });
}

function copyCode() {
  navigator.clipboard.writeText(codeEl.value).then(function() {
    document.getElementById('statusMsg').textContent = 'Copied!';
    setTimeout(function() { document.getElementById('statusMsg').textContent = ''; }, 2000);
  });
}

function slugify(s) {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').substring(0, 50);
}
</script>`, templateButtons.String(), string(escapedSDK), string(escapedCode))
}

// editPageHTML returns the HTML for editing an existing app (reuses builder UI).
func editPageHTML(a *App) string {
	escapedCode, _ := json.Marshal(a.HTML)
	escapedName, _ := json.Marshal(a.Name)
	escapedDesc, _ := json.Marshal(a.Description)
	escapedTags, _ := json.Marshal(a.Tags)
	escapedIcon, _ := json.Marshal(a.Icon)
	escapedSlug, _ := json.Marshal(a.Slug)
	escapedSDK, _ := json.Marshal(NativeSDK(a.Slug))

	savedAt := "Last saved " + a.UpdatedAt.Format("2 Jan 2006 15:04")
	versionLink := ""
	if len(a.Versions) > 0 {
		v := a.Versions[len(a.Versions)-1]
		versionLink = fmt.Sprintf(`<a href="/apps/%s/versions" style="color:#999;">v%d · %d versions</a>`,
			htmlpkg.EscapeString(a.Slug), v.Number, len(a.Versions))
	}

	return fmt.Sprintf(`
<style>
.builder { display: flex; flex-direction: column; gap: 12px; }
.prompt-bar { display: flex; gap: 8px; }
.prompt-bar input { flex: 1; padding: 10px 14px; border: 1px solid #e0e0e0; border-radius: 6px; font-size: 15px; font-family: inherit; }
.prompt-bar button { padding: 10px 24px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; font-size: 15px; white-space: nowrap; }
.prompt-bar button:disabled { background: #ccc; cursor: not-allowed; }
.preview-area { display: flex; flex-direction: column; min-height: 60vh; }
.preview-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.preview-header h3 { font-size: 14px; font-weight: 600; margin: 0; }
.preview-frame { flex: 1; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; min-height: 50vh; }
.code-toggle { padding: 4px 12px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; color: #333; cursor: pointer; font-size: 12px; font-family: inherit; }
.code-toggle:hover { background: #f5f5f5; color: #111; }
.code-section { display: none; margin-top: 12px; }
.code-section.visible { display: block; }
.code-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.code-header h3 { font-size: 14px; font-weight: 600; margin: 0; }
.code-header .actions { display: flex; gap: 6px; }
.code-header .actions button { padding: 4px 12px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; color: #333; cursor: pointer; font-size: 12px; font-family: inherit; }
.code-header .actions button:hover { background: #f5f5f5; color: #111; }
.code-editor { width: 100%%; min-height: 300px; padding: 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace; font-size: 13px; line-height: 1.5; resize: vertical; tab-size: 2; background: #fafafa; }
.save-bar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.save-bar input { padding: 8px 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; font-size: 14px; color: #333; box-sizing: border-box; }
.save-bar input.name { flex: 1; min-width: 150px; }
.save-bar button { padding: 8px 20px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; white-space: nowrap; }
.status-msg { font-size: 13px; color: #999; margin-left: 8px; }
@media (max-width: 768px) {
  .save-bar { flex-direction: column; align-items: stretch; }
  .save-bar input.name { width: 100%%; min-width: auto; }
  .save-bar input { width: 100%%; }
  .prompt-bar { flex-direction: column; }
  .prompt-bar input, .prompt-bar button { width: 100%%; box-sizing: border-box; }
}
</style>

<div class="builder">
  <p class="card-desc">Edit your app — modify with AI or update the code directly.</p>

  <div class="prompt-bar">
    <input type="text" id="prompt" placeholder="Describe changes... e.g. add a dark mode toggle" onkeydown="if(event.key==='Enter')generate()">
    <button id="genBtn" onclick="generate()">Modify</button>
  </div>

  <div style="display:flex;gap:12px;flex-wrap:wrap;">
    <div style="flex:1;min-width:300px;">
      <div class="code-header" style="margin-bottom:6px;">
        <h3>Code</h3>
        <div class="actions">
          <button onclick="copyCode()">Copy</button>
        </div>
      </div>
      <textarea class="code-editor" id="code" spellcheck="false" style="min-height:50vh;"></textarea>
    </div>
    <div style="flex:1;min-width:300px;">
      <div class="preview-header" style="margin-bottom:6px;">
        <h3>Preview</h3>
        <button class="code-toggle" onclick="updatePreview()">Refresh</button>
      </div>
      <iframe id="preview" class="preview-frame" allow="geolocation" style="min-height:50vh;"></iframe>
      <div id="errorBar" style="display:none;padding:8px 12px;background:#fff0f0;border:1px solid #fcc;border-radius:6px;margin-top:6px;font-size:13px;color:#c00;cursor:pointer" onclick="fixErrors()" title="Click to auto-fix"></div>
    </div>
  </div>

  <div class="save-bar">
    <input class="name" type="text" id="appName" placeholder="App name">
    <input type="text" id="appDesc" placeholder="Description" style="flex:1;min-width:120px;">
    <input type="text" id="appTags" placeholder="Tags (optional)" style="width:140px;">
    <button onclick="saveApp()">Save</button>
    <span class="status-msg" id="statusMsg"></span>
  </div>
  <div style="display:flex;justify-content:space-between;align-items:center;font-size:13px;color:#999;">
    <span id="savedAt">%s</span>
    <span>%s</span>
  </div>
</div>

<script>
var codeEl = document.getElementById('code');
var preview = document.getElementById('preview');
var appIcon = %s;
var editSlug = %s;
var previewSDK = %s;
var lastErrors = [];

// Pre-populate fields
codeEl.value = %s;
document.getElementById('appName').value = %s;
document.getElementById('appDesc').value = %s;
document.getElementById('appTags').value = %s;
showPreview();

codeEl.addEventListener('keydown', function(e) {
  if (e.key === 'Tab') {
    e.preventDefault();
    var start = this.selectionStart, end = this.selectionEnd;
    this.value = this.value.substring(0, start) + '  ' + this.value.substring(end);
    this.selectionStart = this.selectionEnd = start + 2;
  }
});

function showPreview() {
  var html = codeEl.value;
  if (!html.trim()) return;
  preview.style.display = '';
  document.getElementById('errorBar').style.display = 'none';
  lastErrors = [];
  var pdoc = preview.contentDocument || preview.contentWindow.document;
  pdoc.open(); pdoc.write(previewSDK + html); pdoc.close();
  setTimeout(function() {
    try {
      var iwin = preview.contentWindow;
      if (iwin && iwin.mu && iwin.mu.errors && iwin.mu.errors.length > 0) {
        lastErrors = iwin.mu.errors;
        var msgs = lastErrors.map(function(e){ return e.message; }).join('; ');
        var bar = document.getElementById('errorBar');
        bar.textContent = msgs.slice(0, 200) + ' — click to fix';
        bar.style.display = 'block';
      }
    } catch(e) {}
  }, 2500);
}

function updatePreview() { showPreview(); }

function fixErrors() {
  if (lastErrors.length === 0) return;
  var msgs = lastErrors.map(function(e){ return e.message; }).join('; ');
  document.getElementById('prompt').value = 'Fix these errors: ' + msgs;
  generate();
}

function toggleCode() {
  var section = document.getElementById('codeSection');
  var btn = document.getElementById('codeToggle');
  if (section.classList.contains('visible')) {
    section.classList.remove('visible');
    btn.textContent = 'Show Code';
  } else {
    section.classList.add('visible');
    btn.textContent = 'Hide Code';
  }
}

function generate() {
  var promptEl = document.getElementById('prompt');
  var p = promptEl.value.trim();
  if (!p) return;
  var btn = document.getElementById('genBtn');
  btn.disabled = true;
  btn.textContent = 'Generating...';
  document.getElementById('statusMsg').textContent = '';

  fetch('/apps/build/generate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt: p, code: codeEl.value })
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.error) { document.getElementById('statusMsg').textContent = data.error; return; }
    codeEl.value = data.html;
    showPreview();
    if (data.icon) { appIcon = data.icon; }
    document.getElementById('statusMsg').textContent = 'Code updated — review changes and click Save when ready.';
  })
  .catch(function(e) { document.getElementById('statusMsg').textContent = 'Error: ' + e.message; })
  .finally(function() { btn.disabled = false; btn.textContent = 'Modify'; });
}

function saveApp() {
  var name = document.getElementById('appName').value.trim();
  var desc = document.getElementById('appDesc').value.trim();
  var tags = (document.getElementById('appTags').value || '').trim();
  var html = codeEl.value.trim();
  if (!name) { document.getElementById('statusMsg').textContent = 'App name is required'; return; }
  if (!html) { document.getElementById('statusMsg').textContent = 'No code to save'; return; }

  document.getElementById('statusMsg').textContent = 'Saving...';
  fetch('/apps/' + editSlug, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, icon: appIcon, description: desc, tags: tags, html: html })
  })
  .then(function(r) {
    if (!r.ok) {
      return r.text().then(function(t) {
        try { var j = JSON.parse(t); throw new Error(j.error || 'Save failed'); }
        catch(e) { if (e.message) throw e; throw new Error('Save failed (status ' + r.status + ')'); }
      });
    }
    return r.json();
  })
  .then(function(data) {
    if (data.error) { document.getElementById('statusMsg').textContent = data.error; return; }
    var now = new Date();
    var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    var ts = now.getDate() + ' ' + months[now.getMonth()] + ' ' + now.getFullYear() + ' ' + String(now.getHours()).padStart(2,'0') + ':' + String(now.getMinutes()).padStart(2,'0');
    document.getElementById('savedAt').textContent = 'Last saved ' + ts;
    document.getElementById('statusMsg').textContent = 'Saved!';
    setTimeout(function() { document.getElementById('statusMsg').textContent = ''; }, 3000);
  })
  .catch(function(e) { document.getElementById('statusMsg').textContent = e.message || 'Save failed'; });
}

function copyCode() {
  navigator.clipboard.writeText(codeEl.value).then(function() {
    document.getElementById('statusMsg').textContent = 'Copied!';
    setTimeout(function() { document.getElementById('statusMsg').textContent = ''; }, 2000);
  });
}
</script>`, savedAt, versionLink, escapedIcon, escapedSlug, escapedSDK, escapedCode, escapedName, escapedDesc, escapedTags)
}
