package agent

// The agents page: what acts for you, and what each one may touch.
//
// It lists and it revokes; it does not create. Making an agent happens in one
// place, the builder at /agent/new, because this page used to carry a second
// create form that asked different questions and issued a different result.
//
// Nothing here is checked twice: the scope chosen in the builder is written into
// the token's permissions, and the MCP boundary enforces it against every call.
// This page cannot grant more than it shows, because it is not the thing doing
// the granting.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Handler serves /agents.
func RosterHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		// Signed out, this page is what the instance already has.
		//
		// It was a bounce to /login, which is a door with nothing visible
		// behind it: a stranger has no roster, so the page they were sent to
		// sign in for is empty for them by definition. What this instance can
		// already do, and the address of each agent, is a real answer — and it
		// is the same argument /contact makes, that "how do I use this" should
		// not need an account to ask. See builtinsPage.
		app.Respond(w, r, app.Response{
			Title:       "Agents",
			Description: "The agents on this instance, what each is for, and the address to write to.",
			HTML:        builtinsPage(),
		})
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "delete":
			_ = RemoveAgent(owner, r.FormValue("id"))
			http.Redirect(w, r, "/agents?removed=1", http.StatusSeeOther)
			return
		case "delete-run":
			// One workflow record, from the summary on the builder — the only
			// place runs are still listed, now that the page listing all of them
			// is gone.
			_ = deleteFlow(owner, r.FormValue("id"))
			http.Redirect(w, r, "/agents", http.StatusSeeOther)
			return
		case "token":
			// Back to where it was asked for.
			//
			// Both token forms post here, because this is where the action
			// lives — so pressing "Issue one" on an agent's Connect page landed
			// you on the roster, looking at a different page than the one you
			// were reading, with the secret somewhere on it. The form says
			// where it came from and this honours it.
			back := localPath(r.FormValue("back"))
			id := r.FormValue("id")
			secret, err := IssueToken(owner, id)
			if err != nil {
				http.Redirect(w, r, back+query(back, "error="+urlSafe(err.Error())), http.StatusSeeOther)
				return
			}
			// The secret goes across the redirect in memory, not in the URL.
			//
			// It was `&secret=…`: a bearer token in the browser's history and in
			// the access log of whatever terminates TLS in front of us, which on
			// a self-hosted install is an nginx or Caddy logging the full URI by
			// default. The URL now says which agent was just given a token, which
			// is a name, and the page collects the token itself. See secret.go.
			stashSecret(owner, id, secret)
			http.Redirect(w, r, back+query(back, "created="+urlSafe(id)), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/agents", http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, Agents(owner))
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(`<div class="page-col page-stack">`)
	// The way to make one, first.
	//
	// Three sentences of explanation stood here — what an agent is, what it
	// gets, what a token is for — and they were read once, by somebody who then
	// read them again every subsequent visit on the way to the button at the
	// bottom of the list. A page whose whole job is "here are yours, make
	// another" does not need a paragraph to say so.
	//
	// The button is the standard one, in the standard place: the top, where
	// every other page in this product puts its primary action.
	b.WriteString(`<div class="page-action">` + newAgentAction(owner) + `</div>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		// The way out is a link, because it reads as one. Hitting the agent limit
		// names where to change it, and as plain text that is a place you click
		// and nothing happens — so the one route the message exists to open is
		// the one that dead-ends. Named as the page rather than as its route,
		// for the reason the post banner gives: /plans is how it is addressed,
		// "Plans" is what it is called.
		said := html.EscapeString(msg)
		said = strings.ReplaceAll(said, "your account", `<a href="/account">your account</a>`)
		b.WriteString(`<p class="text-error">` + said + `</p>`)
	}
	if r.URL.Query().Get("removed") != "" {
		b.WriteString(`<p class="text-sm text-secondary">Agent removed and its token revoked.</p>`)
	}
	created := r.URL.Query().Get("created")
	if secret := takeSecret(owner, created); secret != "" {
		b.WriteString(secretPanel(secret, For(owner, created), app.BaseURL(r)))
	} else if created != "" {
		// An agent that runs here has no secret to show, so say what happened
		// and what it is now for — otherwise the page just silently grows a row.
		name := "It"
		if a := For(owner, created); a != nil {
			name = html.EscapeString(a.Name)
		}
		b.WriteString(`<p class="text-sm text-secondary">` + name + ` is ready. Open its name below ` +
			`to talk to it here — it has no token, so nothing outside this instance can call it.</p>`)
	}

	// Yours first, then ours.
	//
	// It was the other way round, because the section listing this instance's own
	// agents was added to a page that had only ever listed yours, and new
	// sections go at the top. That put six things nobody made above the one thing
	// they did, on the page named for what they made — and the empty state read
	// "That is the only one so far", a sentence about the row above it that only
	// worked in that order.
	//
	// The heading is drawn either way now. "Your agents / none yet" is the
	// account saying something true about itself; skipping it when the list is
	// empty means the page tells you what it has and not what you have.
	// One list, with the default first.
	//
	// Micro is not one of "our agents" in the sense the ten specialists were —
	// it is the agent this account already has. It answers agent@, it is who the
	// chat talks to, and it exists before anybody makes anything. Leaving it off
	// meant a new account opened /agents and was told it had none, which is
	// false, and meant the only row on the page after making one was a thing you
	// had to make to see anything at all.
	//
	// No headings. "Your agents" over a list that also contains the default
	// would be wrong, and two headings for two rows is furniture. What
	// distinguishes them is already on the row: the default has no Edit and no
	// Remove, because neither is a thing you can do to it.
	EnsureTags(owner)
	roster := Agents(owner)
	b.WriteString(`<div class="collection-grid">`)
	// The default carries the same sign of life as the rest. It is the one most
	// accounts have actually used, so a roster where every row but that one says
	// when it last spoke is a roster missing the row that would say the most.
	// Every agent this instance ships, not only the default. That was a single
	// hardcoded row back when there was a single one to draw; a second built-in
	// registered itself, answered at its own address, and appeared nowhere a
	// person could find it.
	for _, id := range PlatformNames() {
		b.WriteString(platformSeenRow(id, owner))
	}
	for _, a := range roster {
		b.WriteString(agentRow(a, csrf, app.BaseURL(r)))
	}
	b.WriteString(`</div>`)

	// One way to make an agent. This page used to carry its own create form
	// alongside the builder at /agent/new, and the two disagreed: one asked for
	// services and issued a token, the other asked for tools and a system prompt
	// and issued nothing, and neither linked to the other. The builder took the
	// missing question ("where does it run?") so this could become a link.
	//
	// At the cap it is not a button to the builder, because the builder is a
	// form you cannot submit. It becomes the thing that would actually change
	// the answer.
	// The catalogue, under the list rather than over it.
	//
	// /tools left the sidebar, because a tool is a property of something rather
	// than a destination: an agent's tools are what it may reach for, and this
	// is the page where that is decided. So the way in is from here — but below
	// what the page is about, because somebody arrives to see their agents and
	// not to browse tools, and the link was above both the list and the button.

	// The instance's own agents are listed here, at the top, by the loop over
	// PlatformNames above.
	//
	// The comment that stood here said the opposite — that they had been
	// removed, because rows of things nobody made above the one thing that is
	// yours teach that an agent is something the product hands you. That was
	// true when it was written and the code had since gone back the other way,
	// so the file argued with itself; a second listing was added underneath on
	// the strength of the prose, and Micro and Code appeared twice on the page.
	//
	// The prose was the stale half. They belong here: they are real, they
	// route, agent+news@ answers today, and a specialist with an address that
	// nothing tells you about is a feature that quietly stops being maintained.
	// What the original objection was actually about is the ordering, and that
	// is a live question rather than a settled one.

	// Close the column this page opened. Without it the footer was swallowed
	// into a 720px div and rendered halfway up the page: #content is a flex
	// child sized to hold the footer at the bottom, and an unclosed div puts
	// the footer inside the content instead of after it.
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Agents", Description: "The agents that act for you, and what each one may reach", HTML: b.String()})
}

// newAgentAction is the way to make another one, or the reason there is not.
//
// At the cap it is not a button to the builder, because the builder is a form
// you cannot submit. It becomes the thing that would actually change the
// answer.
func newAgentAction(owner string) string {
	full, have, max := AtAgentLimit(owner)
	if !full {
		return app.ActionLink("/agent/new", "New")
	}
	return app.ActionLink("/account/topup", "Top up to lift the limit") +
		fmt.Sprintf(`<p class="text-sm text-secondary mt-2 m-0">Your plan runs %d agent%s and `+
			`you have %d. Verify your address or put credit on <a href="/account">your account</a>, `+
			`or delete one first.</p>`, max, plural(max), have)
}

func urlSafe(s string) string { return strings.ReplaceAll(html.EscapeString(s), " ", "%20") }

// localPath is a redirect target this instance is willing to send somebody to.
//
// A path on this host, nothing else. It arrives in a form field, so without
// this an open redirect is one hidden input away — and the roster is the safe
// default because it is where the agent is either way.
func localPath(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "/agents"
	}
	return s
}

// query appends parameters to a path that may already have some.
func query(path, params string) string {
	if strings.Contains(path, "?") {
		return "&" + params
	}
	return "?" + params
}

// secretPanel shows the token once. There is no second chance by design: it is
// stored hashed, so this page could not show it again if it wanted to.
func secretPanel(secret string, a *Agent, base string) string {
	name, endpoint := "your agent", base+"/mcp"
	if a != nil {
		name, endpoint = a.Name, a.Endpoint(base)
	}
	return `<div class="card card-ok m-0 mb-5">` +
		`<h4 class="m-0 mb-2 text-base">` + html.EscapeString(name) + ` is ready</h4>` +
		`<p class="text-sm text-muted m-0 mb-2">Copy this token now — it is stored hashed and cannot be shown again.</p>` +
		`<pre class="agent-secret">` + html.EscapeString(secret) + `</pre>` +
		`<p class="text-sm text-muted mt-2 m-0">Point it at <code>` + html.EscapeString(endpoint) + `</code></p>` +
		`</div>`
}

// agentRow shows an agent the way it is actually used.
//
// It used to show every agent the same: an MCP endpoint, "never used", and an
// "Issue token" button with no hint whether one was needed. For an agent you
// only talk to here that is three kinds of wrong at once — the endpoint cannot
// be called without a token, "never used" describes a token that does not
// exist, and the button reads like a required step. The first question anybody
// asked was "do I need to issue a token?", which is the page's fault.
//
// So it branches on whether there is a credential, not on the declared kind:
// what matters is whether this thing can be called from outside, and a token is
// exactly what decides that.
// entry is one line in the directory: somebody you can write to.
//
// Both lists render through this, which they did not before — an agent you made
// and one that came with the instance were two different pieces of markup with
// different links in a different order, so the page looked like two products
// stacked. "Talk to it" and "How to reach it →" on one, "Connect / Edit / Fork /
// Talk to it →" on the other, all at slightly different sizes.
//
// What a directory entry is: a name, what it is for, its address, and the two
// ways to reach it. Everything else — the token, the endpoint, the scope, where
// it runs — is administration, and lives one click away on the page that exists
// for it.
type entry struct {
	Name string // what it is called
	Path string // where the name opens
	// Chat is where talking to it happens, and empty when there is nowhere.
	//
	// An agent that runs elsewhere is reached over MCP by something outside this
	// instance; there is no conversation with it here, and offering one is a box
	// that will never be used. Its name opens the page it does have.
	Chat  string
	For   string // one line: what it is for
	ID    string // for the links that need it
	Admin bool   // whether Edit and Fork apply, which is only to your own
	Extra string // owner-only controls, in the strip with the links
	// Seen is what it last dealt with, and When is how long ago.
	//
	// The roster was a directory: name, purpose, three links, and nothing
	// saying whether any of them were alive. A list you are scanning to decide
	// who to talk to needs a sign of life against each one.
	//
	// Two fields because they go in two places. The subject reads as a line
	// under the name, where the description already is; the time is a
	// right-hand column, the way it is on every list of conversations here and
	// everywhere else. It was one string reading "Last: Tuesday · 2 hours ago",
	// which put a label nobody needs in front of the only interesting word and
	// buried the timestamp mid-sentence.
	Seen   string
	When   string
	Status string
}

// entryRow draws one.
//
// Chat first, because talking to it is the point of it existing. Email second,
// as a mailto — this product's whole claim is that you can write to the thing,
// and a page listing agents that had no way to start a message was describing
// the feature rather than offering it.
func entryRow(e entry) string {
	var b strings.Builder
	b.WriteString(`<div class="agent-row"><div class="agent-card-content"><div class="agent-head"><a class="agent-name" href="` + e.Path + `">` + html.EscapeString(e.Name) + `</a></div>`)
	if e.For != "" {
		b.WriteString(`<div class="agent-for">` + html.EscapeString(e.For) + `</div>`)
	}
	if e.Status != "" {
		b.WriteString(`<div class="activity-status">` + html.EscapeString(e.Status) + `</div>`)
	}
	b.WriteString(`<div class="agent-links"><div class="agent-open-actions">`)
	if e.Chat != "" {
		b.WriteString(`<a href="` + e.Chat + `" aria-label="Chat with ` + html.EscapeString(e.Name) + `" title="Chat"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M4 4h16v12H9l-5 4z"/></svg></a>`)
	}
	b.WriteString(`<a href="/agent/connect?id=` + html.EscapeString(e.ID) + `" aria-label="Connect to ` + html.EscapeString(e.Name) + `" title="Connect"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m9 15 6-6M8 16l-1 1a4 4 0 0 1-6-6l4-4a4 4 0 0 1 6 0m2 10a4 4 0 0 0 6 0l4-4a4 4 0 0 0-6-6l-1 1"/></svg></a></div><div class="agent-manage-actions">`)
	if e.Admin {
		b.WriteString(`<a href="/agent/new?id=` + html.EscapeString(e.ID) + `">Edit</a>`)
	}
	b.WriteString(e.Extra + `</div></div></div></div>`)
	return b.String()
}

func agentRow(a *Agent, csrf, base string) string {
	// What it is for, in one line.
	//
	// The row used to carry four: the scope as a coloured list of service
	// names, a meta line about tokens, an address with a verb in front of it,
	// and a badge saying whether it runs here — every fact the system knows,
	// on a list you are scanning to decide who to write to. A directory entry
	// says what somebody does; the rest is on their page.
	//
	// The description is what the agent was made for and what the router reads
	// when deciding who answers, so it is the closest thing to a job title. An
	// agent made before there was a field for it falls back to its scope.
	for_ := strings.TrimSpace(a.Description)
	if for_ == "" {
		if a.Unscoped() {
			for_ = "Everything"
		} else {
			labels := make([]string, 0, len(a.Services))
			for _, sv := range a.Services {
				labels = append(labels, service.Label(sv))
			}
			for_ = strings.Join(labels, ", ")
		}
	}

	// Remove stays on the row, because it is the one thing you do to an entry
	// without opening it — and it is the reason the row has a right-hand side.
	extra := fmt.Sprintf(`<form method="POST" action="/agents" class="form-action m-0" onsubmit="return confirm('Remove this agent?')">
    <input type="hidden" name="_csrf" value="%s">
    <input type="hidden" name="action" value="delete">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="agent-remove">Remove</button>
  </form>`, html.EscapeString(csrf), html.EscapeString(a.ID))

	// Every agent opens on talking to it.
	//
	// There used to be a fork here: an agent declared "external" opened on the
	// Connect page instead, because there was nothing to talk to — it ran in
	// Claude or Cursor and called in with a token. That distinction is gone.
	// Every agent runs here and answers at POST /agent/<name>; a token is a
	// thing you additionally give something outside, not a different kind of
	// agent. See the Kind field in roster.go.
	open, chat := Path(a.Owner, a.ID), Path(a.Owner, a.ID)

	return entryRow(entry{
		Name:   a.Name,
		Status: activity(a.Owner, a.ID),
		Path:   open,
		Chat:   chat,
		For:    for_,
		Seen:   seenLine(a.Owner, a.ID),
		When:   seenWhen(a.Owner, a.ID),
		ID:     a.ID,
		Admin:  true,
		Extra:  extra,
	})
}

// platformRow is one of this instance's own agents.
// platformSeenRow is platformRow with the account's own history against it.
//
// Public callers omit the viewer so private activity is never included.
func platformSeenRow(name, accountID string) string {
	return platformRow(name, accountID)
}

func platformRow(name string, viewers ...string) string {
	a := Platform(name)
	if a == nil {
		return ""
	}

	for_ := strings.TrimSpace(a.Description)
	if for_ == "" {
		for_ = toolWords(a.Tools)
	}
	if len(a.Tools) == 0 {
		for_ = "Everything"
	}
	// The default says so, because nothing else on the row does. It has no Edit
	// and no Remove, which is a difference you notice only by comparing it with
	// a row underneath — and on an account with no agents of its own there is
	// nothing to compare it to.
	if strings.EqualFold(name, DefaultPlatformAgent) {
		for_ = "The default — " + strings.ToLower(for_[:1]) + for_[1:]
	}

	// No Edit and no Remove: this one is the instance's, not yours. Making one
	// of your own starts from the builder rather than from this row.
	path := "/agent/" + strings.ToLower(name)
	e := entry{
		Name: a.Name,
		Path: path,
		Chat: path,
		For:  for_,
		ID:   strings.ToLower(name),
	}
	if len(viewers) > 0 {
		e.Status = activity(viewers[0], name)
		e.Seen = seenLine(viewers[0], name)
		e.When = seenWhen(viewers[0], name)
	}
	return entryRow(e)
}

// seenLine is what an agent last dealt with, and seenWhen is how long ago.
//
// Two functions because they go in two places on the row: the subject under the
// name, the time out to the right. No "Last:" in front of either — a label on a
// line that is obviously the last thing is a word the reader has to step over
// to get to the only part that says anything.
//
// "Not used yet" rather than nothing, because a blank where every other row has
// a line reads as a render that failed — and because an agent you made and
// never wrote to is a real state worth seeing on the list that would let you.
func seenLine(accountID, agentID string) string {
	about, at := LastSeen(accountID, agentID)
	if at.IsZero() {
		return "Not used yet"
	}
	if about == "" {
		return "a conversation"
	}
	return trimTo(about, seenChars)
}

func seenWhen(accountID, agentID string) string {
	if _, at := LastSeen(accountID, agentID); !at.IsZero() {
		return app.TimeAgo(at)
	}
	return ""
}

// seenChars bounds the subject on a row. Long enough to recognise a
// conversation, short enough that the line stays one.
const seenChars = 50

func toolWords(tools []string) string {
	var out []string
	seen := map[string]bool{}
	for _, t := range tools {
		name := t
		if i := strings.IndexByte(t, '_'); i > 0 {
			name = t[:i]
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return "Everything"
	}
	return strings.Join(out, ", ")
}
