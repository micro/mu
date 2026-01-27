package flow

import (
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
)

// Handler serves /flows routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/flows")
	path = strings.TrimPrefix(path, "/")

	// Must be logged in
	_, user := auth.TrySession(r)
	if user == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusFound)
		return
	}

	// JSON response
	if app.WantsJSON(r) {
		handleJSON(w, r, path, user.ID)
		return
	}

	// HTML response
	handleHTML(w, r, path, user.ID)
}

func handleJSON(w http.ResponseWriter, r *http.Request, path, userID string) {
	switch r.Method {
	case "GET":
		if path == "" {
			// List flows
			flows := ListByUser(userID)
			app.RespondJSON(w, map[string]any{"flows": flows})
			return
		}
		// Get specific flow
		f := Get(path)
		if f == nil || f.UserID != userID {
			app.RespondError(w, http.StatusNotFound, "flow not found")
			return
		}
		app.RespondJSON(w, f)

	case "POST":
		if path == "" {
			// Create flow
			name := r.FormValue("name")
			source := r.FormValue("source")
			f, err := Create(userID, name, source)
			if err != nil {
				app.RespondError(w, http.StatusBadRequest, err.Error())
				return
			}
			app.RespondJSON(w, f)
			return
		}

		// Run flow
		if strings.HasSuffix(path, "/run") {
			id := strings.TrimSuffix(path, "/run")
			f := Get(id)
			if f == nil || f.UserID != userID {
				app.RespondError(w, http.StatusNotFound, "flow not found")
				return
			}
			result := Execute(f, userID)
			app.RespondJSON(w, result)
			return
		}

	case "DELETE":
		f := Get(path)
		if f == nil || f.UserID != userID {
			app.RespondError(w, http.StatusNotFound, "flow not found")
			return
		}
		if err := f.Delete(); err != nil {
			app.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		app.RespondJSON(w, map[string]any{"success": true})

	default:
		app.RespondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handleHTML(w http.ResponseWriter, r *http.Request, path, userID string) {
	if path != "" {
		// Single flow view
		f := Get(path)
		if f == nil || f.UserID != userID {
			app.NotFound(w, r, "Flow not found")
			return
		}
		renderFlowDetail(w, r, f)
		return
	}

	// Handle form submissions
	if r.Method == "POST" {
		action := r.FormValue("action")
		switch action {
		case "create":
			name := r.FormValue("name")
			source := r.FormValue("source")
			_, err := Create(userID, name, source)
			if err != nil {
				// Show error
				renderFlowList(w, userID, err.Error())
				return
			}
		case "delete":
			id := r.FormValue("id")
			if f := Get(id); f != nil && f.UserID == userID {
				f.Delete()
			}
		case "run":
			id := r.FormValue("id")
			if f := Get(id); f != nil && f.UserID == userID {
				Execute(f, userID)
			}
		case "toggle":
			id := r.FormValue("id")
			if f := Get(id); f != nil && f.UserID == userID {
				f.Enabled = !f.Enabled
				f.Save()
			}
		}
		http.Redirect(w, r, "/flows", http.StatusFound)
		return
	}

	// List flows
	renderFlowList(w, userID, "")
}

func renderFlowList(w http.ResponseWriter, userID string, errorMsg string) {
	flows := ListByUser(userID)

	var b strings.Builder

	// Error message if any
	if errorMsg != "" {
		b.WriteString(`<div class="alert-error mb-4">`)
		b.WriteString(errorMsg)
		b.WriteString(`</div>`)
	}

	b.WriteString(`<p class="text-muted mb-4">Automations that run on schedule or on demand.</p>`)

	// Templates section
	templates := GetTemplates()
	if len(templates) > 0 {
		b.WriteString(`<details class="mb-4"><summary class="btn btn-secondary">📝 Templates</summary>`)
		b.WriteString(`<div class="mt-2 card-list">`)
		for _, t := range templates {
			b.WriteString(`<div class="card">`)
			b.WriteString(`<div class="card-title">` + t.Name + `</div>`)
			b.WriteString(`<div class="card-desc">` + t.Description + `</div>`)
			b.WriteString(`<pre class="mt-2" style="background: #f5f5f5; padding: 0.5rem; border-radius: 4px; font-size: 0.85em; overflow-x: auto;">` + escapeHTML(t.Source) + `</pre>`)
			b.WriteString(`<form method="POST" class="mt-2">`)
			b.WriteString(`<input type="hidden" name="action" value="create">`)
			b.WriteString(`<input type="hidden" name="name" value="` + t.Name + `">`)
			b.WriteString(`<input type="hidden" name="source" value="` + escapeHTML(t.Source) + `">`)
			b.WriteString(`<button type="submit" class="btn btn-sm">Use Template</button>`)
			b.WriteString(`</form></div>`)
		}
		b.WriteString(`</div></details>`)
	}

	// Create form
	b.WriteString(`
<details class="mb-4">
<summary class="btn">+ New Flow</summary>
<form method="POST" class="mt-2 p-3 bg-light" style="border-radius: 8px;">
<input type="hidden" name="action" value="create">
<div class="mb-2">
<label>Name</label>
<input type="text" name="name" placeholder="Morning Briefing" required style="width: 100%;">
</div>
<div class="mb-2">
<label>Flow</label>
<textarea name="source" rows="6" placeholder="every day at 7am:
    get reminder
    then search news for &quot;tech&quot;
    then email to me" required style="width: 100%; font-family: monospace;"></textarea>
</div>
<button type="submit" class="btn">Create Flow</button>
</form>
</details>
`)

	if len(flows) == 0 {
		b.WriteString(app.Empty("No flows yet. Create one to automate tasks."))
	} else {
		b.WriteString(`<div class="card-list">`)
		for _, f := range flows {
			b.WriteString(`<div class="card">`)
			b.WriteString(`<div class="d-flex justify-between items-center">`)
			b.WriteString(`<a href="/flows/` + f.ID + `" class="card-title">` + f.Name + `</a>`)

			// Status badge
			if f.Enabled {
				b.WriteString(`<span class="tag" style="background: #e8f5e9; color: #2e7d32;">Active</span>`)
			} else {
				b.WriteString(`<span class="tag" style="background: #f5f5f5; color: #757575;">Paused</span>`)
			}
			b.WriteString(`</div>`)

			// Schedule
			if f.Schedule != "" {
				b.WriteString(`<div class="card-meta">⏰ ` + f.Schedule + `</div>`)
			} else {
				b.WriteString(`<div class="card-meta">Manual trigger</div>`)
			}

			// Stats
			b.WriteString(`<div class="card-meta">`)
			if f.RunCount > 0 {
				b.WriteString(`Runs: ` + itoa(f.RunCount))
				if !f.LastRun.IsZero() {
					b.WriteString(` · Last: ` + app.TimeAgo(f.LastRun))
				}
			} else {
				b.WriteString(`Never run`)
			}
			b.WriteString(`</div>`)

			if f.LastError != "" {
				b.WriteString(`<div class="text-error text-sm">Last error: ` + f.LastError + `</div>`)
			}

			// Actions
			b.WriteString(`<div class="mt-2 d-flex gap-2">`)
			b.WriteString(`<form method="POST" style="display:inline;"><input type="hidden" name="action" value="run"><input type="hidden" name="id" value="` + f.ID + `"><button type="submit" class="btn btn-sm">▶ Run</button></form>`)
			b.WriteString(`<form method="POST" style="display:inline;"><input type="hidden" name="action" value="toggle"><input type="hidden" name="id" value="` + f.ID + `"><button type="submit" class="btn btn-sm btn-secondary">`)
			if f.Enabled {
				b.WriteString(`Pause`)
			} else {
				b.WriteString(`Enable`)
			}
			b.WriteString(`</button></form>`)
			b.WriteString(`</div>`)

			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}

	html := app.RenderHTML("Flows", "Automations", b.String())
	w.Write([]byte(html))
}

func renderFlowDetail(w http.ResponseWriter, r *http.Request, f *Flow) {
	var b strings.Builder

	// Back link
	b.WriteString(`<p><a href="/flows">← Back to Flows</a></p>`)

	// Status
	b.WriteString(`<div class="mb-4">`)
	if f.Enabled {
		b.WriteString(`<span class="tag" style="background: #e8f5e9; color: #2e7d32;">Active</span>`)
	} else {
		b.WriteString(`<span class="tag" style="background: #f5f5f5; color: #757575;">Paused</span>`)
	}
	if f.Schedule != "" {
		b.WriteString(` <span class="text-muted">⏰ ` + f.Schedule + `</span>`)
	}
	b.WriteString(`</div>`)

	// Source code
	b.WriteString(`<h3>Source</h3>`)
	b.WriteString(`<pre style="background: #f5f5f5; padding: 1rem; border-radius: 8px; overflow-x: auto;">` + f.Source + `</pre>`)

	// Parsed view
	parsed, err := Parse(f.Source)
	if err == nil {
		b.WriteString(`<h3>Steps</h3>`)
		b.WriteString(`<ol>`)
		for _, step := range parsed.Steps {
			b.WriteString(`<li><code>` + step.Tool + `</code>`)
			if len(step.Args) > 0 {
				b.WriteString(` (`)
				first := true
				for k, v := range step.Args {
					if !first {
						b.WriteString(`, `)
					}
					b.WriteString(k + `="` + v + `"`)
					first = false
				}
				b.WriteString(`)`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ol>`)
	}

	// Stats
	b.WriteString(`<h3>Stats</h3>`)
	b.WriteString(`<p>Run count: ` + itoa(f.RunCount) + `</p>`)
	if !f.LastRun.IsZero() {
		b.WriteString(`<p>Last run: ` + app.TimeAgo(f.LastRun) + `</p>`)
	}
	if f.LastError != "" {
		b.WriteString(`<p class="text-error">Last error: ` + f.LastError + `</p>`)
	}

	// History
	if len(f.History) > 0 {
		b.WriteString(`<h3>Recent Runs</h3>`)
		b.WriteString(`<table class="data-table"><thead><tr><th>Time</th><th>Status</th><th>Duration</th></tr></thead><tbody>`)
		// Show most recent first
		for i := len(f.History) - 1; i >= 0; i-- {
			h := f.History[i]
			status := `<span style="color: green;">✓</span>`
			if !h.Success {
				status = `<span style="color: red;">✗</span>`
			}
			b.WriteString(`<tr><td>` + app.TimeAgo(h.Time) + `</td><td>` + status + `</td><td>` + h.Duration + `</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}

	// Actions
	b.WriteString(`<div class="mt-4 d-flex gap-2">`)
	b.WriteString(`<form method="POST" action="/flows"><input type="hidden" name="action" value="run"><input type="hidden" name="id" value="` + f.ID + `"><button type="submit" class="btn">▶ Run Now</button></form>`)
	b.WriteString(`<form method="POST" action="/flows"><input type="hidden" name="action" value="toggle"><input type="hidden" name="id" value="` + f.ID + `"><button type="submit" class="btn btn-secondary">`)
	if f.Enabled {
		b.WriteString(`Pause`)
	} else {
		b.WriteString(`Enable`)
	}
	b.WriteString(`</button></form>`)
	b.WriteString(`<form method="POST" action="/flows" onsubmit="return confirm('Delete this flow?');"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="` + f.ID + `"><button type="submit" class="btn btn-danger">Delete</button></form>`)
	b.WriteString(`</div>`)

	html := app.RenderHTML(f.Name, "Flow details", b.String())
	w.Write([]byte(html))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
