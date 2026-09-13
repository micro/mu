package work

// Work is the owner's view of delegated commitments. The tasks service stores
// their state; this package runs them and presents their progress and outcomes.
import (
	"errors"
	"html"
	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/tasks"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 32768)
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Invalid form")
			return
		}
		name := "work_submit"
		args := map[string]any{"prompt": r.PostForm.Get("prompt"), "agent": r.PostForm.Get("agent")}
		if r.PostForm.Get("action") == "retry" {
			name = "work_retry"
			args = map[string]any{"id": r.PostForm.Get("id")}
		}
		result, err := api.Call(r, name, args)
		if err != nil {
			var e *api.Failure
			if errors.As(err, &e) {
				app.Error(w, r, e.Status, e.Message)
			} else {
				app.ServerError(w, r, "Could not save work. Check the list before retrying.")
			}
			return
		}
		row, ok := result.(map[string]any)
		if !ok {
			app.ServerError(w, r, "Could not read the work result")
			return
		}
		id, _ := row["id"].(string)
		http.Redirect(w, r, "/work?id="+url.QueryEscape(id), http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		app.Error(w, r, 405, "Method not allowed")
		return
	}
	var b strings.Builder
	b.WriteString(app.Note("Work is what you have asked Micro to take on. Follow progress and outcomes here; Inbox carries messages and updates."))
	if id := r.URL.Query().Get("id"); id != "" {
		t, err := ownedWork(acc.ID, id)
		if err != nil {
			app.NotFound(w, r, "No work with that ID")
			return
		}
		b.WriteString(`<div class="section-actions"><a href="/work">Back to Work</a>`)
		if t.Thread != "" {
			b.WriteString(`<a href="/inbox?id=` + url.QueryEscape(t.Thread) + `">Conversation &rarr;</a>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(app.Section("Goal", `<p>`+html.EscapeString(t.Title)+`</p>`, app.Note(state(t.Status)+" · "+agent.NameOf(acc.ID, t.Agent))))
		if t.Result != "" {
			b.WriteString(app.Section("Outcome", string(app.Render([]byte(t.Result)))))
		} else {
			b.WriteString(app.Note("No outcome yet. Refresh to check progress."))
		}
		if len(t.Steps) > 0 {
			var steps strings.Builder
			steps.WriteString(`<ol>`)
			for _, s := range t.Steps {
				label := "Completed"
				if !s.OK {
					label = "Failed"
				}
				steps.WriteString(`<li>` + html.EscapeString(s.Tool) + " — " + label + `</li>`)
			}
			steps.WriteString(`</ol>`)
			b.WriteString(app.Section("Activity", steps.String()))
		}
		if t.Status == tasks.StatusFailed || t.Status == tasks.StatusBlocked {
			b.WriteString(app.Note("Review the outcome before retrying. Any earlier external actions may be repeated."))
			b.WriteString(`<form method="POST" action="/work" class="form"><input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `"><input type="hidden" name="action" value="retry"><input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `"><div class="form-actions"><button type="submit">Retry</button></div></form>`)
		}
	} else if r.URL.Query().Get("view") == "new" {
		options := []app.Option{{Value: "", Label: "Micro", On: true}}
		for _, a := range agent.Agents(acc.ID) {
			options = append(options, app.Option{Value: agent.Slug(a), Label: a.Name})
		}
		b.WriteString(`<a href="/work">Back to Work</a>`)
		b.WriteString(app.Section("New work", `<form method="POST" action="/work" class="form"><input type="hidden" name="_csrf" value="`+html.EscapeString(auth.CSRFToken(r))+`">`+app.Field{Name: "prompt", Label: "What would you like done?", Rows: 4, Max: 8000, Required: true, Wide: true}.HTML()+app.Field{Name: "agent", Label: "Agent", Options: options}.HTML()+`<div class="form-actions"><button type="submit">Start</button></div></form>`))
	} else {
		b.WriteString(`<div class="section-actions"><a href="/work?view=new">New</a><a href="/tasks">Personal tasks &rarr;</a></div>`)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}
		all := tasks.List(acc.ID, "")
		var jobs []*tasks.Task
		for _, t := range all {
			if t.Assignee == tasks.Agent {
				jobs = append(jobs, t)
			}
		}
		if len(jobs) == 0 {
			b.WriteString(app.Section("Nothing underway", app.Note("Start work with a clear goal. Micro will run it in the background and keep the outcome here.")))
		}
		if offset > len(jobs) {
			offset = len(jobs)
		}
		end := offset + 20
		if end > len(jobs) {
			end = len(jobs)
		}
		if offset > len(jobs) {
			offset = len(jobs)
		}
		b.WriteString(`<div class="page-stack">`)
		for _, t := range jobs[offset:end] {
			title := []rune(t.Title)
			if len(title) > 100 {
				title = append(title[:100], '…')
			}
			body := `<a href="/work?id=` + url.QueryEscape(t.ID) + `">` + html.EscapeString(string(title)) + `</a>`
			body += `<p class="card-meta">` + html.EscapeString(state(t.Status)+" · "+agent.NameOf(acc.ID, t.Agent)) + `</p>`
			b.WriteString(app.Section("", body))
		}
		b.WriteString(`</div><div class="section-actions">`)
		if offset > 0 {
			prev := offset - 20
			if prev < 0 {
				prev = 0
			}
			b.WriteString(`<a href="/work?offset=` + strconv.Itoa(prev) + `">Previous</a>`)
		}
		if end < len(jobs) {
			b.WriteString(`<a href="/work?offset=` + strconv.Itoa(end) + `">More &rarr;</a>`)
		}
		b.WriteString(`</div>`)
	}
	app.Respond(w, r, app.Response{Title: "Work", HTML: `<div class="page-stack">` + b.String() + `</div>`})
}
func state(status string) string {
	switch status {
	case tasks.StatusTodo:
		return "Queued"
	case tasks.StatusDoing:
		return "Running"
	case tasks.StatusBlocked:
		return "Blocked"
	case tasks.StatusFailed:
		return "Failed"
	case tasks.StatusDone:
		return "Done"
	}
	return status
}
