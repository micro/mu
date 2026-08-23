package sandbox

// The page: a prompt, and what came back.
//
// A working machine rather than a description of one, for the same reason
// /browser puts a URL box on the screen and /maps draws a map. The claim here
// is that you get a computer; the only way to show that is to let somebody run
// something on it and see the output.
//
// It is the same shell the tool reaches, through the same gate and the same
// charge — see exec in box.go. A page that ran commands for free would be a way
// round the price, and one that ran them somewhere else would be a demo.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
)

// Handler serves /sandbox.
func Handler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<div class="sbx">`)
	b.WriteString(`<p class="svc-lead">` + Spec.Description + `. A container of your ` +
		`own with a shell in it: what you put in <code>/work</code> stays there between ` +
		`commands, and nothing you run can reach this machine.</p>`)

	if !Configured() {
		b.WriteString(app.Problem("This instance has no container runtime, so there is " +
			"nothing to give you. Docker is not built into the binary — it is a separate " +
			"program with a daemon — so an admin installs one and restarts."))
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "Sandbox", Description: Spec.Description, HTML: b.String()})
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		b.WriteString(`<p class="sbx-problem">` +
			app.TextLink("Sign in", "/login?redirect=/sandbox") +
			` to get a machine. The files are yours and running things costs, so it ` +
			`needs an account to keep them under and to bill.</p></div>`)
		app.Respond(w, r, app.Response{Title: "Sandbox", Description: Spec.Description, HTML: b.String()})
		return
	}

	// POST, unlike /browser's GET form. A command is not a link: it changes
	// something, it may cost, and putting `rm -rf .` in a URL makes it a thing
	// that runs when somebody follows it.
	command := ""
	if r.Method == http.MethodPost {
		if !auth.ValidCSRF(r) {
			b.WriteString(app.Problem("That form was stale. Reload the page and try again."))
			b.WriteString(`</div>`)
			app.Respond(w, r, app.Response{Title: "Sandbox", Description: Spec.Description, HTML: b.String()})
			return
		}
		command = strings.TrimSpace(r.FormValue("command"))
	}

	b.WriteString(`<form class="sbx-form" method="post" action="/sandbox">`)
	b.WriteString(`<input type="hidden" name="csrf_token" value="` +
		html.EscapeString(auth.CSRFToken(r)) + `">`)
	b.WriteString(`<div class="sbx-line"><span class="sbx-prompt">/work $</span>` +
		`<input class="sbx-cmd" type="text" name="command" autofocus autocomplete="off" ` +
		`spellcheck="false" placeholder="ls -la" value="` + html.EscapeString(command) + `"></div>`)
	b.WriteString(`<button type="submit">Run</button>`)
	b.WriteString(`</form>`)

	if command != "" {
		b.WriteString(running(r, acc.ID, command))
	} else {
		// What this instance actually gives, rather than what the service can
		// give. The numbers are derived from the host, so a page that quoted
		// the defaults would be wrong on most machines — and "why did my build
		// get killed" is answered by the number, not by the feature.
		l := limits()
		b.WriteString(app.NoteHTML(`Running a command costs ` +
			credits(quota.OperationCost(quota.OpSandboxRun)) + `, because it is CPU and ` +
			`memory here. Keeping and reading files is free. This instance gives each ` +
			`machine <code>` + html.EscapeString(l.Memory) + `</code> of memory and ` +
			`<code>` + html.EscapeString(l.CPUs) + `</code> CPU, from ` +
			html.EscapeString(image()) + `, and runs at most ` +
			strconv.Itoa(machineBudget()) + ` at once — yours is stopped when it has been ` +
			`idle a while, or to make room, and your files are kept either way.`))
	}

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Sandbox", Description: Spec.Description, HTML: b.String()})
}

// running runs what was typed and shows it.
func running(r *http.Request, accountID, command string) string {
	res, err := paidRun(r.Context(), accountID, command, "")
	if err != nil {
		return `<p class="sbx-problem">` + html.EscapeString(err.Error()) + `</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="sbx-out">`)
	out := res.Out
	if strings.TrimSpace(out) == "" {
		// A command that printed nothing succeeded silently, which is normal and
		// looks identical to a page that failed to render. Say which.
		out = "(no output)"
	}
	b.WriteString(`<pre class="sbx-text">` + html.EscapeString(out) + `</pre>`)
	// The exit status, when it is not zero. Shown rather than folded into the
	// output, because a command whose whole failure is its status — a test run,
	// a grep that matched nothing — prints nothing at all.
	if res.Code != 0 {
		b.WriteString(`<p class="sbx-code">exited ` + strconv.Itoa(res.Code) + `</p>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// credits writes a price the way a person reads one.
func credits(n int) string {
	if n == 1 {
		return "1 credit"
	}
	return strconv.Itoa(n) + " credits"
}
