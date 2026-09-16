package work

import (
	"encoding/json"
	"fmt"
	"html"
	"mu/agent"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/tasks"
	"net/http"
	"regexp"
	"strings"
)

var secretValue = regexp.MustCompile(`(?i)((?:password|secret|cookie|private[_-]?key|access[_-]?token|refresh[_-]?token)["']?\s*[:=]\s*["']?)[^\s"'&,}]+`)

func diagnosticText(s string) string {
	s = ai.ProviderErrorDetail(s)
	s = secretValue.ReplaceAllString(s, "${1}[redacted]")
	if len(s) > 16384 {
		s = s[:16384] + "\n[truncated]"
	}
	return strings.ToValidUTF8(s, "")
}
func recordedStep(s agent.Step, status string) tasks.Step {
	args, _ := json.Marshal(s.Args)
	return tasks.Step{ID: s.ID, Tool: s.Tool, Detail: diagnosticText(tasks.StepDetail(s.Args)), Args: diagnosticText(string(args)), Output: diagnosticText(s.Output), Error: diagnosticText(s.Error), OK: s.OK, Seconds: s.Took.Seconds(), Started: s.Started, Finished: s.Finished, Status: status}
}
func diagnosticReport(t *tasks.Task) string {
	// No prompt or conversation body is added to a report copied for support.
	report := map[string]any{"work_id": t.ID, "status": t.Status, "created": t.Created, "updated": t.Updated, "attempts": t.Attempts, "steps": t.Steps, "outcome": t.Result}
	b, _ := json.MarshalIndent(report, "", "  ")
	return string(b)
}

// AdminHandler looks up an opaque work reference without requiring log searches.
func AdminHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil || !acc.Admin {
		app.Forbidden(w, r, "Operator access required")
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	body := `<form class="search-bar" method="GET" action="/admin/work"><input name="id" aria-label="Work ID" placeholder="Work ID" required><button>Find</button></form>`
	if id != "" {
		found := false
		for _, owner := range auth.AllAccounts() {
			if owner == nil {
				continue
			}
			t, e := tasks.Get(owner.ID, id)
			if e != nil {
				continue
			}
			body += `<h2>` + html.EscapeString(t.Title) + `</h2><pre>` + html.EscapeString(diagnosticReport(t)) + `</pre>`
			found = true
			break
		}
		if !found {
			body += fmt.Sprintf(`<p>No work found for %s.</p>`, html.EscapeString(id))
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	app.Respond(w, r, app.Response{Title: "Work diagnostics", HTML: body})
}

func saveProgress(owner, id, run string, steps []tasks.Step, report, failure, version string) {
	if err := tasks.Progress(owner, id, run, steps, report, failure, version); err != nil {
		app.Log("work", "could not record progress for %s run %s: %v", id, run, err)
	}
}
