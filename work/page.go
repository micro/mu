package work

import (
	"context"
	"fmt"
	"html"
	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/tasks"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodPost {
		workAction(w, r, acc.ID)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	if id := r.URL.Query().Get("id"); id != "" {
		t, e := tasks.Get(acc.ID, id)
		if e != nil {
			app.NotFound(w, r, "No work with that ID")
			return
		}
		if r.URL.Query().Get("format") == "report" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="report.json"`)
			fmt.Fprint(w, diagnosticReport(t))
			return
		}
		body := workDetail(t, auth.CSRFToken(r))
		if app.WantsJSON(r) {
			app.RespondJSON(w, map[string]any{"html": body, "status": t.Status})
			return
		}
		app.Respond(w, r, app.Response{Title: "Work", HTML: `<div id="work-detail" class="page-stack">` + body + `</div>` + workPollJS})
		return
	}
	if r.URL.Query().Get("view") == "new" {
		options := []app.Option{{Value: "", Label: "Micro", On: true}, {Value: "me", Label: "Me"}}
		for _, a := range agent.Agents(acc.ID) {
			options = append(options, app.Option{Value: agent.Slug(a), Label: a.Name})
		}
		body := `<form method="POST" action="/work" class="form">` + app.CSRFField(auth.CSRFToken(r)) + app.Field{Name: "prompt", Label: "What needs doing?", Rows: 4, Max: 8000, Required: true, Wide: true}.HTML() + app.Field{Name: "agent", Label: "Assigned to", Options: options}.HTML() + `<div class="form-actions"><button>Save</button><a href="/work">Cancel</a></div></form>`
		app.Respond(w, r, app.Response{Title: "New work", HTML: body})
		return
	}
	filter := r.URL.Query().Get("status")
	archived := r.URL.Query().Get("view") == "archived"
	var b strings.Builder
	b.WriteString(`<div class="form-actions"><a class="btn" href="/work?view=new">New</a></div><nav class="view-switch" aria-label="Work status">`)
	for _, f := range []struct{ value, label string }{{"", "All"}, {"todo", "Queued"}, {"doing", "Running"}, {"blocked", "Needs input"}, {"failed", "Failed"}, {"done", "Done"}, {"canceled", "Stopped"}} {
		current := ""
		if filter == f.value && !archived {
			current = ` aria-current="page"`
		}
		b.WriteString(`<a href="/work?status=` + f.value + `"` + current + `>` + f.label + `</a>`)
	}
	b.WriteString(`<a href="/work?view=archived">Archived</a></nav>`)
	rows := tasks.List(acc.ID, filter)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Created.After(rows[j].Created) })
	filtered := rows[:0:0]
	for _, t := range rows {
		if t.Archived == archived {
			filtered = append(filtered, t)
		}
	}
	pager := app.Paginate(r, len(filtered), 25)
	for _, t := range filtered[pager.From:pager.To] {
		b.WriteString(`<article class="list-row"><a href="/work?id=` + url.QueryEscape(t.ID) + `">` + html.EscapeString(t.Title) + `</a><div class="metadata">` + badge(t.Status) + `<span>` + html.EscapeString(assignee(t)) + `</span><time datetime="` + t.Created.Format(time.RFC3339) + `" title="` + t.Created.Format(time.RFC1123) + `">Created ` + app.TimeAgo(t.Created) + `</time><span>Updated ` + app.TimeAgo(t.Updated) + `</span></div></article>`)
	}
	b.WriteString(pager.Nav("/work?status=" + url.QueryEscape(filter) + "&view=" + url.QueryEscape(r.URL.Query().Get("view"))))
	if len(filtered) == 0 {
		b.WriteString(`<p>No work here.</p>`)
	}
	app.Respond(w, r, app.Response{Title: "Work", HTML: b.String()})
}

func workAction(w http.ResponseWriter, r *http.Request, owner string) {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Invalid form")
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	action, id := r.PostForm.Get("action"), r.PostForm.Get("id")
	var err error
	if id != "" {
		if _, err = tasks.Get(owner, id); err != nil {
			app.NotFound(w, r, "No work with that ID")
			return
		}
	}
	switch action {
	case "stop":
		err = tasks.Stop(owner, id)
		if err == nil {
			if cancel, ok := activeRuns.Load(owner + ":" + id); ok {
				cancel.(context.CancelFunc)()
			}
		}
	case "archive", "restore":
		err = tasks.ArchiveTask(owner, id, action == "archive")
	case "delete":
		err = tasks.Remove(owner, id)
		if err == nil {
			id = ""
		}
	case "edit":
		t, _ := tasks.Get(owner, id)
		prompt := strings.TrimSpace(r.PostForm.Get("prompt"))
		if t == nil || t.Status == tasks.StatusDoing || prompt == "" || len(prompt) > 8000 {
			err = fmt.Errorf("stop work before editing, and enter a goal of up to 8000 bytes")
		} else {
			_, err = tasks.Update(owner, id, prompt, "", "", "", "")
		}
	case "done":
		t, _ := tasks.Get(owner, id)
		if t == nil || t.Assignee != tasks.Me {
			err = fmt.Errorf("only tasks assigned to you can be marked done manually")
		} else {
			_, err = tasks.Update(owner, id, "", "", tasks.StatusDone, "", "")
		}
	case "retry":
		if _, running := activeRuns.Load(owner + ":" + id); running {
			err = fmt.Errorf("the previous run is still stopping; try again shortly")
			break
		}
		t, _ := tasks.Get(owner, id)
		if t == nil || (t.Status != tasks.StatusFailed && t.Status != tasks.StatusBlocked && t.Status != tasks.StatusCanceled) {
			err = fmt.Errorf("only failed, blocked or stopped work can be retried")
		} else if err = workCredits(owner); err == nil {
			err = tasks.Run(owner, id)
		}
	case "":
		prompt := strings.TrimSpace(r.PostForm.Get("prompt"))
		if prompt == "" || len(prompt) > 8000 {
			err = fmt.Errorf("enter a goal of up to 8000 bytes")
			break
		}
		if r.PostForm.Get("agent") == "me" {
			var t *tasks.Task
			t, err = tasks.Create(owner, prompt, "", tasks.Me, time.Time{})
			if t != nil {
				id = t.ID
			}
		} else {
			result, callErr := api.Call(r, "work_submit", map[string]any{"prompt": prompt, "agent": r.PostForm.Get("agent")})
			err = callErr
			if m, ok := result.(map[string]any); ok {
				id, _ = m["id"].(string)
			}
		}
	default:
		err = fmt.Errorf("unknown action")
	}
	if err != nil {
		if e, ok := err.(*api.Failure); ok {
			app.Error(w, r, e.Status, e.Message)
		} else {
			app.BadRequest(w, r, err.Error())
		}
		return
	}
	dest := "/work"
	if id != "" {
		dest += "?id=" + url.QueryEscape(id)
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
func assignee(t *tasks.Task) string {
	if t.Assignee == tasks.Me {
		return "Me"
	}
	return agent.NameOf(t.Owner, t.Agent)
}
func badge(status string) string {
	return `<span class="status-badge status-` + html.EscapeString(status) + `">` + state(status) + `</span>`
}
func state(status string) string {
	switch status {
	case tasks.StatusTodo:
		return "Queued"
	case tasks.StatusDoing:
		return "Running"
	case tasks.StatusBlocked:
		return "Needs input"
	case tasks.StatusFailed:
		return "Failed"
	case tasks.StatusDone:
		return "Done"
	case tasks.StatusCanceled:
		return "Stopped"
	}
	return status
}
func workDetail(t *tasks.Task, csrf string) string {
	var b strings.Builder
	b.WriteString(`<div class="section-actions"><a href="/work">Work</a>`)
	if t.Thread != "" {
		b.WriteString(`<a href="/inbox?id=` + url.QueryEscape(t.Thread) + `">Conversation</a>`)
	}
	b.WriteString(`</div><h2>` + html.EscapeString(t.Title) + `</h2><div class="metadata">` + badge(t.Status) + `<span>` + html.EscapeString(assignee(t)) + `</span><span>Updated ` + app.TimeAgo(t.Updated) + `</span></div>`)
	if t.Result != "" {
		b.WriteString(`<section><h3>Outcome</h3>` + app.RenderString(t.Result) + `</section>`)
	}
	if t.Status == tasks.StatusDoing {
		b.WriteString(`<p role="status">Running · last update ` + t.Updated.Format("15:04:05 UTC") + `</p>`)
	}
	b.WriteString(`<section><h3>Activity</h3>` + stepHTML(t.Steps) + `</section>`)
	form := func(action, label string) {
		b.WriteString(`<form class="form-action" method="POST" action="/work">` + app.CSRFField(csrf) + `<input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `"><input type="hidden" name="action" value="` + action + `"><button>` + label + `</button></form>`)
	}
	b.WriteString(`<div class="form-actions">`)
	if t.Status == tasks.StatusDoing {
		form("stop", "Stop")
	} else {
		if t.Status == tasks.StatusFailed || t.Status == tasks.StatusBlocked || t.Status == tasks.StatusCanceled {
			form("retry", "Retry")
		}
		if t.Assignee == tasks.Me && t.Status != tasks.StatusDone {
			form("done", "Mark done")
		}
		if t.Archived {
			form("restore", "Restore")
		} else {
			form("archive", "Archive")
		}
	}
	b.WriteString(`</div>`)
	if t.Status != tasks.StatusDoing {
		b.WriteString(`<details><summary>Edit instructions</summary><form method="POST" action="/work" class="form">` + app.CSRFField(csrf) + `<input type="hidden" name="action" value="edit"><input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `">` + app.Field{Name: "prompt", Label: "Goal", Value: t.Title, Rows: 4, Max: 8000, Required: true}.HTML() + `<button>Save</button></form></details>`)
		b.WriteString(`<details><summary>Delete work</summary><p>Deletes this record and its attempt history. Completed external actions remain.</p>`)
		form("delete", "Delete")
		b.WriteString(`</details>`)
	}
	if t.Status == tasks.StatusFailed || t.Status == tasks.StatusBlocked || t.Status == tasks.StatusCanceled {
		b.WriteString(`<p class="text-muted">Review completed actions before retrying; they may be repeated.</p>`)
	}
	for i, a := range t.Attempts {
		b.WriteString(`<details><summary>Attempt ` + fmt.Sprint(i+1) + ` · ` + state(a.Status) + ` · ` + a.Started.Format("02 Jan 15:04 UTC") + `</summary><p class="text-muted">Run ` + html.EscapeString(a.ID) + ` · Version ` + html.EscapeString(a.Version) + `</p>` + stepHTML(a.Steps))
		if a.Error != "" {
			b.WriteString(`<h3>Error</h3><pre>` + html.EscapeString(a.Error) + `</pre>`)
		}
		if a.Report != "" {
			b.WriteString(`<h3>Final report</h3><pre>` + html.EscapeString(a.Report) + `</pre>`)
		}
		b.WriteString(`</details>`)
	}
	b.WriteString(`<details><summary>Diagnostic report</summary><p class="text-muted">Work ID: ` + html.EscapeString(t.ID) + `. Review the report for private content before sharing.</p><textarea id="report" class="report" readonly aria-label="Diagnostic report">` + html.EscapeString(diagnosticReport(t)) + `</textarea><div class="form-actions"><button type="button" onclick="navigator.clipboard.writeText(document.getElementById('report').value).then(function(){this.textContent='Copied'}.bind(this))">Copy report</button><a href="/work?id=` + url.QueryEscape(t.ID) + `&format=report">Download report</a><a href="/contact#support">Contact support</a></div></details>`)
	return b.String()
}
func stepHTML(steps []tasks.Step) string {
	if len(steps) == 0 {
		return `<p class="text-muted">No tool calls recorded yet.</p>`
	}
	var b strings.Builder
	for _, s := range steps {
		status := s.Status
		if status == "" {
			status = "done"
			if !s.OK {
				status = "failed"
			}
		}
		b.WriteString(`<details class="activity-step"><summary>` + html.EscapeString(s.Tool) + ` ` + badge(status) + fmt.Sprintf(` <span class="text-muted">%.1fs</span>`, s.Seconds) + `</summary>`)
		if !s.Started.IsZero() {
			b.WriteString(`<p>` + s.Started.Format("15:04:05 UTC") + `</p>`)
		}
		if s.Args != "" {
			b.WriteString(`<h3>Input</h3><pre>` + html.EscapeString(s.Args) + `</pre>`)
		} else if s.Detail != "" {
			b.WriteString(`<p>` + html.EscapeString(s.Detail) + `</p>`)
		}
		if s.Output != "" {
			b.WriteString(`<h3>Result</h3><pre>` + html.EscapeString(s.Output) + `</pre>`)
		}
		if s.Error != "" && s.Error != s.Output {
			b.WriteString(`<h3>Error</h3><pre>` + html.EscapeString(s.Error) + `</pre>`)
		}
		b.WriteString(`</details>`)
	}
	return b.String()
}

const workPollJS = `<script>(function(){var root=document.getElementById('work-detail');var count=0;async function poll(){if(!root||!root.isConnected||++count>360)return;try{var r=await fetch(location.pathname+location.search,{headers:{Accept:'application/json'}});if(!r.ok)return;var d=await r.json();var rendered=!root.contains(document.activeElement)&&!root.querySelector('details[open]');if(rendered)root.innerHTML=d.html;if(rendered&&d.status!=='doing')return;}catch(e){}setTimeout(poll,3000);}setTimeout(poll,3000);})();</script>`
