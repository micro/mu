package api

// The reference somebody reads before writing a client.
//
// /api was a redirect to /mcp, on the argument that two documented doors is a
// decision the reader has to make before they can start. That argument was
// right when the second door was another way for an *agent* to call tools. It
// is wrong now: a person building a desktop client, a mobile app or a front end
// is not choosing between two things, they are looking for the one that fits
// what they are building, and being sent to a tool-calling protocol reads as
// "not for you".
//
// So the two pages say different things and link to each other. /mcp is for
// something that plans over a catalogue. This is for something that already
// knows what it wants to call.
//
// Everything here is derived. The methods, their arguments and types, what each
// costs and whether it needs an account all come off the specs — the same
// declaration the tool, the nav entry and the price come from. A reference that
// is written by hand is a reference that is wrong by the second release.

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// RESTPageHandler serves /api.
func RESTPageHandler(w http.ResponseWriter, r *http.Request) {
	base := app.BaseURL(r)

	var b strings.Builder

	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>HTTP API</h2>`)
	b.WriteString(`<p class="card-desc">Every service method as a plain HTTP call. ` +
		`JSON in, JSON out, one URL per method.</p>`)
	b.WriteString(`<pre style="background:#f5f5f5;padding:8px;font-size:13px;overflow-x:auto">` +
		html.EscapeString("GET  "+base+RESTPrefix+"<service>/<method>?arg=value\n"+
			"POST "+base+RESTPrefix+"<service>/<method>   {\"arg\":\"value\"}") +
		`</pre>`)
	b.WriteString(`<p class="card-meta">` +
		`<a href="` + RESTRoot + `">Machine-readable catalogue &rarr;</a> &middot; ` +
		`<a href="/token">Get a token</a> &middot; ` +
		`<a href="/pricing">What calls cost</a></p>`)
	b.WriteString(`<p class="card-meta">Building an AI agent instead? ` +
		`<a href="/mcp">The MCP endpoint</a> serves the same methods as tools, with a ` +
		`catalogue to plan over.</p>`)
	b.WriteString(`</div>`)

	b.WriteString(restAuthCard(base))
	b.WriteString(restErrorsCard())
	b.WriteString(restReference(base))

	app.Respond(w, r, app.Response{
		Title:       "API",
		Description: "HTTP API — every service method as a plain HTTP call",
		HTML:        b.String(),
	})
}

func restAuthCard(base string) string {
	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Authentication</h3>`)
	b.WriteString(`<p class="card-desc">Public methods need nothing. Anything holding ` +
		`your own data needs to know who you are.</p>`)

	b.WriteString(`<p><b>A token.</b> Make one at <a href="/token">/token</a> and send it ` +
		`as a header. A token can be scoped to named services, and the scope is enforced ` +
		`on every call.</p>`)
	b.WriteString(`<pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto">` +
		html.EscapeString(`curl -H "Authorization: Bearer $MU_TOKEN" \
  `+base+RESTPrefix+`notes/list`) + `</pre>`)

	b.WriteString(`<p><b>Paying instead.</b> A metered method answers an unauthenticated ` +
		`call with <code>402</code> and an x402 challenge; pay it and the same request ` +
		`succeeds. No account, no signup — see <a href="/pricing">pricing</a>.</p>`)

	b.WriteString(`<p><b>From a browser.</b> A signed-in session works for reads. ` +
		`A <code>POST</code> resting on the session cookie also needs an ` +
		`<code>X-CSRF-Token</code> header, because a cookie alone does not say the ` +
		`request came from a page this instance served. A token does, so programs ` +
		`should send one.</p>`)
	b.WriteString(`</div>`)
	return b.String()
}

func restErrorsCard() string {
	rows := []struct{ code, when string }{
		{"200", "The call ran. The answer is in <code>result</code>."},
		{"400", "The arguments were wrong. The message says how."},
		{"401", "No caller. Send a token, or pay."},
		{"402", "This method is metered and nothing has paid for it. The challenge says how much."},
		{"403", "Identified, but not enough — a paid wallet on a method that needs an account, or a cookie-authenticated POST with no CSRF token."},
		{"404", "No such method."},
		{"405", "This method changes something, so it cannot be a GET."},
	}
	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Responses</h3>`)
	b.WriteString(`<p class="card-desc">A refusal says which kind it is, so a client knows ` +
		`whether to retry, re-authenticate or give up.</p>`)
	b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px">`)
	for _, row := range rows {
		b.WriteString(`<tr><td style="padding:4px 8px;vertical-align:top"><code>` + row.code +
			`</code></td><td style="padding:4px 8px">` + row.when + `</td></tr>`)
	}
	b.WriteString(`</table>`)
	b.WriteString(`<p class="card-meta">Errors are ` +
		html.EscapeString(`{"error":"..."}`) + `; success is ` +
		html.EscapeString(`{"result":"..."}`) + `.</p>`)
	b.WriteString(`</div>`)
	return b.String()
}

// restMethod is one callable method, flattened from a Spec for rendering.
type restMethod struct {
	Service     string
	Method      string
	Tool        string
	Path        string
	Doc         string
	Cost        int
	Destructive bool
	NeedsAuth   bool
	Params      []ToolParam
}

// restMethods is every method that can be called here, in the order a reader
// wants them: by service, then alphabetically.
//
// Read from the specs and then matched against the derived tools, because the
// two can disagree — a Spec declares an endpoint, and whether a tool came out
// of it is tool/derive.go's answer. Listing a method with no tool behind it
// would publish a URL that answers 404.
func restMethods() []restMethod {
	var out []restMethod
	for _, sp := range service.Specs() {
		for name, ep := range sp.Endpoints {
			tool := sp.Name + "_" + strings.ToLower(name)
			t, ok := ToolByName(tool)
			if !ok {
				continue
			}
			cost := 0
			if ep.Cost != "" {
				cost = quota.GetOperationCost(ep.Cost)
			}
			out = append(out, restMethod{
				Service:     sp.Name,
				Method:      name,
				Tool:        tool,
				Path:        RESTPrefix + sp.Name + "/" + strings.ToLower(name),
				Doc:         ep.Doc,
				Cost:        cost,
				Destructive: ep.Destructive,
				NeedsAuth:   ToolNeedsAuth(tool),
				Params:      t.Params,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// restReference renders the index and the per-method cards, in the same
// two-column shape /mcp uses so the two pages read as one reference.
func restReference(base string) string {
	methods := restMethods()

	var nav strings.Builder
	nav.WriteString(`<nav class="ep-nav"><div class="ep-nav-title">Methods</div>`)
	last := ""
	for _, m := range methods {
		if m.Service != last {
			nav.WriteString(`<div class="ep-nav-title">` + html.EscapeString(m.Service) + `</div>`)
			last = m.Service
		}
		price := ""
		if m.Cost > 0 {
			price = `<span class="ep-price">` + strconv.Itoa(m.Cost) + `</span>`
		}
		nav.WriteString(`<a href="#api-` + html.EscapeString(m.Tool) + `">` +
			html.EscapeString(strings.ToLower(m.Method)) + price + `</a>`)
	}
	nav.WriteString(`</nav>`)

	var cards strings.Builder
	for _, m := range methods {
		cards.WriteString(restMethodCard(m, base))
	}

	return `<div class="ep-layout">` + nav.String() +
		`<div class="ep-main">` + app.List(cards.String()) + `</div></div>`
}

func restMethodCard(m restMethod, base string) string {
	var b strings.Builder
	b.WriteString(`<div class="card" id="api-` + html.EscapeString(m.Tool) + `">`)

	if m.Cost > 0 {
		b.WriteString(`<span class="tool-price"><b>` + strconv.Itoa(m.Cost) + `</b> <span>` +
			creditWord(m.Cost) + `</span></span>`)
	}

	verb := "GET"
	if m.Destructive {
		verb = "POST"
	}
	b.WriteString(`<span class="card-title"><code>` + verb + ` ` + html.EscapeString(m.Path) + `</code></span>`)
	b.WriteString(app.Desc(m.Doc))

	var notes []string
	if m.NeedsAuth {
		notes = append(notes, "Needs an account.")
	}
	if m.Destructive {
		notes = append(notes, "Changes something, so <code>POST</code> only.")
	}
	if m.Cost > 0 {
		notes = append(notes, "Draws "+strconv.Itoa(m.Cost)+" "+creditWord(m.Cost)+
			" per call, or accepts payment.")
	}
	if len(notes) > 0 {
		b.WriteString(`<p class="card-meta">` + strings.Join(notes, " ") + `</p>`)
	}

	if len(m.Params) > 0 {
		b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px;margin:8px 0">`)
		b.WriteString(`<tr><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Argument</th>` +
			`<th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Type</th>` +
			`<th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Description</th></tr>`)
		for _, p := range m.Params {
			req := ""
			if p.Required {
				req = ` <span style="color:#e55">*</span>`
			}
			b.WriteString(fmt.Sprintf(
				`<tr><td style="padding:4px 8px"><code>%s</code>%s</td>`+
					`<td style="padding:4px 8px;color:#888">%s</td>`+
					`<td style="padding:4px 8px">%s</td></tr>`,
				html.EscapeString(p.Name), req,
				html.EscapeString(p.Type),
				html.EscapeString(p.Description)))
		}
		b.WriteString(`</table>`)
	}

	b.WriteString(`<pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto">` +
		html.EscapeString(restCurl(m, base)) + `</pre>`)
	b.WriteString(`</div>`)
	return b.String()
}

// restCurl writes the call out the way somebody would actually make it: a GET
// with a query string when the method only reads, a POST with a JSON body when
// it changes something, and the token header only when the method needs one.
func restCurl(m restMethod, base string) string {
	auth := ""
	if m.NeedsAuth || m.Cost > 0 {
		auth = " \\\n  -H \"Authorization: Bearer $MU_TOKEN\""
	}

	if m.Destructive {
		body, _ := json.Marshal(exampleArgs(m.Params))
		return "curl -X POST" + auth + " \\\n  -H \"Content-Type: application/json\" \\\n" +
			"  -d '" + string(body) + "' \\\n  " + base + m.Path
	}

	return "curl" + auth + " \\\n  \"" + base + m.Path + exampleQuery(m.Params) + "\""
}

// exampleArgs fills in the required arguments with something of the right type,
// so a reader can paste the line and see a real answer rather than a complaint
// about a missing field.
func exampleArgs(ps []ToolParam) map[string]any {
	args := map[string]any{}
	for _, p := range ps {
		if !p.Required {
			continue
		}
		switch p.Type {
		case "number":
			args[p.Name] = 1.5
		case "integer":
			args[p.Name] = 1
		case "boolean":
			args[p.Name] = true
		default:
			args[p.Name] = p.Name
		}
	}
	return args
}

func exampleQuery(ps []ToolParam) string {
	var parts []string
	for _, p := range ps {
		if !p.Required {
			continue
		}
		v := p.Name
		switch p.Type {
		case "number":
			v = "1.5"
		case "integer":
			v = "1"
		case "boolean":
			v = "true"
		}
		parts = append(parts, p.Name+"="+v)
	}
	if len(parts) == 0 {
		return ""
	}
	return "?" + strings.Join(parts, "&")
}
