package code

// /code — say what you want, and the thing you get is an app.
//
// # What this is
//
// The same shape as a coding agent in a terminal, aimed at one kind of output.
// You describe something; a document is written; the scanner and the tests run
// over it; if they complain the model is told what they said and asked again.
// Then you look at it running, say what is wrong, and that is the next turn.
//
// Three things make that different from the box on /apps/new, which asks once
// and hands back whatever came out:
//
//   - It is a conversation. The second thing you say is a change to the first,
//     not a new app, so "now make it dark" keeps the tracker you already have.
//   - The checks are in the loop rather than after it. A document that calls a
//     service that does not exist never reaches you; the model is told and
//     tries again, up to three times. See build.go.
//   - Every turn is a version. Undo is not a feature that had to be built here
//     — snapshotVersion already ran on every edit, so the transcript on this
//     page *is* the version list, read back.
//
// # Why the transcript is not stored
//
// It would be the obvious thing to add and it would be a second copy of the
// truth. What was asked for is App.Versions[i].Summary; what came of it is
// App.Versions[i].HTML. A separate conversation log would say the same things
// in a second place, and the two would disagree the first time somebody rolled
// a version back — the log would still claim a change that no longer exists.
//
// So the conversation is derived. Roll back to version 3 and the transcript
// says three turns, because three turns is what the app has been through.
//
// # Why this is not in service/apps
//
// It was, and it could not do its job there. Building an app the way somebody
// would means running an agent with a shell — and services here are leaves.
// None of them imports the agent, and the one that tried to import another was
// caught by a test. A page that composes the agent, the machine and the store
// cannot live inside any one of them.
//
// So it sits above all three, which is what inbox already does for mail and
// chat: a composer, not a service. No Spec, no endpoints, nothing in the
// catalogue. It imports apps for hosting, shell for the machine, and agent for
// the loop, and none of them knows it exists.

import (
	"fmt"
	htmlpkg "html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/service/apps"
)

// Handler serves /code.
func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		app.Unauthorized(w, r)
		return
	}
	if r.Method == http.MethodPost {
		codeTurn(w, r, acc)
		return
	}
	codePage(w, r, acc)
}

// codeTurn is one thing said.
//
// No slug is a new app; a slug is a change to that one. The same box does both,
// because from where somebody is sitting they are the same act — they said what
// they wanted, and either there was already an app to change or there was not.
func codeTurn(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	var req struct {
		Prompt string `json:"prompt"`
		App    string `json:"app"`
	}
	if app.SendsJSON(r) {
		if err := app.DecodeJSON(r, &req); err != nil {
			app.Error(w, r, http.StatusBadRequest, "could not read that")
			return
		}
	} else {
		req.Prompt = r.FormValue("prompt")
		req.App = r.FormValue("app")
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.App = strings.TrimSpace(req.App)
	if req.Prompt == "" {
		app.Error(w, r, http.StatusBadRequest, "Say what you want built")
		return
	}

	// Which app this turn is about, and whether it already exists. A turn with
	// no app names one from what was asked for; a turn with one keeps working
	// on it, which is the whole difference between this and a box that builds
	// something new every time you speak.
	slug, existing := req.App, true
	if slug == "" {
		slug, existing = slugFor(req.Prompt), false
	} else if a := apps.GetApp(slug); a == nil || a.AuthorID != acc.ID {
		// Somebody else's app, or none. Not a 500 and not a silent new app.
		app.Error(w, r, http.StatusNotFound, "No app of yours by that name")
		return
	}

	res, err := run(acc.ID, slug, req.Prompt, existing)
	if err != nil {
		// The failure is the interesting half. "It wrote nothing" and "the page
		// it wrote is not allowed" are different problems with different next
		// moves, and a page that says "something went wrong" for both is a page
		// nobody can act on.
		app.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}
	app.RespondJSON(w, map[string]interface{}{
		"app": res.App.Slug, "name": res.App.Name, "url": "/apps/" + res.App.Slug,
		"said": res.Said, "steps": res.Steps,
	})
}

// codePage renders the workspace: what has been said, the app running, and the
// box to say the next thing.
func codePage(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	slug := strings.TrimSpace(r.URL.Query().Get("app"))

	var a *apps.App
	if slug != "" {
		a = apps.GetApp(slug)
		if a == nil {
			app.Error(w, r, http.StatusNotFound, "No app by that name")
			return
		}
		if a.AuthorID != acc.ID {
			// Somebody else's app is readable at /apps/<slug> and not editable
			// here, and saying so is better than a 404 that reads like a typo.
			app.Error(w, r, http.StatusForbidden,
				"That app is somebody else's. Fork it from /apps/"+a.Slug+" to change it.")
			return
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="code-page">`)
	if a == nil {
		b.WriteString(codeStart(acc))
	} else {
		b.WriteString(codeWorkspace(a))
	}
	b.WriteString(codeBox(a))
	b.WriteString(`</div>`)
	b.WriteString(codeScript())

	title := "Code"
	if a != nil {
		title = a.Name
	}
	app.Respond(w, r, app.Response{
		Title:       title,
		Description: "Describe an app and it gets written, checked and run",
		HTML:        b.String(),
	})
}

// codeStart is the empty state: what this does, and what you already have.
//
// The list of your own apps is here rather than on a second page because the
// first question on arriving with nothing is "did I make one of these before".
func codeStart(acc *auth.Account) string {
	var b strings.Builder
	b.WriteString(`<p class="text-muted">Describe an app. It gets written as a single page, ` +
		`checked — the scanner for what an app may not do, and the tests, which run its ` +
		`calls for real — and rewritten until it passes. Then say what to change.</p>`)

	mine := apps.AuthoredBy(acc.ID)
	if len(mine) > 0 {
		b.WriteString(`<h2 class="h6 mt-3">Yours</h2><ul class="plain">`)
		for i, x := range mine {
			if i >= 8 {
				b.WriteString(`<li>` + app.TextLink("All of them →", "/apps") + `</li>`)
				break
			}
			b.WriteString(`<li>` +
				app.TextLink(htmlpkg.EscapeString(x.Name), "/code?app="+htmlpkg.EscapeString(x.Slug)) +
				` <span class="text-muted text-xs">· ` +
				app.TextLink("open", "/apps/"+htmlpkg.EscapeString(x.Slug)) + `</span>`)
			// CreateApp fills an empty description with the name, so printing
			// both says everything twice: "Expenses Expenses".
			if x.Description != "" && x.Description != x.Name {
				b.WriteString(` <span class="text-muted text-xs">` +
					htmlpkg.EscapeString(x.Description) + `</span>`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	return b.String()
}

// codeWorkspace is the app itself and everything said to get it here.
func codeWorkspace(a *apps.App) string {
	var b strings.Builder

	b.WriteString(`<div class="code-run">`)
	b.WriteString(`<iframe id="code-preview" src="/apps/` + htmlpkg.EscapeString(a.Slug) +
		`?raw=1" title="` + htmlpkg.EscapeString(a.Name) + `"></iframe>`)
	b.WriteString(`</div>`)

	// TextLink, not class="link". mu.css makes .link display:block — right for
	// a card's one call to action, and it turns a row of them separated by
	// middots into three stacked lines with the separators orphaned between.
	// The stylesheet says so in a comment directly above the rule, and this
	// was written with class="link" anyway; measured at 104px tall for one
	// line of text before it was believed.
	slug := htmlpkg.EscapeString(a.Slug)
	b.WriteString(`<p class="code-links">` +
		app.TextLink("Open it", "/apps/"+slug) + ` · ` +
		app.TextLink("Edit the code", "/apps/"+slug+"/edit") + ` · ` +
		app.TextLink("Versions", "/apps/"+slug+"/versions") +
		`</p>`)

	b.WriteString(codeTranscript(a))
	return b.String()
}

// codeTranscript reads the conversation back out of the version list.
//
// Newest last, so it reads downward like a conversation and the box is under
// the most recent thing said. "Initial version" is what CreateApp writes for
// the first snapshot; here that turn is the description, which is a truer
// account of what was asked.
func codeTranscript(a *apps.App) string {
	if len(a.Versions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ol class="code-turns">`)
	for _, v := range a.Versions {
		said := strings.TrimSpace(v.Summary)
		if said == "" || said == "Initial version" {
			said = strings.TrimSpace(a.Description)
		}
		if said == "" {
			said = "Built"
		}
		// Anchored at the version it names. Every one of these pointed at the
		// bare versions page, so v1 and v7 were the same link and landing on
		// the right one was the reader's problem.
		b.WriteString(`<li><span class="code-said">` + htmlpkg.EscapeString(said) + `</span> ` +
			app.TextLink("v"+fmt.Sprint(v.Number),
				"/apps/"+htmlpkg.EscapeString(a.Slug)+"/versions#v"+fmt.Sprint(v.Number)) +
			`</li>`)
	}
	b.WriteString(`</ol>`)
	return b.String()
}

// codeBox is where you say the next thing.
func codeBox(a *apps.App) string {
	// The button says which of the two things this turn is. It said "Build" in
	// both states, under a box asking "What should change?" — a label that
	// contradicts the field above it is worse than no label.
	slug, placeholder, label := "", "What should it do?", "Build"
	cost := quota.OperationCost(quota.OpAppBuild)
	if a != nil {
		slug, placeholder, label = a.Slug, "What should change?", "Change it"
		cost = quota.OperationCost(quota.OpAppEdit)
	}
	return `<form id="code-form" class="code-box">` +
		`<input type="hidden" id="code-app" value="` + htmlpkg.EscapeString(slug) + `">` +
		`<input type="text" id="code-prompt" name="prompt" autocomplete="off" required ` +
		`placeholder="` + placeholder + `">` +
		`<button type="submit" id="code-go">` + label + `</button>` +
		`</form>` +
		`<p id="code-status" class="code-status text-muted text-xs">` +
		fmt.Sprintf("%d credits a turn. Every turn is a version, so nothing is lost.", cost) +
		`</p>`
}

// codeScript posts the turn and waits.
//
// A plain POST rather than a stream. A turn is up to three model calls over a
// whole document, so it is slow — tens of seconds — and the honest thing is to
// say that rather than to look instant and then hang. Streaming the attempts as
// they happen is the better version of this and is a bigger change than the
// page; the status line is written so that it can become one without moving.
//
// Wrapped in a function, and every binding inside it: soft navigation swaps
// #content and re-runs the scripts in it against the same document, so a
// top-level const here throws "already declared" the second time somebody
// arrives. See test/rerun_test.go.
func codeScript() string {
	return `<script>
(function () {
  var f = document.getElementById('code-form');
  if (!f) return;
  var box = document.getElementById('code-prompt');
  var go = document.getElementById('code-go');
  var status = document.getElementById('code-status');
  var appId = document.getElementById('code-app');

  f.addEventListener('submit', function (e) {
    e.preventDefault();
    var said = (box.value || '').trim();
    if (!said) return;

    go.disabled = true;
    box.disabled = true;
    status.textContent = appId.value
      ? 'Changing it, then running the checks. This takes a moment.'
      : 'Writing it, then running the checks. This takes a moment.';

    fetch('/code', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      credentials: 'same-origin',
      body: JSON.stringify({prompt: said, app: appId.value})
    }).then(function (r) {
      return r.json().then(function (j) { return {ok: r.ok, body: j}; });
    }).then(function (res) {
      if (!res.ok) {
        go.disabled = false;
        box.disabled = false;
        status.textContent = (res.body && (res.body.error || res.body.message)) ||
          'That did not work, and the reason did not come back.';
        return;
      }
      // The app is the state, so the page is reloaded on it rather than
      // patched: the transcript, the preview and the box all come from the
      // stored app, and rebuilding them here would be a second renderer.
      window.location = '/code?app=' + encodeURIComponent(res.body.app);
    }).catch(function () {
      go.disabled = false;
      box.disabled = false;
      status.textContent = 'The connection dropped before it finished.';
    });
  });
})();
</script>`
}
