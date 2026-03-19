package apps

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"

	"github.com/google/uuid"
)

// MaxHTMLSize is the maximum size of a mini app's HTML content (256KB).
const MaxHTMLSize = 256 * 1024

// MaxStoreValueSize is the maximum size of a single store value (64KB).
const MaxStoreValueSize = 64 * 1024

// MaxStoreKeys is the maximum number of keys per app+user.
const MaxStoreKeys = 100

// Categories for mini apps.
var Categories = []string{
	"Productivity",
	"Tools",
	"Finance",
	"Writing",
	"Health",
	"Education",
	"Fun",
	"Developer",
	"Other",
}

// App represents a mini app.
type App struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AuthorID    string    `json:"author_id"`
	Author      string    `json:"author"`
	Icon        string    `json:"icon"`
	HTML        string    `json:"html"`
	Category    string    `json:"category"`
	Public      bool      `json:"public"`
	Installs    int       `json:"installs"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var (
	mutex sync.RWMutex
	apps  = map[string]*App{} // slug -> App

	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$`)
)

// Load initialises the apps package and loads cached data.
func Load() {
	b, err := data.LoadFile("apps.json")
	if err != nil {
		return
	}
	var loaded []*App
	if err := json.Unmarshal(b, &loaded); err != nil {
		app.Log("apps", "Failed to load apps.json: %v", err)
		return
	}
	mutex.Lock()
	for _, a := range loaded {
		apps[a.Slug] = a
	}
	mutex.Unlock()
	app.Log("apps", "Loaded %d mini apps", len(loaded))
}

// save persists all apps to disk.
func save() {
	mutex.RLock()
	list := make([]*App, 0, len(apps))
	for _, a := range apps {
		list = append(list, a)
	}
	mutex.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	data.SaveJSON("apps.json", list)
}

// GetApp returns a mini app by slug, or nil if not found.
func GetApp(slug string) *App {
	mutex.RLock()
	defer mutex.RUnlock()
	return apps[slug]
}

// Preview returns HTML for the home dashboard card.
func Preview() string {
	mutex.RLock()
	defer mutex.RUnlock()

	if len(apps) == 0 {
		return `<p class="card-desc">Build and launch mini apps — small, useful tools that do one thing well.</p>
<p><a href="/apps/build">Build with AI</a></p>`
	}

	// Show top 3 most installed public apps
	var public []*App
	for _, a := range apps {
		if a.Public {
			public = append(public, a)
		}
	}
	sort.Slice(public, func(i, j int) bool {
		return public[i].Installs > public[j].Installs
	})

	var sb strings.Builder
	limit := 3
	if len(public) < limit {
		limit = len(public)
	}
	for _, a := range public[:limit] {
		sb.WriteString(fmt.Sprintf(`<p><a href="/apps/%s">%s</a> — %s</p>`,
			htmlpkg.EscapeString(a.Slug),
			htmlpkg.EscapeString(a.Name),
			htmlpkg.EscapeString(truncate(a.Description, 60)),
		))
	}
	sb.WriteString(fmt.Sprintf(`<p class="card-desc">%d apps available</p>`, len(public)))
	return sb.String()
}

// Handler handles /apps requests.
func Handler(w http.ResponseWriter, r *http.Request) {
	// Route sub-paths
	path := strings.TrimPrefix(r.URL.Path, "/apps")
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == "" || path == "/":
		handleList(w, r)
	case path == "/new":
		handleNew(w, r)
	case path == "/build":
		handleBuilder(w, r)
	case path == "/build/generate":
		handleGenerate(w, r)
	case path == "/build/templates":
		handleTemplateList(w, r)
	case strings.HasPrefix(path, "/build/templates/"):
		id := strings.TrimPrefix(path, "/build/templates/")
		handleTemplateGet(w, r, id)
	case path == "/sdk.js":
		handleSDK(w, r)
	case strings.HasSuffix(path, "/run"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/run")
		handleRun(w, r, slug)
	case strings.HasSuffix(path, "/sdk/ai"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/sdk/ai")
		handleSDKAI(w, r, slug)
	case strings.HasSuffix(path, "/sdk/store"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/sdk/store")
		handleSDKStore(w, r, slug)
	default:
		slug := strings.TrimPrefix(path, "/")
		if r.Method == "DELETE" {
			handleDelete(w, r, slug)
		} else if r.Method == "PATCH" || (r.Method == "POST" && app.SendsJSON(r)) {
			handleUpdate(w, r, slug)
		} else {
			handleView(w, r, slug)
		}
	}
}

// handleList shows all public mini apps.
func handleList(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	var list []*App
	for _, a := range apps {
		if a.Public {
			list = append(list, a)
		}
	}
	mutex.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		return list[i].Installs > list[j].Installs
	})

	if app.WantsJSON(r) {
		// Strip HTML from JSON response
		type appSummary struct {
			Slug        string `json:"slug"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Author      string `json:"author"`
			Category    string `json:"category"`
			Installs    int    `json:"installs"`
		}
		summaries := make([]appSummary, len(list))
		for i, a := range list {
			summaries[i] = appSummary{
				Slug:        a.Slug,
				Name:        a.Name,
				Description: a.Description,
				Author:      a.Author,
				Category:    a.Category,
				Installs:    a.Installs,
			}
		}
		app.RespondJSON(w, summaries)
		return
	}

	// HTML
	var sb strings.Builder
	sb.WriteString(`<p class="card-desc">Small, useful apps that do one thing well. No ads, no tracking, no bloat.</p>`)

	// Category filter
	category := r.URL.Query().Get("category")
	sb.WriteString(`<div style="margin-bottom:16px;">`)
	sb.WriteString(fmt.Sprintf(`<a href="/apps" style="margin-right:8px;%s">All</a>`, activeStyle(category == "")))
	for _, cat := range Categories {
		active := strings.EqualFold(category, cat)
		sb.WriteString(fmt.Sprintf(`<a href="/apps?category=%s" style="margin-right:8px;%s">%s</a>`,
			htmlpkg.EscapeString(cat), activeStyle(active), htmlpkg.EscapeString(cat)))
	}
	sb.WriteString(`</div>`)

	// Filter by category
	if category != "" {
		var filtered []*App
		for _, a := range list {
			if strings.EqualFold(a.Category, category) {
				filtered = append(filtered, a)
			}
		}
		list = filtered
	}

	if len(list) == 0 {
		sb.WriteString(`<p>No apps yet. <a href="/apps/new">Create the first one</a>.</p>`)
	} else {
		for _, a := range list {
			sb.WriteString(fmt.Sprintf(`<div style="border:1px solid #eee;border-radius:8px;padding:12px;margin-bottom:12px;">
<h3 style="margin:0 0 4px 0;"><a href="/apps/%s">%s</a></h3>
<p style="margin:0 0 4px 0;color:#666;">%s</p>
<p style="margin:0;font-size:13px;color:#999;">by %s · %s · %d installs</p>
</div>`,
				htmlpkg.EscapeString(a.Slug),
				htmlpkg.EscapeString(a.Name),
				htmlpkg.EscapeString(a.Description),
				htmlpkg.EscapeString(a.Author),
				htmlpkg.EscapeString(a.Category),
				a.Installs,
			))
		}
	}

	sb.WriteString(`<p><a href="/apps/build">Build with AI</a> · <a href="/apps/new">Create manually</a></p>`)

	app.Respond(w, r, app.Response{
		Title:       "Apps",
		Description: "Mini apps — small, useful tools that do one thing well",
		HTML:        sb.String(),
	})
}

// handleNew shows the create form or processes creation.
func handleNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		handleCreate(w, r)
		return
	}

	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var sb strings.Builder
	sb.WriteString(`<p class="card-desc">Create a mini app — a small, self-contained HTML tool.</p>`)
	sb.WriteString(`<form method="POST" action="/apps/new" style="max-width:600px;">`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Name</label><br>`)
	sb.WriteString(`<input type="text" name="name" required maxlength="60" style="width:100%;padding:8px;border:1px solid #ccc;border-radius:4px;" placeholder="Pomodoro Timer"></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Slug (URL-friendly ID)</label><br>`)
	sb.WriteString(`<input type="text" name="slug" required maxlength="50" pattern="[a-z0-9][a-z0-9-]{1,48}[a-z0-9]" style="width:100%;padding:8px;border:1px solid #ccc;border-radius:4px;" placeholder="pomodoro-timer"></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Description</label><br>`)
	sb.WriteString(`<input type="text" name="description" required maxlength="200" style="width:100%;padding:8px;border:1px solid #ccc;border-radius:4px;" placeholder="A simple 25-minute focus timer"></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Category</label><br><select name="category" style="padding:8px;border:1px solid #ccc;border-radius:4px;">`)
	for _, cat := range Categories {
		sb.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, htmlpkg.EscapeString(cat), htmlpkg.EscapeString(cat)))
	}
	sb.WriteString(`</select></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>HTML (your app — max 256KB)</label><br>`)
	sb.WriteString(`<textarea name="html" required style="width:100%;min-height:300px;padding:8px;border:1px solid #ccc;border-radius:4px;font-family:monospace;font-size:13px;" placeholder="<h1>Hello World</h1>"></textarea></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label><input type="checkbox" name="public" value="1" checked> List in public directory</label></div>`)
	sb.WriteString(`<button type="submit" style="padding:8px 24px;background:#000;color:#fff;border:none;border-radius:4px;cursor:pointer;">Create App</button>`)
	sb.WriteString(`</form>`)

	app.Respond(w, r, app.Response{
		Title:       "Create App",
		Description: "Create a new mini app",
		HTML:        sb.String(),
	})
}

// handleCreate processes app creation (POST).
func handleCreate(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var name, slug, description, category, html string
	var public bool

	if app.SendsJSON(r) {
		var req struct {
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
			Category    string `json:"category"`
			HTML        string `json:"html"`
			Public      *bool  `json:"public"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		name = req.Name
		slug = req.Slug
		description = req.Description
		category = req.Category
		html = req.HTML
		if req.Public != nil {
			public = *req.Public
		} else {
			public = true
		}
	} else {
		r.ParseForm()
		name = strings.TrimSpace(r.FormValue("name"))
		slug = strings.TrimSpace(r.FormValue("slug"))
		description = strings.TrimSpace(r.FormValue("description"))
		category = strings.TrimSpace(r.FormValue("category"))
		html = r.FormValue("html")
		public = r.FormValue("public") == "1"
	}

	// Validate
	if name == "" || slug == "" || description == "" || html == "" {
		app.Error(w, r, http.StatusBadRequest, "Name, slug, description, and HTML are required")
		return
	}
	if !slugRe.MatchString(slug) {
		app.Error(w, r, http.StatusBadRequest, "Slug must be 3-50 chars, lowercase letters, numbers, and hyphens")
		return
	}
	if len(html) > MaxHTMLSize {
		app.Error(w, r, http.StatusBadRequest, "HTML content exceeds 256KB limit")
		return
	}
	if !validCategory(category) {
		category = "Other"
	}

	// Check slug uniqueness
	mutex.RLock()
	_, exists := apps[slug]
	mutex.RUnlock()
	if exists {
		app.Error(w, r, http.StatusConflict, "An app with this slug already exists")
		return
	}

	now := time.Now()
	newApp := &App{
		ID:          uuid.New().String(),
		Slug:        slug,
		Name:        name,
		Description: description,
		AuthorID:    acc.ID,
		Author:      acc.Name,
		HTML:        html,
		Category:    category,
		Public:      public,
		Installs:    0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutex.Lock()
	apps[slug] = newApp
	mutex.Unlock()

	save()

	app.Log("apps", "Created app %q by %s", name, acc.ID)

	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, newApp)
		return
	}
	http.Redirect(w, r, "/apps/"+slug, http.StatusSeeOther)
}

// handleView shows a single mini app page.
func handleView(w http.ResponseWriter, r *http.Request, slug string) {
	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, a)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<p class="card-desc">%s</p>`, htmlpkg.EscapeString(a.Description)))
	sb.WriteString(fmt.Sprintf(`<p style="font-size:13px;color:#999;">by %s · %s · %d installs · created %s</p>`,
		htmlpkg.EscapeString(a.Author),
		htmlpkg.EscapeString(a.Category),
		a.Installs,
		a.CreatedAt.Format("2 Jan 2006"),
	))

	// Run button
	sb.WriteString(fmt.Sprintf(`<p><a href="/apps/%s/run" style="display:inline-block;padding:8px 24px;background:#000;color:#fff;border-radius:4px;text-decoration:none;">Launch App</a></p>`, htmlpkg.EscapeString(a.Slug)))

	// Edit/delete for author
	_, acc, err := auth.RequireSession(r)
	if err == nil && acc.ID == a.AuthorID {
		sb.WriteString(fmt.Sprintf(`<p style="margin-top:16px;"><a href="/apps/%s/run">Preview</a> · <a href="#" onclick="if(confirm('Delete this app?')){fetch('/apps/%s',{method:'DELETE'}).then(()=>location='/apps')}">Delete</a></p>`,
			htmlpkg.EscapeString(a.Slug), htmlpkg.EscapeString(a.Slug)))
	}

	app.Respond(w, r, app.Response{
		Title:       a.Name,
		Description: a.Description,
		HTML:        sb.String(),
	})
}

// handleRun renders the mini app in a sandboxed iframe.
func handleRun(w http.ResponseWriter, r *http.Request, slug string) {
	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}

	// Serve raw HTML for iframe src (with sandbox origin isolation)
	if r.URL.Query().Get("raw") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'unsafe-inline' 'self' data: blob:; script-src 'unsafe-inline'; style-src 'unsafe-inline';")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")

		// Inject SDK bridge script before closing </head> or at start
		sdkBridge := `<script>
window.mu={_id:0,_cb:{},
_send:function(t,d){var id=++this._id;return new Promise(function(ok,fail){mu._cb[id]={ok:ok,fail:fail};window.parent.postMessage({type:'mu:'+t,id:id,data:d},'*');})},
ai:function(p,o){return this._send('ai',{prompt:p,options:o||{}})},
fetch:function(u){return this._send('fetch',{url:u})},
user:function(){return this._send('user',{})},
store:{
set:function(k,v){return mu._send('store',{op:'set',key:k,value:v})},
get:function(k){return mu._send('store',{op:'get',key:k})},
del:function(k){return mu._send('store',{op:'del',key:k})},
keys:function(){return mu._send('store',{op:'keys'})}
}};
window.addEventListener('message',function(e){var d=e.data;if(d&&d.type&&d.type.indexOf('mu:')===0&&d.id&&mu._cb[d.id]){if(d.error){mu._cb[d.id].fail(new Error(d.error))}else{mu._cb[d.id].ok(d.result)}delete mu._cb[d.id];}});
</script>`
		html := a.HTML
		if idx := strings.Index(strings.ToLower(html), "<head>"); idx >= 0 {
			html = html[:idx+6] + sdkBridge + html[idx+6:]
		} else if idx := strings.Index(strings.ToLower(html), "<html"); idx >= 0 {
			// Find the end of the <html> tag
			end := strings.Index(html[idx:], ">")
			if end >= 0 {
				pos := idx + end + 1
				html = html[:pos] + sdkBridge + html[pos:]
			}
		} else {
			html = sdkBridge + html
		}
		w.Write([]byte(html))
		return
	}

	// Render the run page with iframe
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<p><a href="/apps/%s">&larr; %s</a></p>`,
		htmlpkg.EscapeString(a.Slug), htmlpkg.EscapeString(a.Name)))
	sb.WriteString(fmt.Sprintf(`<iframe src="/apps/%s/run?raw=1" sandbox="allow-scripts" style="width:100%%;min-height:70vh;border:1px solid #eee;border-radius:8px;background:#fff;"></iframe>`,
		htmlpkg.EscapeString(a.Slug)))

	// SDK bridge in parent page: listens for postMessage from iframe and proxies to backend
	sb.WriteString(fmt.Sprintf(`<script>
window.addEventListener('message',function(e){
var d=e.data;if(!d||!d.type||d.type.indexOf('mu:')!==0)return;
var t=d.type.replace('mu:','');
var slug=%q;
var url='/apps/'+slug+'/sdk/';
if(t==='ai'){url+='ai'}
else if(t==='fetch'){url+='ai'}
else if(t==='store'){url+='store'}
else if(t==='user'){
fetch('/session').then(function(r){return r.json()}).then(function(j){
var iframe=document.querySelector('iframe');
iframe.contentWindow.postMessage({type:d.type+':res',id:d.id,result:j},'*');
});return;
}
fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(d.data)})
.then(function(r){return r.json()})
.then(function(j){
var iframe=document.querySelector('iframe');
iframe.contentWindow.postMessage({type:d.type+':res',id:d.id,result:j},'*');
})
.catch(function(err){
var iframe=document.querySelector('iframe');
iframe.contentWindow.postMessage({type:d.type+':res',id:d.id,error:err.message},'*');
});
});
</script>`, a.Slug))

	app.Respond(w, r, app.Response{
		Title:       a.Name,
		Description: a.Description,
		HTML:        sb.String(),
	})
}

// handleUpdate processes PATCH requests to update an app.
func handleUpdate(w http.ResponseWriter, r *http.Request, slug string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}
	if a.AuthorID != acc.ID {
		app.Forbidden(w, r, "You can only edit your own apps")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Category    *string `json:"category"`
		HTML        *string `json:"html"`
		Public      *bool   `json:"public"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	mutex.Lock()
	if req.Name != nil && *req.Name != "" {
		a.Name = *req.Name
	}
	if req.Description != nil {
		a.Description = *req.Description
	}
	if req.Category != nil && validCategory(*req.Category) {
		a.Category = *req.Category
	}
	if req.HTML != nil {
		if len(*req.HTML) > MaxHTMLSize {
			mutex.Unlock()
			app.RespondError(w, http.StatusBadRequest, "HTML content exceeds 256KB limit")
			return
		}
		a.HTML = *req.HTML
	}
	if req.Public != nil {
		a.Public = *req.Public
	}
	a.UpdatedAt = time.Now()
	mutex.Unlock()

	save()

	app.RespondJSON(w, a)
}

// handleDelete deletes an app.
func handleDelete(w http.ResponseWriter, r *http.Request, slug string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}
	if a.AuthorID != acc.ID {
		app.Forbidden(w, r, "You can only delete your own apps")
		return
	}

	mutex.Lock()
	delete(apps, slug)
	mutex.Unlock()

	save()

	app.Log("apps", "Deleted app %q by %s", a.Name, acc.ID)

	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "deleted"})
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

// handleSDK serves the SDK JavaScript file.
func handleSDK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(sdkJS))
}

// handleSDKAI proxies AI requests from mini apps.
func handleSDKAI(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req struct {
		Prompt  string `json:"prompt"`
		Options struct {
			Context string `json:"context"`
		} `json:"options"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// For now, return a placeholder — this will be wired to the AI subsystem
	app.RespondJSON(w, map[string]string{
		"result": "AI integration coming soon. Prompt: " + truncate(req.Prompt, 100),
	})
}

// handleSDKStore handles key-value storage for mini apps.
func handleSDKStore(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req struct {
		Op    string      `json:"op"`
		Key   string      `json:"key"`
		Value interface{} `json:"value"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	storeKey := fmt.Sprintf("apps/%s/%s.json", slug, acc.ID)

	switch req.Op {
	case "set":
		if req.Key == "" {
			app.RespondError(w, http.StatusBadRequest, "Key is required")
			return
		}
		store := loadStore(storeKey)
		if len(store) >= MaxStoreKeys && store[req.Key] == nil {
			app.RespondError(w, http.StatusBadRequest, "Maximum 100 keys per app")
			return
		}
		valBytes, _ := json.Marshal(req.Value)
		if len(valBytes) > MaxStoreValueSize {
			app.RespondError(w, http.StatusBadRequest, "Value exceeds 64KB limit")
			return
		}
		store[req.Key] = req.Value
		data.SaveJSON(storeKey, store)
		app.RespondJSON(w, map[string]string{"status": "ok"})

	case "get":
		store := loadStore(storeKey)
		val := store[req.Key]
		app.RespondJSON(w, map[string]interface{}{"result": val})

	case "del":
		store := loadStore(storeKey)
		delete(store, req.Key)
		data.SaveJSON(storeKey, store)
		app.RespondJSON(w, map[string]string{"status": "ok"})

	case "keys":
		store := loadStore(storeKey)
		keys := make([]string, 0, len(store))
		for k := range store {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		app.RespondJSON(w, map[string]interface{}{"result": keys})

	default:
		app.RespondError(w, http.StatusBadRequest, "Invalid operation. Use set, get, del, or keys")
	}
}

// GetApp returns an app by slug.
func GetApp(slug string) *App {
	mutex.RLock()
	defer mutex.RUnlock()
	return apps[slug]
}

// GetPublicApps returns all public apps sorted by installs.
func GetPublicApps() []*App {
	mutex.RLock()
	defer mutex.RUnlock()

	var list []*App
	for _, a := range apps {
		if a.Public {
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Installs > list[j].Installs
	})
	return list
}

// SearchApps searches for apps by query string.
func SearchApps(query string) []*App {
	query = strings.ToLower(query)
	mutex.RLock()
	defer mutex.RUnlock()

	var results []*App
	for _, a := range apps {
		if !a.Public {
			continue
		}
		if strings.Contains(strings.ToLower(a.Name), query) ||
			strings.Contains(strings.ToLower(a.Description), query) ||
			strings.EqualFold(a.Category, query) {
			results = append(results, a)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Installs > results[j].Installs
	})
	return results
}

// loadStore loads the key-value store for an app+user combination.
func loadStore(key string) map[string]interface{} {
	b, err := data.LoadFile(key)
	if err != nil {
		return make(map[string]interface{})
	}
	var store map[string]interface{}
	if err := json.Unmarshal(b, &store); err != nil {
		return make(map[string]interface{})
	}
	return store
}

func validCategory(cat string) bool {
	for _, c := range Categories {
		if strings.EqualFold(c, cat) {
			return true
		}
	}
	return false
}

func activeStyle(active bool) string {
	if active {
		return "font-weight:bold;"
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SDK JavaScript served at /apps/sdk.js
const sdkJS = `// Mu Mini App SDK
// Include this in your mini app: <script src="/apps/sdk.js"></script>
(function() {
  var id = 0;
  var callbacks = {};

  window.mu = {
    // Ask AI a question
    ai: function(prompt, options) {
      return send('ai', { prompt: prompt, options: options || {} });
    },

    // Fetch a URL through Mu's proxy
    fetch: function(url) {
      return send('fetch', { url: url });
    },

    // Get current user info
    user: function() {
      return send('user', {});
    },

    // Key-value storage
    store: {
      set: function(key, value) { return send('store', { op: 'set', key: key, value: value }); },
      get: function(key) { return send('store', { op: 'get', key: key }).then(function(r) { return r.result; }); },
      del: function(key) { return send('store', { op: 'del', key: key }); },
      keys: function() { return send('store', { op: 'keys' }).then(function(r) { return r.result; }); }
    }
  };

  function send(type, data) {
    var reqId = ++id;
    return new Promise(function(resolve, reject) {
      callbacks[reqId] = { ok: resolve, fail: reject };
      window.parent.postMessage({ type: 'mu:' + type, id: reqId, data: data }, '*');
    });
  }

  window.addEventListener('message', function(e) {
    var d = e.data;
    if (d && d.id && callbacks[d.id]) {
      if (d.error) {
        callbacks[d.id].fail(new Error(d.error));
      } else {
        callbacks[d.id].ok(d.result);
      }
      delete callbacks[d.id];
    }
  });
})();
`
