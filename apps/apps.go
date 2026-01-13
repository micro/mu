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
	a := &App{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		Name:        name,
		Description: description,
		Code:        code,
		Author:      author,
		AuthorID:    authorID,
		Public:      public,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	apps = append(apps, a)

	if err := saveApps(); err != nil {
		return nil, err
	}

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

	switch {
	case path == "" || path == "/":
		// List apps
		handleList(w, r, sess)
	case path == "new":
		// Create new app form
		handleNew(w, r, sess)
	case strings.HasSuffix(path, "/edit"):
		// Edit app
		id := strings.TrimSuffix(path, "/edit")
		handleEdit(w, r, sess, id)
	case strings.HasSuffix(path, "/preview"):
		// Preview app (renders just the app code)
		id := strings.TrimSuffix(path, "/preview")
		handlePreview(w, r, id)
	case strings.HasSuffix(path, "/delete"):
		// Delete app
		id := strings.TrimSuffix(path, "/delete")
		handleDelete(w, r, sess, id)
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
		content.WriteString(`<div style="margin-bottom: 20px;"><a href="/apps/new" class="button">+ New App</a></div>`)
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
		content.WriteString(`<p style="color: #666;">You haven't created any apps yet.</p>`)
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
	border: 1px solid #ddd;
	border-radius: 8px;
	padding: 15px;
	background: #fff;
}
.app-card:hover {
	border-color: #999;
}
.app-card h4 {
	margin: 0 0 8px 0;
}
.app-card p {
	margin: 0 0 10px 0;
	color: #666;
	font-size: 14px;
}
.app-card .meta {
	font-size: 12px;
	color: #999;
}
.app-card .actions {
	margin-top: 10px;
}
.app-card .actions a {
	margin-right: 10px;
	font-size: 13px;
}
.button {
	display: inline-block;
	padding: 8px 16px;
	background: #333;
	color: #fff;
	text-decoration: none;
	border-radius: 4px;
}
.button:hover {
	background: #555;
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
		b.WriteString(fmt.Sprintf(`<p>%s</p>`, html.EscapeString(a.Description)))
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
		b.WriteString(fmt.Sprintf(`<a href="/apps/%s/edit">Edit</a>`, a.ID))
		b.WriteString(fmt.Sprintf(`<a href="/apps/%s/delete" onclick="return confirm('Delete this app?')">Delete</a>`, a.ID))
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
		description := strings.TrimSpace(r.FormValue("description"))
		code := r.FormValue("code")
		public := r.FormValue("public") == "on"

		if name == "" {
			renderNewForm(w, "Name is required", name, description, code, public)
			return
		}

		a, err := CreateApp(name, description, code, sess.Account, sess.Account, public)
		if err != nil {
			renderNewForm(w, err.Error(), name, description, code, public)
			return
		}

		http.Redirect(w, r, "/apps/"+a.ID, 302)
		return
	}

	renderNewForm(w, "", "", "", defaultTemplate(), false)
}

func defaultTemplate() string {
	return `<!DOCTYPE html>
<html>
<head>
  <style>
    body {
      font-family: system-ui, sans-serif;
      padding: 20px;
      max-width: 600px;
      margin: 0 auto;
    }
  </style>
</head>
<body>
  <h1>My App</h1>
  <p>Edit this template to build your micro app.</p>
</body>
</html>`
}

func renderNewForm(w http.ResponseWriter, errMsg, name, description, code string, public bool) {
	publicChecked := ""
	if public {
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
.form-group textarea {
	font-family: monospace;
	font-size: 13px;
	min-height: 400px;
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
</style>
%s
<form method="POST" id="app-form">
  <div class="form-group">
    <label>Name</label>
    <input type="text" name="name" value="%s" required>
  </div>
  <div class="form-group">
    <label>Description</label>
    <input type="text" name="description" value="%s" placeholder="Brief description of what this app does">
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
    <button type="submit" class="button">Save App</button>
    <button type="button" class="button button-secondary" onclick="previewApp()">Preview</button>
  </div>
</form>
<iframe id="preview" class="preview-frame" sandbox="allow-scripts"></iframe>
<script>
function previewApp() {
  var code = document.getElementById('code-editor').value;
  var iframe = document.getElementById('preview');
  iframe.srcdoc = code;
}
</script>
`, errHTML, html.EscapeString(name), html.EscapeString(description), html.EscapeString(code), publicChecked)

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
		description := strings.TrimSpace(r.FormValue("description"))
		code := r.FormValue("code")
		public := r.FormValue("public") == "on"

		if name == "" {
			renderEditForm(w, a, "Name is required")
			return
		}

		if err := UpdateApp(id, name, description, code, public, sess.Account); err != nil {
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
.form-group textarea {
	font-family: monospace;
	font-size: 13px;
	min-height: 400px;
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
</style>
%s
<form method="POST" id="app-form">
  <div class="form-group">
    <label>Name</label>
    <input type="text" name="name" value="%s" required>
  </div>
  <div class="form-group">
    <label>Description</label>
    <input type="text" name="description" value="%s" placeholder="Brief description of what this app does">
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
    <button type="submit" class="button">Save Changes</button>
    <button type="button" class="button button-secondary" onclick="previewApp()">Preview</button>
    <a href="/apps/%s" style="margin-left: 10px;">Cancel</a>
  </div>
</form>
<iframe id="preview" class="preview-frame" sandbox="allow-scripts"></iframe>
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
		<div style="margin-bottom: 20px;">
			<a href="/apps/%s/edit" class="button">Edit</a>
			<a href="/apps/%s/delete" class="button button-danger" onclick="return confirm('Delete this app?')">Delete</a>
		</div>`, a.ID, a.ID)
	}

	visibility := "Private"
	if a.Public {
		visibility = "Public"
	}

	html := fmt.Sprintf(`
<style>
.button { 
	display: inline-block;
	padding: 8px 16px; 
	background: #333; 
	color: white; 
	text-decoration: none;
	border-radius: 4px; 
	margin-right: 10px;
}
.button:hover { background: #555; }
.button-danger { background: #c00; }
.button-danger:hover { background: #a00; }
.app-frame {
	width: 100%%;
	height: 500px;
	border: 1px solid #ddd;
	border-radius: 4px;
	background: white;
}
.meta {
	color: #666;
	font-size: 14px;
	margin-bottom: 20px;
}
</style>
%s
<div class="meta">by %s · %s · Updated %s</div>
<p>%s</p>
<iframe class="app-frame" sandbox="allow-scripts" srcdoc="%s"></iframe>
<p style="margin-top: 20px;"><a href="/apps">← Back to Apps</a></p>
`, actions, html.EscapeString(a.Author), visibility, a.UpdatedAt.Format("Jan 2, 2006"), html.EscapeString(a.Description), html.EscapeString(a.Code))

	w.Write([]byte(app.RenderHTML(a.Name, a.Name, html)))
}

func handlePreview(w http.ResponseWriter, r *http.Request, id string) {
	a := GetApp(id)
	if a == nil {
		http.Error(w, "App not found", 404)
		return
	}

	// Preview returns raw HTML for embedding
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(a.Code))
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
