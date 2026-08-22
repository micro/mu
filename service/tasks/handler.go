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

	b.WriteString(addForm(csrf))
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

	running := 0
	for _, t := range list {
		if Running(t) {
			running++
		}
		b.WriteString(taskRow(t, csrf))
	}
	b.WriteString(`</div>`)

	// A run takes seconds to a minute and the page was static, so the only way
	// to find out it had finished was to reload and guess. While anything is
	// with the agent, watch for it to land and refresh once it does — polling
	// the list this page already serves as JSON rather than reloading on a
	// timer, so nothing is thrown away mid-typing.
	if running > 0 {
		b.WriteString(taskPollJS)
	}

	b.WriteString(tasksPageCSS)
	app.Respond(w, r, app.Response{Title: "Tasks", Description: "What is to be done", HTML: b.String()})
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
	if Running(t) {
		meta = append(meta, `<span class="task-running">working…</span>`)
	}
	fmt.Fprintf(&b, `<div class="task-meta">%s</div>`, strings.Join(meta, " · "))

	if t.Detail != "" {
		fmt.Fprintf(&b, `<div class="task-detail">%s</div>`, html.EscapeString(t.Detail))
	}
	// What the agent did, before what it concluded. A paragraph on its own
	// gives no way to tell research from invention; a list of the tools it ran
	// does, and it is the first thing to look at when an answer seems wrong.
	if len(t.Steps) > 0 {
		b.WriteString(`<details class="task-steps"><summary>` +
			fmt.Sprintf("%d step%s", len(t.Steps), plural(len(t.Steps))) + `</summary><ol>`)
		for _, st := range t.Steps {
			cls := "task-step"
			if !st.OK {
				cls += " failed"
			}
			line := html.EscapeString(st.Tool)
			if st.Detail != "" {
				line += ` <span class="task-step-detail">` + html.EscapeString(st.Detail) + `</span>`
			}
			fmt.Fprintf(&b, `<li class="%s">%s <span class="task-step-took">%.1fs</span></li>`,
				cls, line, st.Seconds)
		}
		b.WriteString(`</ol></details>`)
	}

	if t.Result != "" {
		// The agent's answer, kept where the work was asked for, and rendered:
		// a model writes markdown, and a list of findings shown as raw asterisks
		// and hashes reads as a bug in the thing that produced it.
		//
		// app.Render, not RenderTrusted — this text came out of a model that had
		// just read news articles and web pages, so any HTML in it is HTML
		// somebody else wrote.
		fmt.Fprintf(&b, `<div class="task-result">%s</div>`, app.Render([]byte(t.Result)))
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

// taskPollJS reloads the page when the agent finishes something.
const taskPollJS = `<script>
(function(){
  var tries = 0;
  function check(){
    if (++tries > 120) return; // ten minutes, then stop asking
    fetch('/tasks', {headers:{'Accept':'application/json'}, credentials:'same-origin'})
      .then(function(r){ return r.json(); })
      .then(function(d){
        var busy = (d.tasks||[]).some(function(t){ return t.status === 'doing'; });
        if (!busy) { location.reload(); return; }
        setTimeout(check, 3000);
      })
      .catch(function(){ setTimeout(check, 5000); });
  }
  setTimeout(check, 3000);
})();
</script>`

const tasksPageCSS = `<style>
.task-add{display:flex;flex-direction:column;gap:8px;margin:10px 0 4px}
.task-add input[type=text],.task-add input:not([type]){min-width:0}
.task-add-row{display:flex;gap:8px;flex-wrap:wrap;align-items:center}
.task-add-row button{flex:0 0 auto}
.task-due{font-size:13px;color:var(--text-muted);display:flex;align-items:center;gap:6px;flex:1;min-width:200px}
.task-due input{flex:1;min-width:0;font-family:inherit;font-size:14px;padding:9px 11px;border:1px solid #d1d5db;border-radius:6px}
.task-assign{font-size:13px;color:var(--text-muted);display:flex;align-items:center;gap:8px;cursor:pointer}
.task-tabs{display:flex;gap:14px;flex-wrap:wrap;margin-bottom:12px}
.task-tab{font-size:13px;color:var(--text-muted);text-decoration:none}
.task-tab.active{color:var(--text-primary);font-weight:600}
.task{padding:12px 0;border-bottom:1px solid var(--divider)}
.task:last-child{border-bottom:none}
.task-title{font-weight:var(--font-weight-medium)}
.task-done .task-title{text-decoration:line-through;color:var(--text-muted)}
.task-meta{font-size:12px;color:var(--text-muted);margin-top:2px}
.task-detail{font-size:14px;margin-top:6px;color:var(--text-secondary)}
.task-result{font-size:14px;margin-top:8px;padding:2px 12px;background:var(--hover-background);border-radius:6px}
.task-result > :first-child{margin-top:10px}
.task-result > :last-child{margin-bottom:10px}
.task-result pre{overflow-x:auto}
.task-steps{margin-top:8px;font-size:13px}
.task-steps summary{cursor:pointer;color:var(--text-muted)}
.task-steps ol{margin:6px 0 0;padding-left:20px}
.task-step{margin:2px 0;font-variant-numeric:tabular-nums}
.task-step.failed{color:#b3261e;text-decoration:line-through}
.task-step-detail{color:var(--text-secondary)}
.task-step-took{color:var(--text-muted);font-size:12px}
.task-running{color:#a86400;font-weight:600}
.task-running::after{content:"";animation:taskdots 1.2s steps(4,end) infinite}
@keyframes taskdots{0%{content:""}25%{content:"."}50%{content:".."}75%{content:"..."}}
.task-actions{margin-top:8px}
.task-actions form{display:inline}
@media only screen and (max-width:600px){
  .task-add{grid-template-columns:1fr}
  .task-add button{width:100%}
  .task-actions .link-button{padding:6px 14px 6px 0;font-size:14px}
}
</style>`

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// addForm is the "what needs doing" form.
//
// Extracted so the due field can be tested without standing up a session: the
// bug it fixes was invisible in the markup and only showed up an hour later in
// somebody's day.
//
// The due field is a picker, and the value posted is an instant. It used to be
// a free-text box hinting "2026-08-09 09:00", which ParseDue read with no
// timezone — so Go parsed it as UTC and somebody in London typing 09:00 got a
// task due at 10:00 their time. Silently wrong by an hour is the worst kind of
// wrong for a deadline: nothing looks broken until you miss something. The
// datetime-local value is the browser's local time, and the script converts it
// to an RFC3339 instant on submit, which ParseDue already accepts. Same shape
// as the events form, because it is the same problem.
// The form is a column, not a row of equal columns.
//
// It used to be one auto-fit grid, so the title, the detail, the due picker,
// the assign checkbox and the button all competed for the same track width.
// The due picker ended up too narrow to read the date it was showing, and the
// checkbox — which the global input rule was sizing like a text field — sat a
// long way from the words it belonged to. Giving each its own line costs
// nothing and removes both problems.
func addForm(csrf string) string {
	return fmt.Sprintf(`<form method="POST" action="/tasks" class="task-add" onsubmit="var d=this.duelocal.value;this.due.value=d?new Date(d).toISOString():''">
  <input type="hidden" name="_csrf" value="%s">
  <input type="hidden" name="due" value="">
  <input name="title" placeholder="What needs doing?" required>
  <input name="detail" placeholder="Detail (optional)">
  <div class="task-add-row">
    <label class="task-due">Due <input type="datetime-local" name="duelocal"></label>
    <button type="submit">Add</button>
  </div>
  <label class="task-assign"><input type="checkbox" name="assign" value="agent"> <span>Give it to the agent — it starts working on this now</span></label>
</form>`, html.EscapeString(csrf))
}
