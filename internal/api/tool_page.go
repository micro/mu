package api

// One tool, at an address of its own.
//
// Clicking a tool in the catalogue went to /mcp#tool-<name> — a fragment on the
// playground, which is one long page of every tool with a JSON-RPC console at
// the top. You arrived somewhere else, scrolled into the middle of a list, and
// the page you had been reading was gone. Three lenses, and the smallest unit
// in all of them had no page.
//
// It should have one, and not only for the click. If the tool is the unit that
// composes — services derive tools, agents call tools, an app calls tools — then
// it is the thing that needs a URL you can send somebody: /tools/mail_send is
// what you paste into a message when you are telling another developer what
// this instance can do. A fragment on a playground is not that.
//
// Derived, like everything else on these pages. This adds no list: the tool
// comes from api.Tools(), the price from internal/quota, the service from the
// name's own prefix. See tool/derive.go.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// ToolPageHandler serves /tools/<name>.
func ToolPageHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/tools"), "/")
	if name == "" || strings.Contains(name, "/") {
		ToolsPageHandler(w, r)
		return
	}

	var found *Tool
	for _, t := range Tools() {
		if t.Name == name {
			found = &t
			break
		}
		// An old name still resolves, because links to it exist and a rename
		// that breaks the address is a rename that costs more than it saved.
		for _, a := range t.Aliases {
			if a == name {
				found = &t
				break
			}
		}
	}
	if found == nil {
		app.NotFound(w, r, "no tool called "+name)
		return
	}

	app.Respond(w, r, app.Response{
		Title:       found.Name,
		Description: found.Description,
		HTML:        toolPage(*found),
	})
}

// toolPage is what one tool is: what it does, what it costs, what it takes, what
// runs it, and how to call it.
func toolPage(t Tool) string {
	var b strings.Builder
	b.WriteString(`<div class="tool-page">`)
	b.WriteString(app.Actions(app.TextLink("← Tools", "/tools")))

	b.WriteString(`<h1 class="tool-name">` + html.EscapeString(t.Name) + `</h1>`)
	b.WriteString(`<p class="tool-lead">` + html.EscapeString(t.Description) + `</p>`)

	// What it costs, said as a sentence rather than a badge. A number in the
	// corner is something to look up; this is the thing somebody is deciding.
	cost := 0
	if t.WalletOp != "" {
		cost = quota.OperationCost(t.WalletOp)
	}
	b.WriteString(`<p class="tool-cost">`)
	if cost > 0 {
		b.WriteString(`<b>` + strconv.Itoa(cost) + ` ` + creditWord(cost) + `</b> per call, ` +
			`from your balance — or paid per request in USDC with no account at all.`)
	} else {
		b.WriteString(`<b>Free.</b> This one only touches this instance's own storage, ` +
			`so there is nothing to charge for.`)
	}
	b.WriteString(`</p>`)

	// What runs it. The point of the page: a tool is derived from a service,
	// and the service is where you find out what it actually does.
	if svc := serviceBehind(t.Name); svc != nil {
		where := html.EscapeString(svc.Description)
		link := html.EscapeString(svc.NavLabel())
		if svc.Page != "" {
			link = `<a href="` + html.EscapeString(svc.Page) + `">` + link + `</a>`
		}
		b.WriteString(`<p class="tool-from">Runs on ` + link + ` — ` + where + `.</p>`)
	}

	if len(t.Params) > 0 {
		b.WriteString(`<h2 class="tool-h">Arguments</h2><table class="tool-args">`)
		b.WriteString(`<tr><th>Name</th><th>Type</th><th>What it is</th></tr>`)
		for _, p := range t.Params {
			req := ""
			if p.Required {
				req = `<span class="tool-req" title="Required">*</span>`
			}
			b.WriteString(`<tr><td><code>` + html.EscapeString(p.Name) + `</code>` + req +
				`</td><td>` + html.EscapeString(p.Type) + `</td><td>` +
				html.EscapeString(p.Description) + `</td></tr>`)
		}
		b.WriteString(`</table>`)
	}

	// Three doors, one call. Worth showing together on the page about one tool,
	// because the fact that they are the same thing is the architecture and it
	// is not obvious from any single one of them.
	b.WriteString(`<h2 class="tool-h">Calling it</h2>`)
	b.WriteString(`<p class="tool-note">Three doors onto the same method. ` +
		`<a href="/mcp">MCP</a> is what an agent speaks, ` +
		`<a href="/api">REST</a> is what a program speaks, and the answer is identical.</p>`)
	b.WriteString(`<pre class="tool-call">` + html.EscapeString(exampleRequest(t)) + `</pre>`)
	if t.Path != "" {
		method := t.Method
		if method == "" {
			method = "POST"
		}
		b.WriteString(`<pre class="tool-call">` + html.EscapeString(method+" "+t.Path) + `</pre>`)
	}
	b.WriteString(`<p class="tool-note">Try it in the ` +
		`<a href="/mcp#tool-` + html.EscapeString(t.Name) + `">playground</a>.</p>`)

	b.WriteString(`</div>`)
	return b.String()
}

// serviceBehind is which service a tool was derived from.
//
// The prefix, because that is the rule rather than a lookup table: a tool is
// named service_method and TestNoMethodRepeatsItsService holds it. A tool whose
// prefix names nothing is one somebody registered by hand, which is the thing
// CLAUDE.md says cannot happen — so nil here is a finding, not a fallback.
func serviceBehind(tool string) *service.Spec {
	name, _, ok := strings.Cut(tool, "_")
	if !ok {
		return nil
	}
	sp, found := service.SpecFor(name)
	if !found {
		return nil
	}
	return &sp
}
