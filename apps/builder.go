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

// builderSystemPrompt instructs the AI to generate app HTML.
const builderSystemPrompt = `You are an app builder for the Mu platform. You generate complete, self-contained HTML apps.

Rules:
- Output ONLY valid JSON with this exact structure: {"name":"Short App Name","icon":"<svg>...</svg>","html":"<complete HTML document>"}
- The name should be short and descriptive (2-4 words, max 50 chars). E.g. "Pomodoro Timer", "Unit Converter", "Habit Tracker"
- The icon must be an SVG icon matching this exact style:
  - viewBox="0 0 32 32" width="32" height="32"
  - xmlns="http://www.w3.org/2000/svg"
  - Stroke-based outlines only: stroke="#555", fill="none"
  - stroke-width between 1.2 and 2.5, stroke-linecap="round" where appropriate
  - Simple geometric shapes (circles, rects, lines, paths) — no text, no gradients, no filters
  - Represent the app's purpose with a clear, minimal symbol
  - Examples: a clock face for timer, grid of squares for calculator, checkmark for tracker
- The html must be a complete HTML document: <!DOCTYPE html><html><head>...</head><body>...</body></html>
- All CSS must be inline in a <style> tag in <head>
- All JavaScript must be inline in a <script> tag before </body>
- Use the font: font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif
- Style guidelines: clean, minimal design. Use subtle borders (#e0e0e0), 6px border-radius, 16-24px padding, #333 text, #fff background
- Button style: padding 8-10px 20-24px, border-radius 6px, primary buttons use background #000 color #fff
- Keep it simple and functional — no external dependencies, no CDN links, no images
- The app runs in a sandboxed iframe — no access to parent page
- If the app needs AI features, include <script src="/apps/sdk.js"></script> and use mu.ai(prompt)
- If the app needs persistent storage, use mu.store.set(key, value) and mu.store.get(key)
- Maximum 256KB HTML
- Make it responsive and mobile-friendly
- Use semantic HTML and accessible patterns

When the user asks to modify an existing app, return the complete updated JSON (not a diff). Keep the same name and icon unless the user asks to change them.`

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
		System:   builderSystemPrompt,
		Rag:      rag,
		Question: question,
		Priority: ai.PriorityHigh,
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
		System:   builderSystemPrompt,
		Question: prompt,
		Priority: ai.PriorityHigh,
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
	apps[slug] = a
	mutex.Unlock()
	save()

	app.Log("apps", "Agent built app %q for %s", name, authorID)
	event.Publish(event.Event{Type: "apps_updated"})
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
    <iframe id="preview" class="preview-frame" sandbox="allow-scripts" style="display:none;"></iframe>
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

function showPreview() {
  var html = codeEl.value;
  if (!html.trim()) return;
  emptyState.style.display = 'none';
  preview.style.display = '';
  preview.srcdoc = html;
}

function updatePreview() { showPreview(); }

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
</script>`, templateButtons.String(), string(escapedCode))
}
