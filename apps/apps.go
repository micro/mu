package apps

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/chat"
	"mu/data"
)

// App represents a user-created micro app
type App struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"` // Brief description of what the app does
	Code        string    `json:"code"`        // HTML/CSS/JS code
	Author      string    `json:"author"`      // Display name
	AuthorID    string    `json:"author_id"`   // User ID
	Public      bool      `json:"public"`      // Whether app is publicly listed
	Status      string    `json:"status"`      // "generating", "ready", "error"
	Error       string    `json:"error"`       // Error message if generation failed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var (
	mutex sync.RWMutex
	apps  []*App
)

// Load initializes the apps package
func Load() {
	loadApps()
	app.Log("apps", "Loaded %d apps", len(apps))
}

func loadApps() {
	mutex.Lock()
	defer mutex.Unlock()

	b, err := data.LoadFile("apps.json")
	if err != nil {
		apps = []*App{}
		return
	}

	var loaded []*App
	if err := json.Unmarshal(b, &loaded); err != nil {
		app.Log("apps", "Error loading apps: %v", err)
		apps = []*App{}
		return
	}
	apps = loaded
}

func saveApps() error {
	b, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	return data.SaveFile("apps.json", string(b))
}

// GetApp returns an app by ID
func GetApp(id string) *App {
	mutex.RLock()
	defer mutex.RUnlock()

	for _, a := range apps {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// GetUserApps returns all apps by a user
func GetUserApps(userID string) []*App {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*App
	for _, a := range apps {
		if a.AuthorID == userID {
			result = append(result, a)
		}
	}

	// Sort by most recent first
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	return result
}

// GetPublicApps returns all public apps
func GetPublicApps() []*App {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*App
	for _, a := range apps {
		if a.Public {
			result = append(result, a)
		}
	}

	// Sort by most recent first
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	return result
}

// CreateApp creates a new app
func CreateApp(name, description, code, author, authorID string, public bool) (*App, error) {
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()
	status := "ready"
	if code == "" {
		status = "generating"
	}

	a := &App{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		Name:        name,
		Description: description,
		Code:        code,
		Author:      author,
		AuthorID:    authorID,
		Public:      public,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	apps = append(apps, a)

	if err := saveApps(); err != nil {
		return nil, err
	}

	return a, nil
}

// CreateAppAsync creates an app and generates code in the background
func CreateAppAsync(name, prompt, author, authorID string) (*App, error) {
	// Content moderation
	if isBlockedContent(prompt) || isBlockedContent(name) {
		return nil, fmt.Errorf("this request contains content that goes against our values")
	}

	// Create app with empty code, status "generating"
	a, err := CreateApp(name, prompt, "", author, authorID, false)
	if err != nil {
		return nil, err
	}

	// Generate code in background
	go func() {
		code, err := generateAppCode(prompt)
		
		mutex.Lock()
		defer mutex.Unlock()
		
		if err != nil {
			a.Status = "error"
			a.Error = err.Error()
		} else {
			a.Code = code
			a.Status = "ready"
		}
		a.UpdatedAt = time.Now()
		saveApps()
	}()

	return a, nil
}

// UpdateApp updates an existing app
func UpdateApp(id, name, description, code string, public bool, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for _, a := range apps {
		if a.ID == id {
			// Only author can update
			if a.AuthorID != userID {
				return fmt.Errorf("not authorized")
			}
			a.Name = name
			a.Description = description
			a.Code = code
			a.Public = public
			a.UpdatedAt = time.Now()
			return saveApps()
		}
	}
	return fmt.Errorf("app not found")
}

// DeleteApp deletes an app
func DeleteApp(id, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for i, a := range apps {
		if a.ID == id {
			// Only author can delete
			if a.AuthorID != userID {
				return fmt.Errorf("not authorized")
			}
			apps = append(apps[:i], apps[i+1:]...)
			return saveApps()
		}
	}
	return fmt.Errorf("app not found")
}

// Handler handles /apps routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/apps")
	path = strings.TrimPrefix(path, "/")

	// Get session (optional for viewing public apps)
	sess, _ := auth.GetSession(r)

	// Reserved single-word app URLs
	reservedApps := map[string]string{
		"todo":     "1768341615408024989",
		"timer":    "1768342273851959552",
		"expenses": "1768342520623825814",
	}

	switch {
	case path == "" || path == "/":
		// List apps
		handleList(w, r, sess)
	case path == "new":
		// Create new app form
		handleNew(w, r, sess)
	case path == "docs":
		// SDK documentation
		handleSDKDocs(w, r)
	case reservedApps[strings.ToLower(path)] != "":
		// Reserved app name -> redirect to the featured app
		http.Redirect(w, r, "/apps/"+reservedApps[strings.ToLower(path)], 302)
	case path == "loading":
		// Loading page for iframe
		handleLoading(w, r)
	case strings.HasSuffix(path, "/edit"):
		// Edit app (legacy - redirects to develop)
		id := strings.TrimSuffix(path, "/edit")
		http.Redirect(w, r, "/apps/"+id+"/develop", 302)
	case strings.HasSuffix(path, "/develop"):
		// Iterative development mode
		id := strings.TrimSuffix(path, "/develop")
		handleDevelop(w, r, sess, id)
	case strings.HasSuffix(path, "/preview"):
		// Preview app (renders just the app code)
		id := strings.TrimSuffix(path, "/preview")
		handlePreview(w, r, id)
	case strings.HasSuffix(path, "/delete"):
		// Delete app
		id := strings.TrimSuffix(path, "/delete")
		handleDelete(w, r, sess, id)
	case strings.HasSuffix(path, "/status"):
		// Status API for polling
		id := strings.TrimSuffix(path, "/status")
		handleStatus(w, r, id)
	default:
		// View app
		handleView(w, r, sess, path)
	}
}

func handleList(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	var userApps []*App
	var userID string
	if sess != nil {
		userID = sess.Account
		userApps = GetUserApps(userID)
	}

	publicApps := GetPublicApps()

	var content strings.Builder

	// Create button (if logged in)
	if sess != nil {
		content.WriteString(`<p style="margin-bottom: 20px;"><a href="/apps/new">+ New App</a></p>`)
	}

	// User's apps
	if len(userApps) > 0 {
		content.WriteString(`<h3>My Apps</h3>`)
		content.WriteString(`<div class="apps-grid">`)
		for _, a := range userApps {
			content.WriteString(renderAppCard(a, true))
		}
		content.WriteString(`</div>`)
	} else if sess != nil {
		content.WriteString(`<p class="info">You haven't created any apps yet.</p>`)
	}

	// Public apps (exclude user's own)
	var otherPublic []*App
	for _, a := range publicApps {
		if a.AuthorID != userID {
			otherPublic = append(otherPublic, a)
		}
	}

	if len(otherPublic) > 0 {
		content.WriteString(`<h3 style="margin-top: 30px;">Public Apps</h3>`)
		content.WriteString(`<div class="apps-grid">`)
		for _, a := range otherPublic {
			content.WriteString(renderAppCard(a, false))
		}
		content.WriteString(`</div>`)
	}

	if len(userApps) == 0 && len(otherPublic) == 0 && sess == nil {
		content.WriteString(`<p>No apps yet. <a href="/login">Login</a> to create one.</p>`)
	}

	// Add CSS for grid
	style := `
<style>
.apps-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
	gap: 15px;
}
.app-card {
	border: 1px solid var(--card-border, #e8e8e8);
	border-radius: var(--border-radius, 6px);
	padding: var(--item-padding, 16px);
	background: var(--card-background, #fff);
}
.app-card:hover {
	border-color: #999;
}
.app-card h4 {
	margin: 0 0 8px 0;
}
.app-card h4 a {
	text-decoration: none;
}
.app-card p {
	margin: 0 0 10px 0;
	color: var(--text-secondary, #555);
	font-size: 14px;
}
.app-card .meta {
	font-size: 12px;
	color: var(--text-muted, #888);
}
.app-card .actions {
	margin-top: 10px;
}
.app-card .actions a {
	margin-right: 15px;
	font-size: 13px;
}
.app-card .actions a.delete {
	color: #c00;
}
</style>`

	html := style + content.String()
	w.Write([]byte(app.RenderHTML("Apps", "Micro Apps", html)))
}

func renderAppCard(a *App, isOwner bool) string {
	var b strings.Builder
	b.WriteString(`<div class="app-card">`)
	b.WriteString(fmt.Sprintf(`<h4><a href="/apps/%s">%s</a></h4>`, a.ID, html.EscapeString(a.Name)))
	if a.Description != "" {
		// Truncate description to first line or 100 chars
		desc := a.Description
		if idx := strings.Index(desc, "\n"); idx > 0 {
			desc = desc[:idx]
		}
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		b.WriteString(fmt.Sprintf(`<p>%s</p>`, html.EscapeString(desc)))
	}
	b.WriteString(fmt.Sprintf(`<div class="meta">by %s`, html.EscapeString(a.Author)))
	if a.Public {
		b.WriteString(` · Public`)
	} else {
		b.WriteString(` · Private`)
	}
	b.WriteString(`</div>`)
	if isOwner {
		b.WriteString(`<div class="actions">`)
		b.WriteString(fmt.Sprintf(`<a href="/apps/%s/develop">Edit</a>`, a.ID))
		b.WriteString(fmt.Sprintf(`<a href="/apps/%s/delete" class="delete" onclick="return confirm('Delete this app?')">Delete</a>`, a.ID))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func handleNew(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	if sess == nil {
		http.Redirect(w, r, "/login?redirect=/apps/new", 302)
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		name := strings.TrimSpace(r.FormValue("name"))
		promptText := strings.TrimSpace(r.FormValue("prompt"))

		if name == "" {
			renderNewForm(w, "Name is required", name, promptText)
			return
		}

		if promptText == "" {
			renderNewForm(w, "Please describe what you want to build", name, promptText)
			return
		}

		// Create app and start async generation
		a, err := CreateAppAsync(name, promptText, sess.Account, sess.Account)
		if err != nil {
			renderNewForm(w, err.Error(), name, promptText)
			return
		}

		// Immediately redirect to develop page
		http.Redirect(w, r, "/apps/"+a.ID+"/develop", 302)
		return
	}

	renderNewForm(w, "", "", "")
}

// generateAppCode uses AI to generate HTML/CSS/JS from a prompt
func generateAppCode(prompt string) (string, error) {
	systemPrompt := `You are an expert web developer. Generate a complete, self-contained single-page web application based on the user's description.

Rules:
1. Output ONLY valid HTML - no markdown, no code fences, no explanations
2. Include all CSS in a <style> tag in the <head>
3. Include all JavaScript in a <script> tag before </body>
4. Use modern, clean design with good typography
5. Make it mobile-responsive
6. Use system-ui font stack
7. The app must be fully functional and self-contained
8. Do not use any external dependencies or CDNs
9. Start with <!DOCTYPE html> and end with </html>

Mu SDK (automatically available as window.mu):
- mu.db.get(key) - retrieve stored value (async)
- mu.db.set(key, value) - store value persistently (async)
- mu.db.delete(key) - delete a key (async)
- mu.db.list() - list all keys (async)
- mu.user.id - current user's ID (null if not logged in)
- mu.user.name - current user's name
- mu.user.loggedIn - boolean
- mu.app.id - this app's ID
- mu.app.name - this app's name

Use mu.db for any data that should persist across page refreshes. Data is stored per-user.

Generate the complete HTML file now:`

	llmPrompt := &chat.Prompt{
		System:   systemPrompt,
		Question: prompt,
		Priority: chat.PriorityHigh,
	}

	response, err := chat.AskLLM(llmPrompt)
	if err != nil {
		return "", err
	}

	// Clean up response - remove any markdown code fences if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```html")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Ensure it starts with DOCTYPE
	if !strings.HasPrefix(strings.ToLower(response), "<!doctype") {
		// Try to find where HTML starts
		if idx := strings.Index(strings.ToLower(response), "<!doctype"); idx > 0 {
			response = response[idx:]
		}
	}

	return response, nil
}

func renderNewForm(w http.ResponseWriter, errMsg, name, prompt string) {
	errHTML := ""
	if errMsg != "" {
		errHTML = fmt.Sprintf(`<div style="color: red; margin-bottom: 15px;">%s</div>`, html.EscapeString(errMsg))
	}

	formHTML := fmt.Sprintf(`
<style>
.form-group { margin-bottom: 15px; }
.form-group label { display: block; margin-bottom: 5px; font-weight: bold; }
.form-group input[type="text"], .form-group textarea {
	width: 100%%;;
	padding: 12px;
	border: 1px solid #ddd;
	border-radius: 4px;
	box-sizing: border-box;
	font-family: inherit;
	font-size: 16px;
}
.form-group textarea {
	min-height: 120px;
}
.hint {
	font-size: 13px;
	color: #666;
	margin-top: 5px;
}
</style>
%s
<form method="POST" style="max-width: 600px;">
  <div class="form-group">
    <label>Name</label>
    <input type="text" name="name" value="%s" placeholder="Pomodoro Timer" required autofocus>
  </div>
  <div class="form-group">
    <label>What do you want to build?</label>
    <textarea name="prompt" placeholder="A pomodoro timer with 25 minute work sessions and 5 minute breaks, start/pause/reset buttons, and a session counter">%s</textarea>
    <div class="hint">Describe your app in plain English. Be specific about features and layout.</div>
  </div>
  <div>
    <button type="submit">Create App</button>
  </div>
</form>
`, errHTML, html.EscapeString(name), html.EscapeString(prompt))

	w.Write([]byte(app.RenderHTML("New App", "Create New App", formHTML)))
}

func handleEdit(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	if sess == nil {
		http.Redirect(w, r, "/login?redirect=/apps/"+id+"/edit", 302)
		return
	}

	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	if a.AuthorID != sess.Account {
		http.Error(w, "Not authorized", 403)
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		name := strings.TrimSpace(r.FormValue("name"))
		promptText := strings.TrimSpace(r.FormValue("prompt"))
		code := r.FormValue("code")
		public := r.FormValue("public") == "on"
		action := r.FormValue("action")

		if name == "" {
			a.Name = name
			a.Description = promptText
			a.Code = code
			a.Public = public
			renderEditForm(w, a, "Name is required")
			return
		}

		// Regenerate code from prompt
		if action == "generate" {
			if promptText == "" {
				a.Name = name
				a.Description = promptText
				a.Code = code
				a.Public = public
				renderEditForm(w, a, "Please describe what you want to build")
				return
			}

			generated, err := generateAppCode(promptText)
			if err != nil {
				a.Name = name
				a.Description = promptText
				a.Code = code
				a.Public = public
				renderEditForm(w, a, "Generation failed: "+err.Error())
				return
			}

			a.Name = name
			a.Description = promptText
			a.Code = generated
			a.Public = public
			renderEditForm(w, a, "")
			return
		}

		if err := UpdateApp(id, name, promptText, code, public, sess.Account); err != nil {
			a.Name = name
			a.Description = promptText
			a.Code = code
			a.Public = public
			renderEditForm(w, a, err.Error())
			return
		}

		http.Redirect(w, r, "/apps/"+id, 302)
		return
	}

	renderEditForm(w, a, "")
}

func renderEditForm(w http.ResponseWriter, a *App, errMsg string) {
	publicChecked := ""
	if a.Public {
		publicChecked = "checked"
	}

	errHTML := ""
	if errMsg != "" {
		errHTML = fmt.Sprintf(`<div style="color: red; margin-bottom: 15px;">%s</div>`, html.EscapeString(errMsg))
	}

	formHTML := fmt.Sprintf(`
<style>
.form-group { margin-bottom: 15px; }
.form-group label { display: block; margin-bottom: 5px; font-weight: bold; }
.form-group input[type="text"], .form-group textarea {
	width: 100%%;
	padding: 8px;
	border: 1px solid #ddd;
	border-radius: 4px;
	box-sizing: border-box;
	font-family: inherit;
}
.form-group textarea#prompt {
	min-height: 80px;
	font-family: inherit;
}
.form-group textarea#code-editor {
	font-family: monospace;
	font-size: 13px;
	min-height: 300px;
}
.checkbox-group {
	display: flex;
	align-items: center;
	gap: 8px;
}
.button { 
	padding: 10px 20px; 
	background: #333; 
	color: white; 
	border: none; 
	border-radius: 4px; 
	cursor: pointer;
	font-size: 14px;
}
.button:hover { background: #555; }
.button-secondary {
	background: #666;
	margin-left: 10px;
}
.preview-frame {
	width: 100%%;
	height: 400px;
	border: 1px solid #ddd;
	border-radius: 4px;
	margin-top: 20px;
}
.hint {
	font-size: 13px;
	color: #666;
	margin-top: 5px;
}
</style>
%s
<form method="POST" id="app-form">
  <div class="form-group">
    <label>Name</label>
    <input type="text" name="name" value="%s" required>
  </div>
  <div class="form-group">
    <label>Prompt</label>
    <textarea name="prompt" id="prompt" placeholder="Describe what you want to build...">%s</textarea>
    <div class="hint">Edit the prompt and click Regenerate to create new code.</div>
  </div>
  <div class="form-group">
    <label>Code (HTML/CSS/JS)</label>
    <textarea name="code" id="code-editor">%s</textarea>
  </div>
  <div class="form-group checkbox-group">
    <input type="checkbox" name="public" id="public" %s>
    <label for="public" style="font-weight: normal;">Make this app public</label>
  </div>
  <div>
    <button type="submit" name="action" value="save" class="button">Save Changes</button>
    <button type="button" class="button button-secondary" onclick="previewApp()">Preview</button>
    <button type="submit" name="action" value="generate" class="button button-secondary">Regenerate</button>
    <a href="/apps/%s" style="margin-left: 10px;">Cancel</a>
  </div>
</form>
<iframe id="preview" class="preview-frame" sandbox="allow-scripts allow-same-origin"></iframe>
<script>
function previewApp() {
  var code = document.getElementById('code-editor').value;
  var iframe = document.getElementById('preview');
  iframe.srcdoc = code;
}
// Load preview on page load
window.onload = function() { previewApp(); };
</script>
`, errHTML, html.EscapeString(a.Name), html.EscapeString(a.Description), html.EscapeString(a.Code), publicChecked, a.ID)

	w.Write([]byte(app.RenderHTML("Edit App", "Edit: "+a.Name, formHTML)))
}

// handleDevelop provides iterative AI-assisted app development
// Blocked terms for content moderation
var blockedTerms = []string{
	"gambling", "casino", "bet", "betting", "slots", "poker",
	"porn", "pornography", "xxx", "adult content", "nsfw",
	"alcohol", "beer", "wine", "liquor", "drunk",
	"haram", "interest calculator", "riba",
}

// isBlockedContent checks if prompt contains unethical content
func isBlockedContent(text string) bool {
	lower := strings.ToLower(text)
	for _, term := range blockedTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func handleDevelop(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	if sess == nil {
		http.Redirect(w, r, "/login?redirect=/apps/"+id+"/develop", 302)
		return
	}

	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	if a.AuthorID != sess.Account {
		http.Error(w, "Not authorized", 403)
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		action := r.FormValue("action")
		instruction := strings.TrimSpace(r.FormValue("instruction"))
		name := strings.TrimSpace(r.FormValue("name"))
		public := r.FormValue("public") == "on"

		if name != "" {
			a.Name = name
		}
		a.Public = public

		switch action {
		case "modify":
			if instruction == "" {
				renderDevelopForm(w, a, "Please describe what you want to change")
				return
			}
			
			// Content moderation
			if isBlockedContent(instruction) {
				renderDevelopForm(w, a, "This request contains content that goes against our values")
				return
			}

			// Set status to generating and kick off async modification
			mutex.Lock()
			a.Status = "generating"
			a.Error = ""
			saveApps()
			mutex.Unlock()

			// Async modification
			go func() {
				modified, err := modifyAppCode(a.Code, instruction)
				
				mutex.Lock()
				defer mutex.Unlock()
				
				if err != nil {
					a.Status = "error"
					a.Error = err.Error()
				} else {
					a.Code = modified
					a.Status = "ready"
					// Append instruction to description history
					if a.Description != "" {
						a.Description = a.Description + "\n• " + instruction
					} else {
						a.Description = "• " + instruction
					}
				}
				a.UpdatedAt = time.Now()
				saveApps()
			}()

			// Redirect to same page (will show spinner)
			http.Redirect(w, r, "/apps/"+id+"/develop", 303)
			return

		case "save":
			if err := UpdateApp(id, a.Name, a.Description, a.Code, a.Public, sess.Account); err != nil {
				renderDevelopForm(w, a, "Save failed: "+err.Error())
				return
			}
			http.Redirect(w, r, "/apps/"+id, 302)
			return
		}
	}

	renderDevelopForm(w, a, "")
}

// modifyAppCode uses AI to make targeted changes to existing code
func modifyAppCode(currentCode, instruction string) (string, error) {
	systemPrompt := `You are an expert web developer. You will receive existing HTML/CSS/JS code and an instruction for how to modify it.

Rules:
1. Output ONLY the complete modified HTML file - no markdown, no code fences, no explanations
2. Make targeted changes based on the instruction - don't rewrite everything unnecessarily
3. Preserve the existing structure and style unless asked to change it
4. Keep all existing functionality unless asked to change it
5. Start with <!DOCTYPE html> and end with </html>
6. NEVER use placeholder comments like "// ...existing code..." - always include the full actual code
7. Output must be complete, valid, runnable HTML

Mu SDK (automatically available as window.mu):
- mu.db.get(key) - retrieve stored value (async)
- mu.db.set(key, value) - store value persistently (async)
- mu.db.delete(key) - delete a key (async) 
- mu.db.list() - list all keys (async)
- mu.user.id - current user's ID (null if not logged in)
- mu.user.name - current user's name
- mu.user.loggedIn - boolean
- mu.app.id - this app's ID
- mu.app.name - this app's name

Use mu.db for any data that should persist. Data is per-user.

Current code:
` + currentCode + `

Apply this modification and output the complete updated HTML file:`

	llmPrompt := &chat.Prompt{
		System:   systemPrompt,
		Question: instruction,
		Priority: chat.PriorityHigh,
	}

	response, err := chat.AskLLM(llmPrompt)
	if err != nil {
		return "", err
	}

	// Clean up response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```html")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Ensure it starts with DOCTYPE
	if !strings.HasPrefix(strings.ToLower(response), "<!doctype") {
		if idx := strings.Index(strings.ToLower(response), "<!doctype"); idx > 0 {
			response = response[idx:]
		}
	}

	return response, nil
}

func renderDevelopForm(w http.ResponseWriter, a *App, message string) {
	publicChecked := ""
	if a.Public {
		publicChecked = "checked"
	}

	// Check if still generating
	isGenerating := a.Status == "generating"
	hasError := a.Status == "error"

	messageHTML := ""
	if hasError {
		messageHTML = fmt.Sprintf(`<div style="color: #c00; margin-bottom: 15px; padding: 10px; background: #f5f5f5; border-radius: 4px;">Generation failed: %s</div>`, html.EscapeString(a.Error))
	} else if message != "" {
		color := "#c00"
		if strings.HasPrefix(message, "✓") {
			color = "#080"
		}
		messageHTML = fmt.Sprintf(`<div style="color: %s; margin-bottom: 15px; padding: 10px; background: #f5f5f5; border-radius: 4px;">%s</div>`, color, html.EscapeString(message))
	}

	// Parse history from description
	historyHTML := ""
	if a.Description != "" {
		lines := strings.Split(a.Description, "\n")
		historyHTML = `<div class="history"><strong>Changes:</strong><ul>`
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				line = strings.TrimPrefix(line, "• ")
				historyHTML += fmt.Sprintf(`<li>%s</li>`, html.EscapeString(line))
			}
		}
		historyHTML += `</ul></div>`
	}

	// Preview URL - use /preview endpoint to avoid escaping issues
	previewURL := fmt.Sprintf("/apps/%s/preview", a.ID)
	if isGenerating {
		previewURL = "/apps/loading" // Special loading page
	}

	// Polling script for generating state
	pollingScript := ""
	if isGenerating {
		pollingScript = fmt.Sprintf(`
<script>
(function poll() {
	fetch('/apps/%s/status')
		.then(r => r.json())
		.then(data => {
			if (data.status === 'ready' || data.status === 'error') {
				window.location.reload();
			} else {
				setTimeout(poll, 2000);
			}
		})
		.catch(() => setTimeout(poll, 2000));
})();
</script>`, a.ID)
	}

	// Disable form inputs while generating
	disabledAttr := ""
	if isGenerating {
		disabledAttr = "disabled"
	}

	formHTML := fmt.Sprintf(`
<style>
.develop-container {
	display: flex;
	flex-direction: column;
	gap: 20px;
}
.preview-section {
	border: 1px solid #ddd;
	border-radius: 8px;
	overflow: hidden;
	background: #fff;
}
.preview-frame {
	width: 100%%;;
	height: 400px;
	border: none;
	display: block;
}
.instruction-section {
	background: #f9f9f9;
	padding: 20px;
	border-radius: 8px;
}
.instruction-input {
	width: 100%%;;
	padding: 12px;
	border: 1px solid #ddd;
	border-radius: 4px;
	font-size: 15px;
	font-family: inherit;
	margin-bottom: 10px;
	box-sizing: border-box;
}
.instruction-input:disabled {
	background: #eee;
}
.history {
	margin-top: 15px;
	font-size: 13px;
	color: #666;
}
.history ul {
	margin: 5px 0 0 20px;
	padding: 0;
}
.history li {
	margin: 3px 0;
}
.meta-section {
	display: flex;
	gap: 15px;
	align-items: center;
	margin-top: 15px;
	padding-top: 15px;
	border-top: 1px solid #ddd;
	flex-wrap: wrap;
}
.meta-section input[type="text"] {
	padding: 8px;
	border: 1px solid #ddd;
	border-radius: 4px;
	font-size: 14px;
	width: auto;
}
.checkbox-group {
	display: flex;
	align-items: center;
	gap: 5px;
}
.checkbox-group input[type="checkbox"] {
	width: auto;
}
.code-toggle {
	margin-top: 15px;
}
.code-toggle summary {
	cursor: pointer;
	color: #666;
	font-size: 13px;
}
.code-editor {
	width: 100%%;;
	min-height: 300px;
	font-family: monospace;
	font-size: 12px;
	padding: 10px;
	border: 1px solid #ddd;
	border-radius: 4px;
	margin-top: 10px;
	box-sizing: border-box;
}
</style>

<div class="develop-container">
  <div class="preview-section">
    <iframe id="preview" class="preview-frame" sandbox="allow-scripts allow-same-origin" src="%s"></iframe>
  </div>
  
  <form method="POST" class="instruction-section">
    %s
    <input type="text" name="instruction" class="instruction-input" placeholder="Describe what you want to change..." %s autofocus>
    <button type="submit" name="action" value="modify" %s>Apply Change</button>
    <button type="submit" name="action" value="save" style="margin-left: 10px;" %s>Done</button>
    <a href="/apps/%s" style="margin-left: 15px;">Cancel</a>
    
    %s
    
    <div class="meta-section">
      <label>Name: <input type="text" name="name" value="%s" %s></label>
      <div class="checkbox-group">
        <input type="checkbox" name="public" id="public" %s %s>
        <label for="public">Public</label>
      </div>
    </div>
    
    <details class="code-toggle">
      <summary>Show code</summary>
      <textarea class="code-editor" name="code" id="code-editor" %s>%s</textarea>
    </details>
  </form>
</div>

<script>
// Manual code editing requires page refresh to see changes
</script>
%s
`, previewURL, messageHTML, disabledAttr, disabledAttr, disabledAttr, a.ID, historyHTML, html.EscapeString(a.Name), disabledAttr, publicChecked, disabledAttr, disabledAttr, html.EscapeString(a.Code), pollingScript)

	w.Write([]byte(app.RenderHTML("Develop: "+a.Name, "Develop: "+a.Name, formHTML)))
}

func handleView(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	// Check access
	if !a.Public {
		if sess == nil || sess.Account != a.AuthorID {
			http.Error(w, "Not authorized", 403)
			return
		}
	}

	isOwner := sess != nil && sess.Account == a.AuthorID

	var actions string
	if isOwner {
		actions = fmt.Sprintf(`
		<p style="margin-bottom: 20px;">
			<a href="/apps/%s/develop">Edit</a>
			<a href="/apps/%s/delete" style="color: #c00; margin-left: 15px;" onclick="return confirm('Delete this app?')">Delete</a>
		</p>`, a.ID, a.ID)
	}

	visibility := "Private"
	if a.Public {
		visibility = "Public"
	}

	// Get only the first line of description (original prompt, not change history)
	description := a.Description
	if idx := strings.Index(description, "\n"); idx > 0 {
		description = description[:idx]
	}

	// Get user info for SDK injection
	viewHTML := fmt.Sprintf(`
<style>
.app-frame {
	width: 100%%;
	height: 500px;
	border: 1px solid var(--card-border, #e8e8e8);
	border-radius: var(--border-radius, 6px);
	background: white;
}
</style>
%s
<p class="info">by %s · %s · Updated %s</p>
<p>%s</p>
<iframe class="app-frame" sandbox="allow-scripts allow-same-origin" src="/apps/%s/preview"></iframe>
<p style="margin-top: 20px;"><a href="/apps">← Back to Apps</a></p>
`, actions, html.EscapeString(a.Author), visibility, a.UpdatedAt.Format("Jan 2, 2006"), html.EscapeString(description), a.ID)

	w.Write([]byte(app.RenderHTML(a.Name, a.Name, viewHTML)))
}

// handleSDKDocs serves the SDK documentation page
func handleSDKDocs(w http.ResponseWriter, r *http.Request) {
	docs := `
<h2>Mu SDK</h2>
<p>The Mu SDK is automatically available in all apps as <code>window.mu</code>.</p>

<h3>Database (mu.db)</h3>
<p>Per-user persistent storage. 100KB quota per app.</p>
<pre>
// Get a value
const value = await mu.db.get('key');

// Set a value (can be any JSON-serializable data)
await mu.db.set('key', value);

// Delete a key
await mu.db.delete('key');

// List all keys
const keys = await mu.db.list();

// Check quota
const {used, limit} = await mu.db.quota();
</pre>

<h3>User Context (mu.user)</h3>
<pre>
mu.user.id        // User ID (string) or null if not logged in
mu.user.name      // User's display name or null
mu.user.loggedIn  // boolean
</pre>

<h3>App Context (mu.app)</h3>
<pre>
mu.app.id    // This app's unique ID
mu.app.name  // This app's name
</pre>

<h3>Example: Todo App</h3>
<pre>
// Load todos on startup
let todos = [];
const saved = await mu.db.get('todos');
if (saved) todos = JSON.parse(saved);

// Save after changes
function saveTodos() {
  mu.db.set('todos', JSON.stringify(todos));
}
</pre>

<h3>Featured Apps</h3>
<ul>
<li><a href="/apps/todo">Todo</a> - Task management</li>
<li><a href="/apps/timer">Timer</a> - Focus/pomodoro timer</li>
<li><a href="/apps/expenses">Expenses</a> - Expense tracking</li>
</ul>
`
	w.Write([]byte(app.RenderHTML("SDK Documentation", "Mu SDK", docs)))
}

// handleLoading serves a loading spinner page for the preview iframe
func handleLoading(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<style>
body {
	font-family: system-ui, sans-serif;
	display: flex;
	align-items: center;
	justify-content: center;
	height: 100vh;
	margin: 0;
	background: #f5f5f5;
}
.loader {
	text-align: center;
	color: #666;
}
.spinner {
	width: 40px;
	height: 40px;
	border: 3px solid #ddd;
	border-top-color: #333;
	border-radius: 50%;
	animation: spin 1s linear infinite;
	margin: 0 auto 15px;
}
@keyframes spin {
	to { transform: rotate(360deg); }
}
</style>
</head>
<body>
<div class="loader">
	<div class="spinner"></div>
	Applying changes...
</div>
</body>
</html>`))
}

func handlePreview(w http.ResponseWriter, r *http.Request, id string) {
	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	// Get user info for SDK
	var userID, userName string
	if sess, _ := auth.GetSession(r); sess != nil {
		userID = sess.Account
		if acc, err := auth.GetAccount(sess.Account); err == nil {
			userName = acc.Name
		}
	}

	// Inject SDK into the HTML
	html := InjectSDK(a.Code, a.ID, a.Name, userID, userName)

	// Preview returns HTML with SDK injected
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleStatus returns app status as JSON for polling
func handleStatus(w http.ResponseWriter, r *http.Request, id string) {
	a := GetApp(id)
	if a == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"error": "not found"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	status := a.Status
	if status == "" {
		status = "ready"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"error":  a.Error,
		"hasCode": a.Code != "",
	})
}

func handleDelete(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	if sess == nil {
		http.Redirect(w, r, "/login", 302)
		return
	}

	if err := DeleteApp(id, sess.Account); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	http.Redirect(w, r, "/apps", 302)
}

// GetUserAppsPreview returns HTML preview of user's apps for home page
func GetUserAppsPreview(userID string, limit int) string {
	userApps := GetUserApps(userID)
	if len(userApps) == 0 {
		return ""
	}

	if limit > 0 && len(userApps) > limit {
		userApps = userApps[:limit]
	}

	var b strings.Builder
	b.WriteString(`<div class="apps-preview">`)
	for _, a := range userApps {
		b.WriteString(fmt.Sprintf(`<a href="/apps/%s" class="app-preview-item">%s</a>`, a.ID, html.EscapeString(a.Name)))
	}
	b.WriteString(`<a href="/apps" class="app-preview-more">more →</a>`)
	b.WriteString(`</div>`)
	return b.String()
}
