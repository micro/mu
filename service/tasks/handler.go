package tasks

// The list, for the person whose list it is.
//
// Everything the tools do, the page does — add, hand to the agent, run, finish,
// remove — through the same functions, so the two cannot drift. The page is
// also where the agent's work comes back: a result written by a run at 6am is
// still on the task at lunchtime, which is the difference between an agent that
// answers and an agent that reports.

import (
	"fmt"
	"html"
	"net/http"
	neturl "net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves /tasks.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/tasks"), "/")
		id, action, _ := strings.Cut(rest, "/")
		handleAction(w, r, id, action)
		return
	}
	if app.WantsJSON(r) {
		handleJSON(w, r)
		return
	}
	listPage(w, r)
}

func handleJSON(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	app.RespondJSON(w, map[string]any{
		"tasks": List(sess.Account, strings.TrimSpace(r.URL.Query().Get("status"))),
	})
}

// handleAction adds, advances, runs or removes a task and returns to the page.
func handleAction(w http.ResponseWriter, r *http.Request, id, action string) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !auth.ValidCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}

	var actErr error
	switch {
	case id == "":
		due, err := ParseDue(strings.TrimSpace(r.FormValue("due")))
		if err != nil {
			actErr = err
			break
		}
		assignee := Me
		if r.FormValue("assign") == "agent" {
			assignee = Agent
		}
		_, actErr = Create(sess.Account, r.FormValue("title"), r.FormValue("detail"), assignee, due)

	case action == "done":
		_, actErr = Update(sess.Account, id, "", "", StatusDone, "", "")
	case action == "reopen":
		_, actErr = Update(sess.Account, id, "", "", StatusTodo, "", "")
	case action == "assign":
		_, actErr = Update(sess.Account, id, "", "", "", Agent, "")
	case action == "unassign":
		_, actErr = Update(sess.Account, id, "", "", "", Me, "")
	case action == "run":
		actErr = Run(sess.Account, id)
	case action == "delete":
		actErr = Remove(sess.Account, id)
	default:
		app.NotFound(w, r, "Unknown action")
		return
	}

	dest := "/tasks"
	if actErr != nil {
		dest += "?error=" + neturl.QueryEscape(actErr.Error())
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func listPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	auth.SetCSRFCookie(w, r)
	csrf := auth.CSRFToken(r)
	filter := strings.TrimSpace(r.URL.Query().Get("status"))
	list := List(sess.Account, filter)

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<p class="text-sm text-muted">What is to be done. Assign a task to the agent ` +
		`and it can pick it up — or press Run and it starts now.</p>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<p class="text-error">` + html.EscapeString(msg) + `</p>`)
	}

	fmt.Fprintf(&b, `<form method="POST" action="/tasks" class="task-add">
  <input type="hidden" name="_csrf" value="%s">
  <input name="title" placeholder="What needs doing?" required>
  <input name="detail" placeholder="Detail (optional)">
  <input name="due" placeholder="Due (optional, e.g. 2026-08-09 09:00)">
  <label class="task-assign"><input type="checkbox" name="assign" value="agent"> Give it to the agent</label>
  <button type="submit">Add</button>
</form>`, html.EscapeString(csrf))
	b.WriteString(`</div>`)

	// Filters. Counted, because "3 open" is the thing you want to know before
	// you decide whether to look.
	all := List(sess.Account, "")
	open, doing := 0, 0
	for _, t := range all {
		if t.Status == StatusTodo {
			open++
		}
		if t.Status == StatusDoing {
			doing++
		}
	}
	b.WriteString(`<div class="card"><div class="task-tabs">`)
	tab(&b, "", filter, fmt.Sprintf("All (%d)", len(all)))
	tab(&b, StatusTodo, filter, fmt.Sprintf("To do (%d)", open))
	tab(&b, StatusDoing, filter, fmt.Sprintf("Doing (%d)", doing))
	tab(&b, StatusDone, filter, "Done")
	b.WriteString(`</div>`)

	if len(list) == 0 {
		b.WriteString(`<p class="text-sm text-muted">Nothing here. Add something above, ` +
			`or an agent connected over <a href="/mcp">MCP</a> can with <code>tasks_create</code>.</p>`)
	}

	for _, t := range list {
		b.WriteString(taskRow(t, csrf))
	}
	b.WriteString(`</div>`)

	b.WriteString(tasksPageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Tasks", "What is to be done", b.String(), r)))
}

func tab(b *strings.Builder, status, active, label string) {
	href := "/tasks"
	if status != "" {
		href += "?status=" + status
	}
	class := "task-tab"
	if status == active {
		class += " active"
	}
	fmt.Fprintf(b, `<a href="%s" class="%s">%s</a>`, href, class, html.EscapeString(label))
}

func taskRow(t *Task, csrf string) string {
	var b strings.Builder
	class := "task"
	if !t.Open() {
		class += " task-done"
	}
	fmt.Fprintf(&b, `<div class="%s">`, class)

	fmt.Fprintf(&b, `<div class="task-title">%s</div>`, html.EscapeString(t.Title))

	var meta []string
	meta = append(meta, html.EscapeString(t.Status))
	if t.Assignee == Agent {
		meta = append(meta, "agent")
	}
	if !t.Due.IsZero() {
		meta = append(meta, "due "+html.EscapeString(t.Due.Local().Format("2 Jan 15:04")))
	}
	if Running(t.ID) {
		meta = append(meta, "running now")
	}
	fmt.Fprintf(&b, `<div class="task-meta">%s</div>`, strings.Join(meta, " · "))

	if t.Detail != "" {
		fmt.Fprintf(&b, `<div class="task-detail">%s</div>`, html.EscapeString(t.Detail))
	}
	if t.Result != "" {
		// The agent's answer, kept where the work was asked for.
		fmt.Fprintf(&b, `<div class="task-result">%s</div>`, html.EscapeString(t.Result))
	}

	b.WriteString(`<div class="task-actions">`)
	if t.Open() {
		button(&b, t.ID, "done", csrf, "Done", "")
		if t.Assignee == Agent {
			button(&b, t.ID, "run", csrf, "Run now", "")
			button(&b, t.ID, "unassign", csrf, "Take back", "")
		} else {
			button(&b, t.ID, "assign", csrf, "Give to agent", "")
		}
	} else {
		button(&b, t.ID, "reopen", csrf, "Reopen", "")
	}
	fmt.Fprintf(&b, `<form method="POST" action="/tasks/%s/delete" onsubmit="return confirm('Delete %s?')">
  <input type="hidden" name="_csrf" value="%s">
  <button type="submit" class="link-button danger">Delete</button>
</form>`, html.EscapeString(t.ID),
		html.EscapeString(strings.ReplaceAll(t.Title, "'", "\\'")), html.EscapeString(csrf))
	b.WriteString(`</div></div>`)
	return b.String()
}

func button(b *strings.Builder, id, action, csrf, label, extra string) {
	fmt.Fprintf(b, `<form method="POST" action="/tasks/%s/%s">
  <input type="hidden" name="_csrf" value="%s">%s
  <button type="submit" class="link-button">%s</button>
</form>`, html.EscapeString(id), action, html.EscapeString(csrf), extra, html.EscapeString(label))
}

const tasksPageCSS = `<style>
.task-add{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:8px;align-items:center;margin:10px 0 4px}
.task-add input[type=text],.task-add input:not([type]){min-width:0}
.task-assign{font-size:13px;color:var(--text-muted);display:flex;align-items:center;justify-content:flex-start;gap:6px;white-space:nowrap}
.task-tabs{display:flex;gap:14px;flex-wrap:wrap;margin-bottom:12px}
.task-tab{font-size:13px;color:var(--text-muted);text-decoration:none}
.task-tab.active{color:var(--text-primary);font-weight:600}
.task{padding:12px 0;border-bottom:1px solid var(--divider)}
.task:last-child{border-bottom:none}
.task-title{font-weight:var(--font-weight-medium)}
.task-done .task-title{text-decoration:line-through;color:var(--text-muted)}
.task-meta{font-size:12px;color:var(--text-muted);margin-top:2px}
.task-detail{font-size:14px;margin-top:6px;color:var(--text-secondary)}
.task-result{font-size:14px;margin-top:8px;padding:8px 10px;background:var(--hover-background);border-radius:6px;white-space:pre-wrap}
.task-actions{margin-top:8px}
.task-actions form{display:inline}
@media only screen and (max-width:600px){
  .task-add{grid-template-columns:1fr}
  .task-add button{width:100%}
  .task-actions .link-button{padding:6px 14px 6px 0;font-size:14px}
}
</style>`
