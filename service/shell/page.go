package shell

// The page opens an interactive session in the account's existing container.
// Legacy command POSTs remain supported and use the tool's per-command charge.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/container"
	"mu/internal/quota"
	"mu/internal/sshaccess"
)

// Handler serves /shell.
func Handler(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		terminalHandler(w, r)
		return
	}
	var b strings.Builder
	b.WriteString(`<div class="sbx">`)

	if !Configured() {
		// The reason rather than a guess at it. This said "an admin installs
		// one and restarts", which is the wrong instruction for the common
		// failure: Docker installed and running, and this server running as a
		// user that cannot open its socket.
		b.WriteString(app.Problem("No machine available — " + container.Reason() + "."))
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "Shell", Description: Spec.Description, HTML: b.String()})
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		b.WriteString(`<p class="sbx-problem">` +
			app.TextLink("Sign in", "/login?redirect=/shell") +
			` to get a machine. The files are yours and running things costs, so it ` +
			`needs an account to keep them under and to bill.</p></div>`)
		app.Respond(w, r, app.Response{Title: "Shell", Description: Spec.Description, HTML: b.String()})
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
			app.Respond(w, r, app.Response{Title: "Shell", Description: Spec.Description, HTML: b.String()})
			return
		}
		// Two forms post here: a command, and the SSH keys. They are told
		// apart by which field arrived rather than by a hidden action field,
		// because a missing action would otherwise run whichever branch was
		// written first.
		switch {
		case r.FormValue("sshkey") != "":
			b.WriteString(sshaccess.Add(acc.ID, r.FormValue("sshkey"), r.FormValue("keyname")))
		case r.FormValue("removekey") != "":
			if err := auth.RemoveSSHKey(acc.ID, r.FormValue("removekey")); err != nil {
				b.WriteString(app.Problem(err.Error()))
			}
		default:
			command = strings.TrimSpace(r.FormValue("command"))
		}
	}

	b.WriteString(`<div id="shell-terminal-controls" class="form-actions shell-terminal-actions" data-csrf="` + html.EscapeString(auth.CSRFToken(r)) + `"><button type="button" id="shell-connect">Open terminal</button><button type="button" id="shell-disconnect" hidden>Disconnect</button><button type="button" data-terminal-key="ctrl-c" disabled>Ctrl+C</button><button type="button" data-terminal-key="tab" disabled>Tab</button><button type="button" data-terminal-key="escape" disabled>Esc</button><button type="button" data-terminal-key="up" aria-label="Up arrow" disabled>↑</button><button type="button" data-terminal-key="down" aria-label="Down arrow" disabled>↓</button><button type="button" data-terminal-key="left" aria-label="Left arrow" disabled>←</button><button type="button" data-terminal-key="right" aria-label="Right arrow" disabled>→</button><span id="shell-status" role="status">Disconnected</span></div><div id="shell-terminal" class="shell-terminal" aria-label="Terminal"></div><noscript>JavaScript is needed for the terminal. You can also connect using SSH below.</noscript>`)

	b.WriteString(`<div id="shell-result" aria-live="polite">`)
	if command != "" {
		b.WriteString(running(r, acc.ID, command))
	} else {
		// What this instance actually gives, rather than what the service can
		// give. The numbers are derived from the host, so a page that quoted
		// the defaults would be wrong on most machines — and "why did my build
		// get killed" is answered by the number, not by the feature.
		l := limits()
		note := `Opening a terminal costs ` +
			credits(quota.OperationCost(quota.OpShellRun)) + `. Sessions last up to four hours. Keeping and reading files is free. `
		if shared() {
			note = `Shared shell execution is disabled. Existing files are preserved; an administrator must migrate the workspaces to per-account containers before enabling shell access.`
		} else {
			note += `This instance gives each machine <code>` +
				html.EscapeString(l.Memory) + `</code> of memory and <code>` +
				html.EscapeString(l.CPUs) + `</code> CPU, from ` +
				html.EscapeString(image()) + `, and runs at most ` +
				strconv.Itoa(machineBudget()) + ` at once — yours is stopped when it ` +
				`has been idle a while, or to make room, and your files are kept either way.`
		}
		b.WriteString(app.NoteHTML(note))
	}

	b.WriteString(`</div>`)
	b.WriteString(sshaccess.Card(r, acc.ID, "/shell", "Shell access",
		"Register a public key and you can open a shell in your machine from a terminal. The same box, the same files, the same limits — a person at the prompt instead of a command at a time.",
		"ssh"))
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Shell", Description: Spec.Description, HTML: b.String()})
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
