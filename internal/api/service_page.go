package api

// A service's page, derived rather than written.
//
// Thirty-two services carried 22,000 lines of hand-written HTML between them —
// a third of everything under service/ — and every one of those pages was built
// once, in the sitting that built the service, and never returned to. That is
// not a presentation layer. It is thirty-two private renderings, and it is why
// the site does not look like one product.
//
// The contract for showing a service already existed and nothing followed it.
// From internal/service/spec.go, on Card:
//
//	A card is a view, not a widget. It renders and it links; it does not hold
//	state or take input. Anything that does belongs in an app, which already
//	has a sandbox and a security boundary — there is no reason for a second one.
//
// So this is the page that contract implies: what the service is, what it knows
// right now, what it can be asked, and how to ask it. Four things, all read off
// the Spec and the tool registry, none of them typed out per service.
//
// # What this is not for
//
// A service you *do* something in. Mail, blog, notes, files, apps, chat — you
// compose, you write, you upload, you run. Those pages are applications and
// they pass the test that settled this: if all you could do was look at blogs,
// it would not be a blog service. They keep what they have.
//
// This is for the other kind, which is most of them: a service you look at and
// leave. Weather, markets, transit, flights, prayer times. There the page was
// only ever a bigger version of the card, with a location box on top and
// nothing underneath the card could not say.
//
// # Why not delete the page entirely
//
// Because the page is the demonstration. "The tools are real" is the whole
// argument this instance makes, and a page showing today's actual forecast is
// the proof — for a reader and for a search engine, since most of these are
// public. What was never load-bearing is that the proof be hand-built.

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// AgentPrompts is the agent that specialises in this service and what is worth
// asking it, filled in by the server.
//
// A hook rather than an import, and this is the one place in this file where
// that is not a compromise: agent/ consumes tools and internal/api serves them,
// so the dependency only runs one way and a door onto the catalogue must not
// import the thing that reads it. Empty path means there is no such agent.
var AgentPrompts func(service string) (path string, examples []string)

// ServicePage renders one service: its card, its tools, its doors.
//
// Takes the Spec rather than a name so a service calls it with its own — the
// declaration is already in that package and a lookup by string would be a
// second way to get it wrong.
func ServicePage(w http.ResponseWriter, r *http.Request, spec service.Spec) {
	app.Respond(w, r, app.Response{
		Title:       spec.NavLabel(),
		Description: spec.Description,
		HTML:        servicePage(spec),
	})
}

func servicePage(spec service.Spec) string {
	var b strings.Builder
	b.WriteString(`<div class="svc-page">`)
	b.WriteString(`<p class="svc-lead">` + html.EscapeString(spec.Description) + `</p>`)

	// What it knows right now. The whole page, really — everything below is
	// about how to ask for more of it.
	if spec.Card != nil {
		b.WriteString(`<div class="svc-card">` + spec.Card().HTML + `</div>`)
	}

	// Somewhere to go for a person, which the card cannot always be.
	//
	// This is what rendering the derived page first showed: a signed-out
	// visitor to /weather gets "Log in for weather" and a list of tool names,
	// which is a real answer for a developer and a dead end for everybody
	// else. The old hand-written page had a line pointing at the agent. It was
	// right, and it was right on one page out of thirty-two — this is the same
	// line, derived, on all of them.
	b.WriteString(askTheAgent(spec.Name))

	b.WriteString(serviceToolList(spec.Name))

	b.WriteString(`<p class="svc-doors">Every one of these is callable — by your agent, ` +
		`by an agent you point at this instance over <a href="/mcp">MCP</a>, or by a program ` +
		`over <a href="/api">REST</a>. Same method, same answer, same price.</p>`)

	b.WriteString(`</div>`)
	return b.String()
}

// serviceToolList is what this service can be asked, one row per tool.
//
// Derived from the registry rather than from the Spec's own Endpoints, because
// the tool is what a caller actually names and the two are not quite the same
// list — an endpoint can be REST-only, and an alias resolves without appearing.
// This is the list you can act on.
func serviceToolList(name string) string {
	var rows strings.Builder
	n := 0
	for _, t := range Tools() {
		if t.RESTOnly {
			continue
		}
		prefix, _, ok := strings.Cut(t.Name, "_")
		if !ok || prefix != name {
			continue
		}
		n++
		cost := 0
		if t.WalletOp != "" {
			cost = quota.OperationCost(t.WalletOp)
		}
		price := `<span class="svc-free">Free</span>`
		if cost > 0 {
			price = `<span class="svc-price">` + strconv.Itoa(cost) + ` ` + creditWord(cost) + `</span>`
		}
		rows.WriteString(`<a class="svc-tool" href="/tools/` + html.EscapeString(t.Name) + `">` +
			`<span class="svc-tool-name">` + html.EscapeString(t.Name) + `</span>` +
			`<span class="svc-tool-desc">` + html.EscapeString(t.Description) + `</span>` +
			price + `</a>`)
	}
	if n == 0 {
		return ""
	}
	return `<h2 class="svc-h">What it can be asked</h2><div class="svc-tools">` +
		rows.String() + `</div>`
}

// askTheAgent offers the specialist, in its own words.
//
// The examples come from the registry, so they are the same four the agent's
// own page suggests — a service page that invented its own would be a second
// list to keep in step, and the first thing to go stale.
func askTheAgent(name string) string {
	if AgentPrompts == nil {
		return ""
	}
	path, examples := AgentPrompts(name)
	if path == "" || len(examples) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="svc-ask"><span class="svc-ask-lead">Or just ask — ` +
		`<a href="` + html.EscapeString(path) + `">the ` + html.EscapeString(name) +
		` agent</a> calls these for you:</span>`)
	for _, e := range examples {
		b.WriteString(`<a class="pill" href="` + html.EscapeString(path) +
			`?q=` + url.QueryEscape(e) + `">` + html.EscapeString(e) + `</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
