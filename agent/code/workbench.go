package code

// The workbench: what this agent has actually made.
//
// A chat with the Code agent showed a conversation and nothing else, which for
// this one agent is the wrong half of the screen. The deliverable is not the
// reply — the eval next door says so at length and marks the filesystem rather
// than the prose — it is a file on a machine and an app at an address. Neither
// appeared anywhere in the product: to see what you had built you asked it to
// tell you, which costs a model call to read a directory, and it might be
// wrong.
//
// So the rail beside the conversation carries the two things that outlive it.
// The files come from service/shell, which owns the machine, and the apps from
// service/apps, which owns the hosting; this package only arranges them. That
// is the whole of what a composer is allowed to be.
//
// # Why the files can be missing
//
// A machine is stopped when nobody is using it, and listing files does not
// start one — see shell.WorkspaceOf, which explains why waking a container to
// draw a page is a cost somebody else pays. So the section says asleep rather
// than empty, because those are different facts and only one of them means
// your work is gone.

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/container"
	"mu/service/apps"
	"mu/service/shell"
)

// look is how long the page waits on the machine. A rail that hangs is worse
// than a rail that says it could not look: the conversation is the page, and
// this is beside it.
const look = 4 * time.Second

// RailSection is the workspace and the apps, for the rail on this agent's page.
//
// Empty for every other agent, and the condition lives here rather than in the
// page: agent/ renders the same rail for all of them and should not learn which
// one has a machine. Nothing else has one, so nothing else has a section.
func RailSection(accountID, agentID string) string {
	if agentID != ID || strings.TrimSpace(accountID) == "" {
		return ""
	}
	return workspaceHTML(accountID) + appsHTML(accountID)
}

func workspaceHTML(accountID string) string {
	var b strings.Builder
	b.WriteString(`<div class="chat-sess-head">Workspace</div><div class="chat-sess-list">`)

	switch {
	case !shell.Configured():
		// An instance with no container runtime. Said plainly, because the
		// agent will fail at its first tool call and this is the only place
		// that can explain why before it does.
		b.WriteString(note("No machine on this instance — " + container.Reason() + "."))
	default:
		ctx, cancel := context.WithTimeout(context.Background(), look)
		defer cancel()
		ws, err := shell.WorkspaceOf(ctx, accountID)
		switch {
		case err != nil:
			b.WriteString(note("Could not read your machine just now."))
		case !ws.Awake:
			b.WriteString(note("Asleep. Your files are kept; ask for something and it wakes."))
		case len(ws.Files) == 0:
			b.WriteString(note("Nothing here yet. Ask it to build something."))
		default:
			for _, f := range ws.Files {
				b.WriteString(fileRow(f))
			}
			if ws.Total > len(ws.Files) {
				b.WriteString(note(strconv.Itoa(ws.Total-len(ws.Files)) + " more."))
			}
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

func fileRow(f shell.File) string {
	name := html.EscapeString(f.Name)
	if f.Dir {
		// Not a link. Opening a directory is a second page and a second set of
		// paths to get right, and what somebody wants from this rail is the
		// file the agent just wrote — which is at the top level, because the
		// prompt tells it to build there.
		return `<div class="chat-sess-row"><span class="chat-sess">` + name + `/</span></div>`
	}
	return `<div class="chat-sess-row"><a class="chat-sess" href="/code/file?name=` +
		html.EscapeString(url.QueryEscape(f.Name)) + `">` + name +
		` <span class="text-muted text-sm">` + size(f.Size) + `</span></a></div>`
}

func appsHTML(accountID string) string {
	mine := apps.AuthoredBy(accountID)
	var b strings.Builder
	b.WriteString(`<div class="chat-sess-head">Apps</div><div class="chat-sess-list">`)
	if len(mine) == 0 {
		b.WriteString(note("Nothing hosted yet. Ask it to host what it builds."))
	}
	for i, a := range mine {
		if i >= appsShown {
			// A count and not a link. There is no page that lists an account's
			// own apps — apps.AuthoredBy had no caller at all before this one,
			// which is its own small piece of evidence for what this package is
			// fixing — and pointing at the directory, which lists everybody's,
			// would answer a different question.
			b.WriteString(note(strconv.Itoa(len(mine)-appsShown) + " more."))
			break
		}
		// Said when it is not published.
		//
		// AuthoredBy is every app of this account's; the directory lists only
		// the public ones — see apps.Directory. So a private app appeared here
		// looking exactly like a published one, and the only way to find out it
		// was not in the directory was to go and look. Reported as an app
		// listed here that "doesn't exist as an app".
		mark := ""
		if !a.Public {
			mark = ` <span class="text-muted text-sm">private</span>`
		}
		b.WriteString(`<div class="chat-sess-row"><a class="chat-sess" href="/apps/` +
			html.EscapeString(a.Slug) + `">` + html.EscapeString(a.Name) + mark + `</a></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// appsShown is how many apps the rail draws before pointing at the page that
// lists them all.
const appsShown = 8

func note(text string) string {
	return `<div class="chat-sess-empty">` + html.EscapeString(text) + `</div>`
}

// size is a file's size in the units a person reads.
func size(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + "M"
	case n >= 1<<10:
		return strconv.FormatInt(n/(1<<10), 10) + "K"
	default:
		return strconv.FormatInt(n, 10) + "B"
	}
}

// Handler serves /code, which is this agent's front door.
//
// It was a link on two pages of /apps — "New app" and "Describe an app", the
// primary call to action of that whole section — gated in the route table and
// claimed by no handler, so it fell to the catch-all and rendered the front
// page. The design intent was written down twice and never built: see
// apps.AuthoredBy, exported for "the page that builds them", and the note on
// /apps saying the box that cannot iterate is the one to delete because this
// agent is the one that can.
//
// A redirect rather than a second page. Somewhere to describe an app is exactly
// what /agent/code is, and two pages with a box on them that both build an app
// is the thing /apps deleted its own box to avoid.
func Handler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, Path, http.StatusSeeOther)
}

// FileHandler serves /code/file — one file out of the workspace, read-only.
//
// The click that the listing invites. Reading a file the agent wrote should not
// cost a model call and should not need the agent's attention at all; it is
// yours and it is on a machine this instance runs.
func FileHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Redirect(w, r, Path, http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), look)
	defer cancel()
	text, truncated, err := shell.ReadFile(ctx, acc.ID, name, 64<<10)

	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card">`)
	b.WriteString(`<p class="text-sm"><a href="` + Path + `">&larr; Code</a></p>`)
	b.WriteString(`<h3>` + html.EscapeString(name) + `</h3>`)
	if err != nil {
		// The likely cause first. A machine is stopped when nobody is using it,
		// and the list this link came from may have been drawn before that
		// happened — so the honest first guess is that it went to sleep, not
		// that the file is gone.
		b.WriteString(app.Problem("Could not read it. The machine may have gone to " +
			"sleep since this list was drawn — ask the agent for something and it wakes."))
	} else {
		if truncated {
			b.WriteString(app.NoteHTML("The first 64K of it. The whole file is on the machine."))
		}
		b.WriteString(`<pre>` + html.EscapeString(text) + `</pre>`)
	}
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{
		Title:       name,
		Description: "A file in your workspace",
		HTML:        b.String(),
	})
}
