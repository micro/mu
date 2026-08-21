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
// The rendering lives in service_ref.go, because it turned out to be the same
// page /services/<name> wants: the card, every method with its arguments and
// its price, and a form that calls it. So a derived service has no page of its
// own at all — its Page *is* /services/<name>, and /weather and /hazards
// redirect there. There was briefly an api.ServicePage rendering a second,
// near-identical page at the old address, which is the drift this file exists
// to stop, one level up.
//
// What stays here is the rule below for which services get it, and the offer
// of the specialist agent that goes on every one.
//
// # When a service keeps its page
//
// The rule, arrived at by converting one and getting the next one wrong: a
// service keeps its page when the page shows something the card cannot. There
// are exactly three reasons that is ever true, and they are checkable rather
// than a matter of taste.
//
//  1. **It is your data.** Thirteen services are Scoped — mail, notes, files,
//     contacts, tasks, images and the rest. A card is shared by definition, so
//     it can never show one person's mail. These are applications and always
//     were.
//
//  2. **It takes an argument to say anything.** A barcode, a search, a
//     callsign, a category, a stop. food, places, web, markets, routes,
//     transit, flights. A form is not a card, and the answer changes with what
//     you type.
//
//  3. **It is browsable.** news, video, blog, social, apps — many items, paged,
//     read one at a time. A card shows the top four and that is all it should.
//
// Fail all three and the page was a bigger version of the card. Two services
// fail all three, and both have gone: weather and hazards.
//
// That is a much smaller answer than the one this file started with, and it is
// the true one. The 22,000 lines of hand-written HTML under service/ do not
// come out this way; most of them are earning their place by rule 1 or rule 3.
//
// What actually improved everything was the other half — see service.Viewer. A
// card that knows who is looking is better on Home, in an agent's answer, on
// this page and in an email, which is fourteen services rather than two pages.
//
// # Why not delete the page entirely
//
// Because the page is the demonstration. "The tools are real" is the whole
// argument this instance makes, and a page showing today's actual forecast is
// the proof — for a reader and for a search engine, since most of these are
// public. What was never load-bearing is that the proof be hand-built.

import (
	"html"
	"net/url"
	"strings"
)

// AgentPrompts is the agent that specialises in this service and what is worth
// asking it, filled in by the server.
//
// A hook rather than an import, and this is the one place in this file where
// that is not a compromise: agent/ consumes tools and internal/api serves them,
// so the dependency only runs one way and a door onto the catalogue must not
// import the thing that reads it. Empty path means there is no such agent.
var AgentPrompts func(service string) (path string, examples []string)

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
