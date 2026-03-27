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
	"mu/internal/event"

	"github.com/google/uuid"
)

// MaxHTMLSize is the maximum size of an app's HTML content (256KB).
const MaxHTMLSize = 256 * 1024

// MaxStoreValueSize is the maximum size of a single store value (64KB).
const MaxStoreValueSize = 64 * 1024

// MaxStoreKeys is the maximum number of keys per app+user.
const MaxStoreKeys = 100

// Version represents a snapshot of an app at a point in time.
type Version struct {
	Number    int       `json:"number"`
	HTML      string    `json:"html"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon,omitempty"`
	SavedAt   time.Time `json:"saved_at"`
	Summary   string    `json:"summary,omitempty"` // optional change description
}

// MaxVersions is the maximum number of versions kept per app.
const MaxVersions = 50

// App represents an app.
type App struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AuthorID    string    `json:"author_id"`
	Author      string    `json:"author"`
	Icon        string    `json:"icon"`
	HTML        string    `json:"html"`
	Tags        string    `json:"tags"` // Comma-separated tags
	Public      bool      `json:"public"`
	Installs    int       `json:"installs"`
	ForkedFrom  string    `json:"forked_from,omitempty"` // slug of original app
	Versions    []Version `json:"versions,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var (
	mutex sync.RWMutex
	apps  = map[string]*App{} // slug -> App

	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$`)
)

// snapshotVersion saves the current state as a new version. Must be called with mutex held.
func snapshotVersion(a *App, summary string) {
	num := 1
	if len(a.Versions) > 0 {
		num = a.Versions[len(a.Versions)-1].Number + 1
	}
	v := Version{
		Number:  num,
		HTML:    a.HTML,
		Name:    a.Name,
		Icon:    a.Icon,
		SavedAt: time.Now(),
		Summary: summary,
	}
	a.Versions = append(a.Versions, v)
	// Trim old versions
	if len(a.Versions) > MaxVersions {
		a.Versions = a.Versions[len(a.Versions)-MaxVersions:]
	}
}

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
	app.Log("apps", "Loaded %d apps", len(loaded))

	// Seed built-in apps on first run
	if len(loaded) == 0 {
		seedApps()
	}

	data.RegisterDeleter("app", DeleteApp)
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

// Preview returns HTML for the home dashboard card.
func Preview() string {
	mutex.RLock()
	defer mutex.RUnlock()

	if len(apps) == 0 {
		return `<p><a href="/apps/build">+ Create your first app</a></p>`
	}

	// Show 3 most recent public apps
	var public []*App
	for _, a := range apps {
		if a.Public {
			public = append(public, a)
		}
	}
	if len(public) == 0 {
		return `<p><a href="/apps/build">+ Create your first app</a></p>`
	}
	sort.Slice(public, func(i, j int) bool {
		return public[i].CreatedAt.After(public[j].CreatedAt)
	})

	var sb strings.Builder
	limit := 3
	if len(public) < limit {
		limit = len(public)
	}
	for _, a := range public[:limit] {
		sb.WriteString(fmt.Sprintf(`<p style="display:flex;align-items:center;gap:8px;"><img src="/apps/%s/icon.svg" width="20" height="20"><a href="/apps/%s">%s</a> — %s</p>`,
			htmlpkg.EscapeString(a.Slug),
			htmlpkg.EscapeString(a.Slug),
			htmlpkg.EscapeString(a.Name),
			htmlpkg.EscapeString(truncate(a.Description, 60)),
		))
	}
	sb.WriteString(fmt.Sprintf(`<p class="card-desc">%d apps available · <a href="/apps/build">Build new</a></p>`, len(public)))
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
	case path == "/run":
		handleCodeRun(w, r)
	case path == "/sdk.js":
		handleSDK(w, r)
	case strings.HasSuffix(path, "/edit"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/edit")
		handleEdit(w, r, slug)
	case strings.HasSuffix(path, "/versions"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/versions")
		handleVersions(w, r, slug)
	case strings.HasSuffix(path, "/fork"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/fork")
		handleFork(w, r, slug)
	case strings.HasSuffix(path, "/icon.svg"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/icon.svg")
		handleIcon(w, r, slug)
	case strings.HasSuffix(path, "/run"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/run")
		handleRun(w, r, slug)
	case strings.HasSuffix(path, "/sdk/ai"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/sdk/ai")
		handleSDKAI(w, r, slug)
	case strings.HasSuffix(path, "/sdk/store"):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/sdk/store")
		handleSDKStore(w, r, slug)
	case strings.HasSuffix(path, "/delete") && r.Method == "POST":
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/delete")
		handleDelete(w, r, slug)
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

// handleList shows all public apps.
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
			Tags        string `json:"tags"`
			Installs    int    `json:"installs"`
		}
		summaries := make([]appSummary, len(list))
		for i, a := range list {
			summaries[i] = appSummary{
				Slug:        a.Slug,
				Name:        a.Name,
				Description: a.Description,
				Author:      a.Author,
				Tags:        a.Tags,
				Installs:    a.Installs,
			}
		}
		app.RespondJSON(w, summaries)
		return
	}

	sess, acc := auth.TrySession(r)
	var userID string
	var isAdmin bool
	if sess != nil {
		userID = sess.Account
		isAdmin = acc.Admin
	}

	// HTML
	var sb strings.Builder
	sb.WriteString(`<p class="card-desc">Small, useful apps that do one thing well. No ads, no tracking, no bloat.</p>`)

	// Tag filter
	tag := r.URL.Query().Get("tag")

	// Collect known tags from public apps for filter pills
	tagSet := map[string]bool{}
	for _, a := range list {
		for _, t := range splitTags(a.Tags) {
			tagSet[t] = true
		}
	}
	if len(tagSet) > 0 {
		sb.WriteString(`<div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:16px;">`)
		sb.WriteString(fmt.Sprintf(`<a href="/apps" style="padding:4px 12px;border-radius:12px;font-size:12px;text-decoration:none;%s">All</a>`, pillStyle(tag == "")))
		var sortedTags []string
		for t := range tagSet {
			sortedTags = append(sortedTags, t)
		}
		sort.Strings(sortedTags)
		for _, t := range sortedTags {
			sb.WriteString(fmt.Sprintf(`<a href="/apps?tag=%s" style="padding:4px 12px;border-radius:12px;font-size:12px;text-decoration:none;%s">%s</a>`,
				htmlpkg.EscapeString(t), pillStyle(strings.EqualFold(tag, t)), htmlpkg.EscapeString(t)))
		}
		sb.WriteString(`</div>`)
	}

	// Filter by tag
	if tag != "" {
		var filtered []*App
		for _, a := range list {
			if hasTag(a.Tags, tag) {
				filtered = append(filtered, a)
			}
		}
		list = filtered
	}

	if len(list) == 0 {
		sb.WriteString(`<p>No apps yet. <a href="/apps/new">Create the first one</a>.</p>`)
	} else {
		for _, a := range list {
			if userID != "" && (app.IsBlocked(userID, a.AuthorID) || app.IsDismissed(userID, "app", a.Slug)) {
				continue
			}
			tagsHTML := ""
			if a.Tags != "" {
				tagsHTML = " · " + htmlpkg.EscapeString(a.Tags)
			}
			controls := app.ItemControls(userID, isAdmin, "app", a.Slug, a.AuthorID, "/apps/"+a.Slug+"/edit", "/apps/"+a.Slug+"/delete")
			sb.WriteString(fmt.Sprintf(`<div style="position:relative;border:1px solid #eee;border-radius:8px;padding:12px;margin-bottom:12px;display:flex;gap:12px;align-items:flex-start;">
<img src="/apps/%s/icon.svg" width="32" height="32" style="flex-shrink:0;margin-top:2px;">
<div>
<h3 style="margin:0 0 4px 0;"><a href="/apps/%s/run">%s</a></h3>
<p style="margin:0 0 4px 0;color:#666;">%s</p>
<p style="margin:0;font-size:13px;color:#999;">by %s%s · %d launches%s</p>
</div>
</div>`,
				htmlpkg.EscapeString(a.Slug),
				htmlpkg.EscapeString(a.Slug),
				htmlpkg.EscapeString(a.Name),
				htmlpkg.EscapeString(a.Description),
				htmlpkg.EscapeString(a.Author),
				tagsHTML,
				a.Installs,
				controls,
			))
		}
	}

	sb.WriteString(`<p><a href="/apps/build">Build with AI</a> · <a href="/apps/new">Create manually</a></p>`)

	app.Respond(w, r, app.Response{
		Title:       "Apps",
		Description: "Apps — small, useful tools that do one thing well",
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
	sb.WriteString(`<p class="card-desc">Create an app — a small, self-contained HTML tool.</p>`)
	sb.WriteString(`<form method="POST" action="/apps/new" style="max-width:600px;">`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Name</label><br>`)
	sb.WriteString(`<input type="text" name="name" required maxlength="60" style="width:100%;box-sizing:border-box;padding:8px;border:1px solid #ccc;border-radius:4px;" placeholder="Pomodoro Timer"></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Description</label><br>`)
	sb.WriteString(`<input type="text" name="description" maxlength="200" style="width:100%;box-sizing:border-box;padding:8px;border:1px solid #ccc;border-radius:4px;" placeholder="A simple 25-minute focus timer"></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>Tags <span style="color:#999;font-size:12px;">(comma-separated, optional)</span></label><br>`)
	sb.WriteString(`<input type="text" name="tags" maxlength="200" style="width:100%;box-sizing:border-box;padding:8px;border:1px solid #ccc;border-radius:4px;" placeholder="productivity, timer"></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label>HTML (your app — max 256KB)</label><br>`)
	sb.WriteString(`<textarea name="html" required style="width:100%;min-height:300px;padding:8px;border:1px solid #ccc;border-radius:4px;font-family:monospace;font-size:13px;" placeholder="<h1>Hello World</h1>"></textarea></div>`)
	sb.WriteString(`<div style="margin-bottom:12px;"><label><input type="checkbox" name="public" value="1" checked> List in public directory</label></div>`)
	sb.WriteString(`<button type="submit" style="padding:8px 24px;background:#000;color:#fff;border:none;border-radius:4px;cursor:pointer;">Create App</button>`)
	sb.WriteString(`</form>`)

	app.Respond(w, r, app.Response{
		Title:       "Create App",
		Description: "Create a new app",
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

	var name, slug, icon, description, tags, html string
	var public bool

	if app.SendsJSON(r) {
		var req struct {
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Icon        string `json:"icon"`
			Description string `json:"description"`
			Tags        string `json:"tags"`
			Category    string `json:"category"` // backward compat
			HTML        string `json:"html"`
			Public      *bool  `json:"public"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		name = req.Name
		slug = req.Slug
		icon = req.Icon
		description = req.Description
		tags = req.Tags
		if tags == "" {
			tags = req.Category // backward compat
		}
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
		tags = strings.TrimSpace(r.FormValue("tags"))
		html = r.FormValue("html")
		public = r.FormValue("public") == "1"
	}

	// Derive slug from name if not provided
	if slug == "" && name != "" {
		slug = slugify(name)
	}

	// Default description to name if not provided
	if description == "" {
		description = name
	}

	// Validate
	if name == "" || slug == "" || html == "" {
		app.Error(w, r, http.StatusBadRequest, "Name and HTML are required")
		return
	}
	if !slugRe.MatchString(slug) {
		app.Error(w, r, http.StatusBadRequest, "ID must be 3-50 chars, lowercase letters, numbers, and hyphens")
		return
	}
	if len(html) > MaxHTMLSize {
		app.Error(w, r, http.StatusBadRequest, "HTML content exceeds 256KB limit")
		return
	}

	// Ensure unique slug
	mutex.RLock()
	base := slug
	for i := 2; apps[slug] != nil; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	mutex.RUnlock()

	now := time.Now()
	newApp := &App{
		ID:          uuid.New().String(),
		Slug:        slug,
		Name:        name,
		Description: description,
		AuthorID:    acc.ID,
		Author:      acc.Name,
		Icon:        icon,
		HTML:        html,
		Tags:        tags,
		Public:      public,
		Installs:    0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutex.Lock()
	snapshotVersion(newApp, "Initial version")
	apps[slug] = newApp
	mutex.Unlock()

	save()

	app.Log("apps", "Created app %q by %s", name, acc.ID)

	// Notify home dashboard to refresh
	event.Publish(event.Event{Type: "apps_updated"})

	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, newApp)
		return
	}
	http.Redirect(w, r, "/apps/"+slug, http.StatusSeeOther)
}

// handleView shows a single app page.
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
	sb.WriteString(fmt.Sprintf(`<div style="display:flex;gap:12px;align-items:center;margin-bottom:12px;"><img src="/apps/%s/icon.svg" width="32" height="32"><div><p class="card-desc" style="margin:0;">%s</p></div></div>`,
		htmlpkg.EscapeString(a.Slug), htmlpkg.EscapeString(a.Description)))
	tagsInfo := ""
	if a.Tags != "" {
		tagsInfo = " · " + htmlpkg.EscapeString(a.Tags)
	}
	forkedInfo := ""
	if a.ForkedFrom != "" {
		forkedInfo = fmt.Sprintf(` · forked from <a href="/apps/%s">%s</a>`, htmlpkg.EscapeString(a.ForkedFrom), htmlpkg.EscapeString(a.ForkedFrom))
	}
	savedInfo := ""
	if !a.UpdatedAt.Equal(a.CreatedAt) {
		savedInfo = " · updated " + a.UpdatedAt.Format("2 Jan 2006 15:04")
	}
	versionInfo := ""
	if len(a.Versions) > 1 {
		versionInfo = fmt.Sprintf(` · <a href="/apps/%s/versions">v%d</a>`, htmlpkg.EscapeString(a.Slug), a.Versions[len(a.Versions)-1].Number)
	}
	sb.WriteString(fmt.Sprintf(`<p style="font-size:13px;color:#999;">by %s%s · %d launches%s%s%s</p>`,
		htmlpkg.EscapeString(a.Author),
		tagsInfo,
		a.Installs,
		savedInfo,
		versionInfo,
		forkedInfo,
	))

	// Run button + Fork button
	sb.WriteString(fmt.Sprintf(`<p><a href="/apps/%s/run" style="display:inline-block;padding:8px 24px;background:#000;color:#fff;border-radius:4px;text-decoration:none;">Launch App</a>`, htmlpkg.EscapeString(a.Slug)))
	_, detailAcc, detailErr := auth.RequireSession(r)
	if detailErr == nil {
		sb.WriteString(fmt.Sprintf(` <a href="/apps/%s/fork" style="display:inline-block;padding:8px 24px;background:#fff;color:#333;border:1px solid #ccc;border-radius:4px;text-decoration:none;margin-left:8px;">Fork</a>`,
			htmlpkg.EscapeString(a.Slug)))
	}
	sb.WriteString(`</p>`)

	// Controls (edit, delete, save, flag, etc.)
	var detailUserID string
	var detailAdmin bool
	if detailErr == nil {
		detailUserID = detailAcc.ID
		detailAdmin = detailAcc.Admin
	}
	// Admin/author controls as plain text links
	if detailAdmin || detailUserID == a.AuthorID {
		sb.WriteString(`<p style="margin-top:16px;font-size:13px">`)
		sb.WriteString(fmt.Sprintf(`<a href="/apps/%s/edit" class="text-muted">Edit</a>`, htmlpkg.EscapeString(a.Slug)))
		sb.WriteString(fmt.Sprintf(` · <a href="#" class="text-error" onclick="if(confirm('Delete this app?')){fetch('/apps/%s',{method:'DELETE'}).then(function(){window.location='/apps'})}return false;">Delete</a>`, htmlpkg.EscapeString(a.Slug)))
		sb.WriteString(`</p>`)
	}

	app.Respond(w, r, app.Response{
		Title:       a.Name,
		Description: a.Description,
		HTML:        sb.String(),
	})
}

// handleEdit shows the builder pre-populated with the app's data for editing.
func handleEdit(w http.ResponseWriter, r *http.Request, slug string) {
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
	if a.AuthorID != acc.ID && !acc.Admin {
		app.Forbidden(w, r, "You can only edit your own apps")
		return
	}

	var sb strings.Builder
	sb.WriteString(editPageHTML(a))

	app.Respond(w, r, app.Response{
		Title:       "Edit " + a.Name,
		Description: "Edit " + a.Name,
		HTML:        sb.String(),
	})
}

// handleVersions shows version history for an app.
func handleVersions(w http.ResponseWriter, r *http.Request, slug string) {
	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}

	// POST to restore a version (author only)
	if r.Method == "POST" {
		_, acc, err := auth.RequireSession(r)
		if err != nil {
			app.Unauthorized(w, r)
			return
		}
		if a.AuthorID != acc.ID {
			app.Forbidden(w, r, "You can only restore your own apps")
			return
		}
		r.ParseForm()
		numStr := r.FormValue("version")
		num := 0
		fmt.Sscanf(numStr, "%d", &num)
		if num == 0 {
			app.Error(w, r, http.StatusBadRequest, "Invalid version number")
			return
		}
		mutex.Lock()
		for _, v := range a.Versions {
			if v.Number == num {
				a.HTML = v.HTML
				a.Name = v.Name
				if v.Icon != "" {
					a.Icon = v.Icon
				}
				a.UpdatedAt = time.Now()
				snapshotVersion(a, fmt.Sprintf("Restored from v%d", num))
				break
			}
		}
		mutex.Unlock()
		save()
		http.Redirect(w, r, "/apps/"+slug, http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		// Return versions without HTML to keep response small
		type versionSummary struct {
			Number  int       `json:"number"`
			Name    string    `json:"name"`
			SavedAt time.Time `json:"saved_at"`
			Summary string    `json:"summary,omitempty"`
			Size    int       `json:"size"`
		}
		summaries := make([]versionSummary, len(a.Versions))
		for i, v := range a.Versions {
			summaries[i] = versionSummary{
				Number:  v.Number,
				Name:    v.Name,
				SavedAt: v.SavedAt,
				Summary: v.Summary,
				Size:    len(v.HTML),
			}
		}
		app.RespondJSON(w, summaries)
		return
	}

	_, acc, _ := auth.RequireSession(r)
	isAuthor := acc != nil && acc.ID == a.AuthorID

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<p><a href="/apps/%s">&larr; %s</a></p>`, htmlpkg.EscapeString(a.Slug), htmlpkg.EscapeString(a.Name)))
	sb.WriteString(`<h2 style="margin-bottom:16px;">Version History</h2>`)

	if len(a.Versions) == 0 {
		sb.WriteString(`<p style="color:#999;">No version history yet.</p>`)
	} else {
		// Show newest first
		for i := len(a.Versions) - 1; i >= 0; i-- {
			v := a.Versions[i]
			summary := v.Summary
			if summary == "" {
				summary = "Saved"
			}
			isCurrent := i == len(a.Versions)-1
			currentBadge := ""
			if isCurrent {
				currentBadge = ` <span style="background:#000;color:#fff;padding:2px 8px;border-radius:4px;font-size:11px;">current</span>`
			}
			restoreBtn := ""
			if isAuthor && !isCurrent {
				restoreBtn = fmt.Sprintf(` · <form method="POST" action="/apps/%s/versions" style="display:inline;"><input type="hidden" name="version" value="%d"><button type="submit" style="background:none;border:none;color:#0066cc;cursor:pointer;padding:0;font-family:inherit;font-size:13px;" onclick="return confirm('Restore version %d?')">Restore</button></form>`,
					htmlpkg.EscapeString(a.Slug), v.Number, v.Number)
			}
			sb.WriteString(fmt.Sprintf(`<div style="border:1px solid #eee;border-radius:6px;padding:12px;margin-bottom:8px;">
<div style="display:flex;justify-content:space-between;align-items:center;">
<div><strong>v%d</strong>%s — %s</div>
<span style="font-size:13px;color:#999;">%s%s</span>
</div>
</div>`,
				v.Number,
				currentBadge,
				htmlpkg.EscapeString(summary),
				v.SavedAt.Format("2 Jan 2006 15:04"),
				restoreBtn,
			))
		}
	}

	app.Respond(w, r, app.Response{
		Title:       "Versions — " + a.Name,
		Description: "Version history for " + a.Name,
		HTML:        sb.String(),
	})
}

// handleFork creates a copy of an app under the current user's account.
func handleFork(w http.ResponseWriter, r *http.Request, slug string) {
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

	// Generate a unique slug for the fork
	newSlug := slug
	mutex.RLock()
	base := newSlug
	for i := 2; apps[newSlug] != nil; i++ {
		newSlug = fmt.Sprintf("%s-%d", base, i)
	}
	mutex.RUnlock()

	now := time.Now()
	forked := &App{
		ID:          uuid.New().String(),
		Slug:        newSlug,
		Name:        a.Name,
		Description: a.Description,
		AuthorID:    acc.ID,
		Author:      acc.Name,
		Icon:        a.Icon,
		HTML:        a.HTML,
		Tags:        a.Tags,
		Public:      true,
		ForkedFrom:  slug,
		Installs:    0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutex.Lock()
	snapshotVersion(forked, "Forked from "+a.Name)
	apps[newSlug] = forked
	mutex.Unlock()

	save()

	app.Log("apps", "Forked app %q -> %q by %s", slug, newSlug, acc.ID)
	event.Publish(event.Event{Type: "apps_updated"})

	http.Redirect(w, r, "/apps/"+newSlug+"/edit", http.StatusSeeOther)
}

// handleRun renders the app in a sandboxed iframe.
func handleRun(w http.ResponseWriter, r *http.Request, slug string) {
	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}

	// Count launch (non-raw only, skip author's own launches)
	if r.URL.Query().Get("raw") != "1" {
		_, acc, _ := auth.RequireSession(r)
		if acc == nil || acc.ID != a.AuthorID {
			mutex.Lock()
			a.Installs++
			mutex.Unlock()
			save()
		}
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
run:function(result){window.parent.postMessage({type:'mu:run',result:result},'*');},
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
	sb.WriteString(fmt.Sprintf(`<iframe src="/apps/%s/run?raw=1" sandbox="allow-scripts" allow="geolocation" style="width:100%%;min-height:70vh;border:1px solid #eee;border-radius:8px;background:#fff;"></iframe>`,
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
else if(t==='api'){
var method=d.data.method||'GET';
var path=d.data.path||'/';
var opts={method:method,headers:{'Content-Type':'application/json','Accept':'application/json'}};
if(d.data.body)opts.body=JSON.stringify(d.data.body);
fetch(path,opts).then(function(r){return r.json()}).then(function(j){
var iframe=document.querySelector('iframe');
iframe.contentWindow.postMessage({type:d.type+':res',id:d.id,result:j},'*');
}).catch(function(err){
var iframe=document.querySelector('iframe');
iframe.contentWindow.postMessage({type:d.type+':res',id:d.id,error:err.message},'*');
});return;
}
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
var res=(j&&j.result!==undefined)?j.result:j;
iframe.contentWindow.postMessage({type:d.type+':res',id:d.id,result:res},'*');
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
	if a.AuthorID != acc.ID && !acc.Admin {
		app.Forbidden(w, r, "You can only edit your own apps")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Icon        *string `json:"icon"`
		Description *string `json:"description"`
		Tags        *string `json:"tags"`
		HTML        *string `json:"html"`
		Public      *bool   `json:"public"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	mutex.Lock()
	// Snapshot current state before applying changes
	changed := false
	if req.HTML != nil && *req.HTML != a.HTML {
		changed = true
	}
	if req.Name != nil && *req.Name != "" && *req.Name != a.Name {
		changed = true
	}

	if req.Name != nil && *req.Name != "" {
		a.Name = *req.Name
	}
	if req.Icon != nil {
		a.Icon = *req.Icon
	}
	if req.Description != nil {
		a.Description = *req.Description
	}
	if req.Tags != nil {
		a.Tags = *req.Tags
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
	if changed {
		snapshotVersion(a, "")
	}
	a.UpdatedAt = time.Now()
	mutex.Unlock()

	save()

	app.RespondJSON(w, a)
}

// handleDelete deletes an app.
// DeleteApp removes an app by slug (used by admin delete)
func DeleteApp(slug string) error {
	mutex.Lock()
	defer mutex.Unlock()
	if _, ok := apps[slug]; !ok {
		return fmt.Errorf("app not found: %s", slug)
	}
	delete(apps, slug)
	save()
	return nil
}

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
	if a.AuthorID != acc.ID && !acc.Admin {
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

// defaultAppIcon is a generic app icon used when no custom icon is set.
const defaultAppIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <rect x="4" y="4" width="24" height="24" rx="4" fill="none" stroke="#555" stroke-width="2"/>
  <circle cx="16" cy="16" r="4" fill="none" stroke="#555" stroke-width="2"/>
</svg>`

// handleIcon serves the SVG icon for an app.
func handleIcon(w http.ResponseWriter, r *http.Request, slug string) {
	mutex.RLock()
	a, ok := apps[slug]
	mutex.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "App not found")
		return
	}

	icon := a.Icon
	if icon == "" || !strings.HasPrefix(strings.TrimSpace(icon), "<svg") {
		icon = defaultAppIcon
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(icon))
}

// handleSDK serves the SDK JavaScript file.
func handleSDK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(sdkJS))
}

// handleSDKAI proxies AI requests from apps.
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

// handleSDKStore handles key-value storage for apps.
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

// UpdateApp updates an existing app's fields. Only non-empty values are applied.
// Returns the updated app or an error.
func UpdateApp(slug, name, description, tags, html, icon string) (*App, error) {
	mutex.Lock()
	a, ok := apps[slug]
	if !ok {
		mutex.Unlock()
		return nil, fmt.Errorf("app not found")
	}
	changed := (html != "" && html != a.HTML) || (name != "" && name != a.Name)
	if name != "" {
		a.Name = name
	}
	if description != "" {
		a.Description = description
	}
	if tags != "" {
		a.Tags = tags
	}
	if icon != "" {
		a.Icon = icon
	}
	if html != "" {
		if len(html) > MaxHTMLSize {
			mutex.Unlock()
			return nil, fmt.Errorf("HTML content exceeds 256KB limit")
		}
		a.HTML = html
	}
	if changed {
		snapshotVersion(a, "")
	}
	a.UpdatedAt = time.Now()
	mutex.Unlock()

	save()
	return a, nil
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

// GetAppsByAuthor returns all public apps by a given author ID, sorted by name.
func GetAppsByAuthor(authorID string) []*App {
	mutex.RLock()
	defer mutex.RUnlock()

	var list []*App
	for _, a := range apps {
		if a.AuthorID == authorID && a.Public {
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
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
			hasTag(a.Tags, query) {
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

// splitTags splits a comma-separated tags string into trimmed, non-empty tags.
func splitTags(tags string) []string {
	var result []string
	for _, t := range strings.Split(tags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// hasTag returns true if the comma-separated tags string contains the given tag (case-insensitive).
func hasTag(tags, tag string) bool {
	for _, t := range splitTags(tags) {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func pillStyle(active bool) string {
	if active {
		return "background:#111;color:#fff;"
	}
	return "background:#f0f0f0;color:#333;"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SDK JavaScript served at /apps/sdk.js
const sdkJS = `// Mu App SDK
// Include this in your app: <script src="/apps/sdk.js"></script>
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

    // Send a result back to the parent (for agent code execution)
    run: function(result) {
      window.parent.postMessage({ type: 'mu:run', result: result }, '*');
    },

    // Platform API access
    api: {
      get: function(path) { return send('api', { method: 'GET', path: path }); },
      post: function(path, body) { return send('api', { method: 'POST', path: path, body: body }); }
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
