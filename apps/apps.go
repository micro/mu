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
	Retries     int       `json:"retries"`     // Number of generation retries
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
	
	// Resume any stuck generations from previous run
	resumeStuckGenerations()
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

const maxGenerationRetries = 3

// resumeStuckGenerations finds apps stuck in "generating" status and retries them
func resumeStuckGenerations() {
	mutex.RLock()
	var stuckApps []*App
	for _, a := range apps {
		if a.Status == "generating" {
			stuckApps = append(stuckApps, a)
		}
	}
	mutex.RUnlock()

	for _, a := range stuckApps {
		if a.Retries >= maxGenerationRetries {
			app.Log("apps", "App %s (%s) exceeded max retries, marking as error", a.ID, a.Name)
			mutex.Lock()
			a.Status = "error"
			a.Error = "Generation failed after multiple retries"
			a.UpdatedAt = time.Now()
			saveApps()
			mutex.Unlock()
			continue
		}

		app.Log("apps", "Resuming generation for app %s (%s), retry %d", a.ID, a.Name, a.Retries+1)
		
		// Resume generation in background
		go func(app_ *App) {
			code, err := generateAppCode(app_.Description)
			
			mutex.Lock()
			defer mutex.Unlock()
			
			app_.Retries++
			if err != nil {
				if app_.Retries >= maxGenerationRetries {
					app_.Status = "error"
					app_.Error = err.Error()
				}
				// else keep status as "generating" for next restart to retry
			} else {
				app_.Code = code
				app_.Status = "ready"
				app_.Error = ""
			}
			app_.UpdatedAt = time.Now()
			saveApps()
		}(a)
	}
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
	case strings.HasSuffix(path, "/embed"):
		// Embed app as widget (minimal iframe-friendly version)
		id := strings.TrimSuffix(path, "/embed")
		handleEmbed(w, r, id)
	case strings.HasSuffix(path, "/widget"):
		// Add/remove app from home widgets
		id := strings.TrimSuffix(path, "/widget")
		handleWidget(w, r, sess, id)
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
	r.ParseForm()
	searchQuery := strings.TrimSpace(r.FormValue("q"))
	
	var userApps []*App
	var userID string
	if sess != nil {
		userID = sess.Account
		userApps = GetUserApps(userID)
	}

	publicApps := GetPublicApps()

	var content strings.Builder

	// Search bar and create button
	content.WriteString(`<div class="apps-header">`)
	if sess != nil {
		content.WriteString(`<a href="/apps/new" class="new-app-btn">+ New App</a>`)
	}
	content.WriteString(`
		<form class="apps-search" action="/apps" method="GET">
			<input type="text" name="q" placeholder="Search apps..." value="` + html.EscapeString(searchQuery) + `">
			<button type="submit">Search</button>
		</form>
	</div>`)

	// Featured apps section (always show at top)
	featuredIDs := []string{"1768341615408024989", "1768342273851959552", "1768342520623825814"} // todo, timer, expenses
	var featuredApps []*App
	for _, id := range featuredIDs {
		if a := GetApp(id); a != nil {
			featuredApps = append(featuredApps, a)
		}
	}
	
	if len(featuredApps) > 0 && searchQuery == "" {
		content.WriteString(`<div class="featured-section">`)
		content.WriteString(`<h3>Featured Apps</h3>`)
		content.WriteString(`<div class="featured-grid">`)
		for _, a := range featuredApps {
			content.WriteString(renderFeaturedCard(a))
		}
		content.WriteString(`</div></div>`)
	}

	// Filter apps by search query
	filterApps := func(apps []*App, query string) []*App {
		if query == "" {
			return apps
		}
		query = strings.ToLower(query)
		var filtered []*App
		for _, a := range apps {
			if strings.Contains(strings.ToLower(a.Name), query) ||
				strings.Contains(strings.ToLower(a.Description), query) {
				filtered = append(filtered, a)
			}
		}
		return filtered
	}

	// User's apps
	filteredUserApps := filterApps(userApps, searchQuery)
	if len(filteredUserApps) > 0 {
		content.WriteString(`<h3>My Apps</h3>`)
		content.WriteString(`<div class="apps-grid">`)
		for _, a := range filteredUserApps {
			content.WriteString(renderAppCard(a, true))
		}
		content.WriteString(`</div>`)
	} else if sess != nil && searchQuery == "" {
		content.WriteString(`<p class="info">You haven't created any apps yet.</p>`)
	}

	// Public apps (exclude user's own and featured)
	var otherPublic []*App
	featuredSet := make(map[string]bool)
	for _, id := range featuredIDs {
		featuredSet[id] = true
	}
	for _, a := range publicApps {
		if a.AuthorID != userID && !featuredSet[a.ID] {
			otherPublic = append(otherPublic, a)
		}
	}
	
	filteredPublic := filterApps(otherPublic, searchQuery)
	if len(filteredPublic) > 0 {
		content.WriteString(`<h3 style="margin-top: 30px;">Community Apps</h3>`)
		content.WriteString(`<div class="apps-grid">`)
		for _, a := range filteredPublic {
			content.WriteString(renderAppCard(a, false))
		}
		content.WriteString(`</div>`)
	}

	if searchQuery != "" && len(filteredUserApps) == 0 && len(filteredPublic) == 0 {
		content.WriteString(`<p class="info">No apps found matching "` + html.EscapeString(searchQuery) + `"</p>`)
	}

	if len(userApps) == 0 && len(otherPublic) == 0 && sess == nil && searchQuery == "" {
		content.WriteString(`<p>No apps yet. <a href="/login">Login</a> to create one.</p>`)
	}

	// Add CSS for grid
	style := `
<style>
.apps-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 25px;
	flex-wrap: wrap;
	gap: 15px;
}
.new-app-btn {
	padding: 10px 20px;
	background: var(--accent-color, #0d7377);
	color: white;
	text-decoration: none;
	border-radius: var(--border-radius, 6px);
	font-weight: 500;
}
.new-app-btn:hover {
	opacity: 0.9;
}
.apps-search {
	display: flex;
	gap: 10px;
}
.apps-search input {
	padding: 10px 15px;
	border: 1px solid var(--card-border, #e8e8e8);
	border-radius: var(--border-radius, 6px);
	font-size: 14px;
	min-width: 200px;
}
.apps-search button {
	padding: 10px 20px;
	background: #333;
	color: white;
	border: none;
	border-radius: var(--border-radius, 6px);
	cursor: pointer;
}
.featured-section {
	margin-bottom: 30px;
	padding-bottom: 20px;
	border-bottom: 1px solid var(--divider, #f0f0f0);
}
.featured-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
	gap: 15px;
	margin-top: 15px;
}
.featured-card {
	border: 2px solid var(--accent-color, #0d7377);
	border-radius: var(--border-radius, 6px);
	padding: 20px;
	text-align: center;
	background: var(--card-background, #fff);
	transition: transform 0.15s ease;
}
.featured-card:hover {
	transform: translateY(-2px);
}
.featured-card h4 {
	margin: 0 0 8px 0;
}
.featured-card h4 a {
	text-decoration: none;
	color: var(--accent-color, #0d7377);
}
.featured-card p {
	margin: 0;
	font-size: 13px;
	color: var(--text-secondary, #555);
}
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

func renderFeaturedCard(a *App) string {
	var b strings.Builder
	b.WriteString(`<div class="featured-card">`)
	b.WriteString(fmt.Sprintf(`<h4><a href="/apps/%s">%s</a></h4>`, a.ID, html.EscapeString(a.Name)))
	if a.Description != "" {
		desc := a.Description
		if idx := strings.Index(desc, "\n"); idx > 0 {
			desc = desc[:idx]
		}
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		b.WriteString(fmt.Sprintf(`<p>%s</p>`, html.EscapeString(desc)))
	}
	b.WriteString(`</div>`)
	return b.String()
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
- mu.fetch(url) - fetch any URL (server-side proxy, bypasses CORS) - returns {ok, status, text(), json()}
- mu.user.id - current user's ID (null if not logged in)
- mu.user.name - current user's name
- mu.user.loggedIn - boolean
- mu.app.id - this app's ID
- mu.app.name - this app's name
- mu.theme.get(name) - get CSS variable value (e.g., mu.theme.get('accent-color'))

IMPORTANT: For fetching external URLs, ALWAYS use mu.fetch() instead of fetch() to avoid CORS issues.

Theme CSS variables are automatically available (use var(--mu-*)):
--mu-text-primary, --mu-text-secondary, --mu-text-muted
--mu-accent-color, --mu-accent-blue
--mu-card-background, --mu-card-border, --mu-hover-background
--mu-spacing-xs/sm/md/lg/xl, --mu-border-radius
--mu-shadow-sm, --mu-shadow-md, --mu-transition-fast
--mu-font-family

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

	// Clean up response - extract just the HTML portion
	response = cleanLLMResponse(response)

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

		case "save_code":
			// Save code from textarea (manual editing)
			code := r.FormValue("code")
			if code != "" {
				a.Code = code
				a.UpdatedAt = time.Now()
				if err := UpdateApp(id, a.Name, a.Description, a.Code, a.Public, sess.Account); err != nil {
					renderDevelopForm(w, a, "Save failed: "+err.Error())
					return
				}
			}
			// Redirect back to develop page to refresh preview
			http.Redirect(w, r, "/apps/"+id+"/develop", 303)
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
- mu.fetch(url) - fetch any URL (server-side proxy, bypasses CORS) - returns {ok, status, text(), json()}
- mu.user.id - current user's ID (null if not logged in)
- mu.user.name - current user's name
- mu.user.loggedIn - boolean
- mu.app.id - this app's ID
- mu.app.name - this app's name
- mu.theme.get(name) - get CSS variable value

IMPORTANT: For fetching external URLs, ALWAYS use mu.fetch() instead of fetch() to avoid CORS issues.

Theme CSS variables available: --mu-text-primary, --mu-accent-color, --mu-spacing-*, --mu-border-radius, etc.

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

	// Clean up response - extract just the HTML portion
	response = cleanLLMResponse(response)

	return response, nil
}

// cleanLLMResponse extracts clean HTML from LLM response, removing any markdown
// code fences, explanatory comments, or other text before/after the HTML
func cleanLLMResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove markdown code fences if present
	response = strings.TrimPrefix(response, "```html")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	lower := strings.ToLower(response)

	// Find where HTML starts (<!DOCTYPE or <html)
	start := strings.Index(lower, "<!doctype")
	if start == -1 {
		start = strings.Index(lower, "<html")
	}
	if start > 0 {
		response = response[start:]
		lower = strings.ToLower(response)
	}

	// Find where HTML ends (</html>) and truncate anything after
	end := strings.LastIndex(lower, "</html>")
	if end > 0 {
		response = response[:end+7] // +7 for len("</html>")
	}

	return strings.TrimSpace(response)
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
      <div style="margin-top: 10px;">
        <button type="submit" name="action" value="save_code" %s>Save Code</button>
        <span style="margin-left: 10px; font-size: 13px; color: #666;">Save changes and refresh preview</span>
      </div>
    </details>
  </form>
</div>

<script>
// Auto-save on Ctrl+S / Cmd+S
document.getElementById('code-editor').addEventListener('keydown', function(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault();
    const form = this.closest('form');
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'action';
    input.value = 'save_code';
    form.appendChild(input);
    form.submit();
  }
});
</script>
%s
`, previewURL, messageHTML, disabledAttr, disabledAttr, disabledAttr, a.ID, historyHTML, html.EscapeString(a.Name), disabledAttr, publicChecked, disabledAttr, disabledAttr, html.EscapeString(a.Code), disabledAttr, pollingScript)

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

	// Widget button for logged-in users (only for public apps or own apps)
	var widgetButton string
	if sess != nil && (a.Public || isOwner) {
		if IsWidgetForUser(a.ID, sess.Account) {
			widgetButton = fmt.Sprintf(`<a href="/apps/%s/widget?action=remove" class="widget-btn remove">✓ On Home</a>`, a.ID)
		} else {
			widgetButton = fmt.Sprintf(`<a href="/apps/%s/widget?action=add" class="widget-btn add">+ Add to Home</a>`, a.ID)
		}
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
.widget-btn {
	display: inline-block;
	padding: 6px 12px;
	border-radius: 4px;
	text-decoration: none;
	font-size: 13px;
	margin-left: 15px;
}
.widget-btn.add {
	background: var(--accent-color, #0d7377);
	color: white;
}
.widget-btn.remove {
	background: #eee;
	color: #333;
}
</style>
%s
<p class="info">by %s · %s · Updated %s %s</p>
<p>%s</p>
<iframe class="app-frame" sandbox="allow-scripts allow-same-origin" src="/apps/%s/preview"></iframe>
<p style="margin-top: 20px;"><a href="/apps">← Back to Apps</a></p>
`, actions, html.EscapeString(a.Author), visibility, a.UpdatedAt.Format("Jan 2, 2006"), widgetButton, html.EscapeString(description), a.ID)

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

<h3>Fetch (mu.fetch)</h3>
<p>Server-side proxy for fetching external URLs. Bypasses CORS restrictions.</p>
<pre>
// Fetch any URL (no CORS issues!)
const response = await mu.fetch('https://api.example.com/data');
if (response.ok) {
  const text = await response.text();
  const json = await response.json();
}
</pre>
<p><strong>Always use mu.fetch() instead of fetch() for external URLs.</strong></p>

<h3>Theme (mu.theme)</h3>
<p>CSS variables are automatically injected. Use them for consistent styling.</p>
<pre>
/* Available CSS variables */
var(--mu-text-primary)      /* #1a1a1a */
var(--mu-text-secondary)    /* #555 */
var(--mu-text-muted)        /* #888 */
var(--mu-accent-color)      /* #0d7377 */
var(--mu-accent-blue)       /* #007bff */
var(--mu-card-background)   /* #ffffff */
var(--mu-card-border)       /* #e8e8e8 */
var(--mu-hover-background)  /* #fafafa */
var(--mu-spacing-xs/sm/md/lg/xl)
var(--mu-border-radius)     /* 6px */
var(--mu-shadow-sm)
var(--mu-shadow-md)
var(--mu-font-family)

/* Get value in JS */
const color = mu.theme.get('accent-color');
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

<h3>Example: Web Browser App</h3>
<pre>
// Fetch a webpage (mu.fetch bypasses CORS)
const url = document.getElementById('url').value;
const response = await mu.fetch(url);
if (response.ok) {
  const html = await response.text();
  document.getElementById('content').textContent = html;
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

// handleEmbed serves app as embeddable widget for home cards
func handleEmbed(w http.ResponseWriter, r *http.Request, id string) {
	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	// Only public apps can be embedded
	if !a.Public {
		http.Error(w, "App is not public", 403)
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

	// Allow embedding in iframes
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
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

// handleWidget adds or removes an app from user's home widgets
func handleWidget(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	if sess == nil {
		http.Redirect(w, r, "/login", 302)
		return
	}

	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	// App must be public or owned by user
	if !a.Public && a.AuthorID != sess.Account {
		http.Error(w, "App is not public", 403)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", 500)
		return
	}

	action := r.URL.Query().Get("action")
	
	// Check if already in widgets
	hasWidget := false
	var newWidgets []string
	for _, wid := range acc.Widgets {
		if wid == id {
			hasWidget = true
			if action == "remove" {
				continue // Skip this widget (remove it)
			}
		}
		newWidgets = append(newWidgets, wid)
	}

	if action == "add" && !hasWidget {
		// Limit to 5 widgets max
		if len(newWidgets) >= 5 {
			http.Error(w, "Maximum 5 widgets allowed", 400)
			return
		}
		newWidgets = append(newWidgets, id)
	}

	acc.Widgets = newWidgets
	auth.UpdateAccount(acc)

	// Redirect back to the app page
	http.Redirect(w, r, "/apps/"+id, 302)
}

// IsWidgetForUser checks if an app is in user's widgets
func IsWidgetForUser(appID, userID string) bool {
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return false
	}
	for _, wid := range acc.Widgets {
		if wid == appID {
			return true
		}
	}
	return false
}

// GetUserWidgets returns user's widget app IDs
func GetUserWidgets(userID string) []string {
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return nil
	}
	return acc.Widgets
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
	b.WriteString(`</div>`)
	return b.String()
}
