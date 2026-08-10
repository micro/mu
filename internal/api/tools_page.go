package api

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"mu/billing"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// ToolsPageHandler renders the catalogue of what this instance runs, through
// one of two lenses.
//
// One catalogue, not two pages. /services is what a person can open, /tools is
// what an agent can call — the same Specs at two granularities, nineteen
// services against sixty-odd tools. Built as separate pages they would be two
// shop windows onto one inventory, and they would drift. Built as one page with
// a lens they cannot.
//
// The lens follows the door you came through, so neither audience has to pick
// a view before knowing what the views are.
//
// This is the browse, not the reference. An agent never reads it — it calls
// tools/list and gets the schemas. The reader is a person deciding whether to
// point an agent here at all, and that decision is "what is in it" and "what
// does it cost", not parameter types. Each card links through to its entry on
// /mcp, which already carries the schema, an example request and a playground.
func ToolsPageHandler(w http.ResponseWriter, r *http.Request) {
	// Pinning happens here because this is where you are when you find out a
	// service exists. A separate settings page for it would mean discovering
	// something and then going somewhere else to keep it.
	if r.Method == http.MethodPost {
		togglePin(w, r)
		return
	}

	services := strings.HasPrefix(r.URL.Path, "/services")
	if v := r.URL.Query().Get("view"); v != "" {
		services = v == "services"
	}

	// Connecting belongs to the tools lens only, and it leads there: someone on
	// /tools came to point an agent at this instance, and a grid of sixty above
	// the endpoint makes them scroll back up for it.
	//
	// /services answers a different question — what does this thing run — and it
	// answers it in full with the grid. An MCP config underneath was the tools
	// page's call to action wearing the services page as a second home; the
	// sidebar already links Tools for anyone who came to connect.
	//
	// No tabs between the two either, for the same reason: the sidebar links
	// both, and a switch that duplicates the navigation beside it is one more
	// thing to read and one more place for the two to disagree.
	// Each page says its part in one sentence, because the three of them are
	// one sentence: an agent acts for you, tools are what it can call, services
	// are what runs behind them. Read cold, "Services" beside "Tools" looks like
	// two names for the same list — the difference is which side of the call you
	// are standing on, and nothing on either page said so.
	var b strings.Builder
	if services {
		b.WriteString(`<p class="lens-lead">The back end for your agents. Every capability this ` +
			`instance runs — the thing behind each tool, with a page you can open and use yourself. ` +
			`Pin the ones you want in the sidebar.</p>`)
		b.WriteString(serviceGrid(r))
	} else {
		b.WriteString(`<p class="lens-lead">What an agent can call. Point one at this instance, ` +
			`give it a token, and these are the tools it gets — priced per call, no account needed ` +
			`if it pays.</p>`)
		b.WriteString(connectSection(r))
		b.WriteString(toolGrid())
	}

	b.WriteString(toolsPageCSS)

	title, desc := "Tools", "Every tool an agent can call on this instance, with what each one costs"
	if services {
		title, desc = "Services", "Everything this instance runs, and what each one is"
	}
	app.Respond(w, r, app.Response{Title: title, Description: desc, HTML: b.String()})
}

// togglePin adds or removes a service from the caller's sidebar and returns
// them to where they were. Signed-out callers are sent to sign in: a pin is a
// preference and there is nowhere to keep one without an account.
//
// Two things it used to get wrong. It sent everyone to /services however they
// arrived, so pinning from /tools?view=services moved you to a different URL
// mid-task. And it posted one field meaning "flip this", so two tabs on the
// same page disagreed: pin from each and the second flip undoes the first, and
// you have pressed pin twice and ended with it off. It says which it means now,
// and comes back to the page you pressed it on.
func togglePin(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		http.Redirect(w, r, "/login?redirect="+url.QueryEscape(app.ReturnTo(r, "/services")),
			http.StatusSeeOther)
		return
	}
	pin, unpin := r.FormValue("pin"), r.FormValue("unpin")
	name := strings.ToLower(strings.TrimSpace(pin + unpin))
	if _, ok := service.SpecFor(name); ok {
		if unpin != "" {
			acc.Unpin(name)
		} else {
			acc.Pin(name)
		}
		auth.UpdateAccount(acc)
	}
	http.Redirect(w, r, app.ReturnTo(r, "/services"), http.StatusSeeOther)
}

// pinControl is the star on a tile. It sits outside the tile's anchor because
// an interactive control inside a link is a control you cannot click without
// also following the link.
func pinControl(r *http.Request, name string, pinned bool) string {
	if _, acc := auth.TrySession(r); acc == nil {
		return ""
	}
	label, cls, field := "Pin to sidebar", "pin-btn", "pin"
	if pinned {
		label, cls, field = "Unpin from sidebar", "pin-btn pinned", "unpin"
	}
	return `<form method="POST" action="/services" class="pin-form">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<input type="hidden" name="return" value="` + html.EscapeString(r.URL.RequestURI()) + `">` +
		`<input type="hidden" name="` + field + `" value="` + html.EscapeString(name) + `">` +
		`<button type="submit" class="` + cls + `" title="` + label + `" aria-label="` + label + `">` +
		`<svg width="15" height="15" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8" ` +
		`stroke-linejoin="round"><polygon points="12,3 14.6,9 21,9.5 16.2,13.8 17.6,20 12,16.8 6.4,20 7.8,13.8 3,9.5 9.4,9"/></svg>` +
		`</button></form>`
}

// serviceGrid is the person's lens: one tile per service you can open.
//
// A grid rather than the list this replaced in the sidebar. Twenty tiles are
// scanned in a look; twenty list rows are read one at a time, and stop being
// read.
//
// Headless services are not here. They were briefly, on the argument that a
// service with no page is still something this instance runs — but a tile you
// cannot open, in a lens whose promise is "what you can open", is not proof of
// anything. Index had no icon either, because a service with no page never
// needed one, and it linked to /mcp, which is not what a tile that looks like
// the others should do. They are in the Tools lens, where index_search is a
// thing an agent can actually call.
//
// Each tile carries a pin, which is how a service gets into the sidebar. The
// sidebar shows what you chose; this shows everything there is to choose.
func serviceGrid(r *http.Request) string {
	counts := map[string]int{}
	for _, g := range groupTools() {
		counts[strings.ToLower(g.Label)] += len(g.Tools)
	}

	isPinned := map[string]bool{}
	if _, acc := auth.TrySession(r); acc != nil {
		for _, n := range acc.PinnedServices() {
			isPinned[n] = true
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="tool-grid service-grid">`)
	for _, s := range service.Nav() {
		b.WriteString(`<div class="service-tile-wrap">`)
		b.WriteString(`<a class="tool-tile service-tile" href="` + html.EscapeString(s.Page) + `">`)
		b.WriteString(`<span class="service-tile-head">` +
			`<img src="/` + html.EscapeString(s.NavIcon()) + `?` + app.Version + `" alt="">` +
			`<span class="tool-tile-name">` + html.EscapeString(s.NavLabel()) + `</span></span>`)
		b.WriteString(`<span class="tool-tile-desc">` + html.EscapeString(s.Description) + `</span>`)

		meta := ""
		if n := counts[strings.ToLower(s.NavLabel())]; n > 0 {
			meta = strconv.Itoa(n) + " tool"
			if n != 1 {
				meta += "s"
			}
		}
		b.WriteString(`<span class="tool-tile-price">` + html.EscapeString(meta) + `</span>`)
		b.WriteString(`</a>`)
		b.WriteString(pinControl(r, s.Name, isPinned[s.Name]))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// toolGrid is the agent's lens: every callable tool, grouped by service, priced.
func toolGrid() string {
	var b strings.Builder
	for _, g := range groupTools() {
		b.WriteString(`<div class="tool-group">`)
		b.WriteString(`<h3 class="tool-group-title">` + html.EscapeString(g.Label) + `</h3>`)
		b.WriteString(`<div class="tool-grid">`)
		for _, t := range g.Tools {
			b.WriteString(`<a class="tool-tile" href="/mcp#tool-` + html.EscapeString(t.Name) + `">`)
			b.WriteString(`<span class="tool-tile-name">` + html.EscapeString(t.Name) + `</span>`)
			b.WriteString(`<span class="tool-tile-desc">` + html.EscapeString(clipDesc(t.Description)) + `</span>`)
			b.WriteString(`<span class="tool-tile-price">` + priceLabel(t) + `</span>`)
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div></div>`)
	}
	return b.String()
}

// connectSection is the step the page was missing: how to actually point an
// agent at this instance.
//
// Landing, browsing the catalogue and then having nowhere to go was the gap —
// someone convinced by "tools for agents" had no next move.
//
// Two paths, split by client rather than by preference, because that is how the
// world actually divides:
//
//   - Cursor and other config-file clients read a url and headers out of
//     mcp.json, so they take a token. That is the shape an agent credential
//     really has: an agent has no account to sign into, somebody issues it one.
//   - Claude Desktop does not. Its claude_desktop_config.json only validates
//     stdio servers — command and args — so a url pasted there silently does
//     nothing. Remote servers go in through Settings > Connectors, which runs
//     OAuth: the client registers itself against /oauth/register, opens a
//     browser, and the *person* signs in. There is nowhere to put a token.
//
// This page used to show the config alone and call OAuth the fallback for a
// client that "can't sign itself in". Both halves were wrong. Nothing signs
// itself in — a browser opens and you do — and for the client most likely to
// arrive here, OAuth is not the fallback, it is the only way in. Someone
// following the old copy would have edited a JSON file and watched it do
// nothing.
//
// The env reference in the token example is deliberate: mcp.json gets committed,
// and Cursor's own documentation warns against inlining a live credential.
func connectSection(r *http.Request) string {
	base := app.BaseURL(r)
	_, acc := auth.TrySession(r)

	cfg := `{
  "mcpServers": {
    "mu": {
      "url": "` + base + `/mcp",
      "headers": {
        "Authorization": "Bearer ${env:MU_TOKEN}"
      }
    }
  }
}`

	var b strings.Builder
	b.WriteString(`<div class="card" id="connect">`)
	b.WriteString(`<span class="card-title">Connect your agent</span>`)

	if acc == nil {
		b.WriteString(`<p class="card-desc">Every tool below becomes available to your agent, ` +
			`and calls are charged to your credits. You need an account either way.</p>`)
		b.WriteString(`<p><a class="connect-cta" href="/signup">Create an account →</a> ` +
			`<span class="connect-note">it is the same account you sign into the app with</span></p>`)
	} else {
		b.WriteString(`<p class="card-desc">Every tool below becomes available to your agent, ` +
			`and calls are charged to your credits. Pick the way your client connects.</p>`)
	}

	// Cursor and anything else that reads a config file.
	b.WriteString(`<div class="connect-way">`)
	b.WriteString(`<h4>Cursor, and clients with a config file</h4>`)
	b.WriteString(`<p class="card-desc">Add this to <code>~/.cursor/mcp.json</code>, ` +
		`with your token in <code>MU_TOKEN</code>.</p>`)
	b.WriteString(`<pre class="connect-cfg">` + html.EscapeString(cfg) + `</pre>`)
	if acc != nil {
		b.WriteString(`<p><a class="connect-cta" href="/token">Create a token →</a></p>`)
	}
	b.WriteString(`</div>`)

	// Claude Desktop, where the config file is not the way in.
	b.WriteString(`<div class="connect-way">`)
	b.WriteString(`<h4>Claude Desktop</h4>`)
	b.WriteString(`<p class="card-desc">Settings &rarr; Connectors &rarr; Add custom connector, ` +
		`and paste <code>` + html.EscapeString(base) + `/mcp</code>. It opens a browser and asks you ` +
		`to sign in to Mu — no token needed. Pasting the URL into ` +
		`<code>claude_desktop_config.json</code> will not work: that file only takes local ` +
		`command-line servers.</p>`)
	b.WriteString(`</div>`)

	b.WriteString(`<p class="card-desc">Anything else that speaks ` +
		`<a href="https://modelcontextprotocol.io">MCP</a> takes one or the other. Try a call ` +
		`first in the <a href="/mcp">playground</a>.</p>`)
	b.WriteString(scopePicker(base))
	b.WriteString(`</div>`)
	return b.String()
}

// scopePicker builds a scoped endpoint URL.
//
// Every tool on one endpoint is right for the server and wrong for a session:
// the definitions are sent to the model on every turn whether or not any of
// them could help. Picking a few services here produces a URL that lists only
// those, which is the difference between an agent weighing sixty tools and
// weighing six.
//
// The checkboxes are built from the registry, so a new service appears here the
// moment it registers.
func scopePicker(base string) string {
	services := ScopeServices()
	if len(services) == 0 {
		return ""
	}
	sort.Strings(services)

	var b strings.Builder
	b.WriteString(`<details class="scope"><summary>Only want some of it?</summary>`)
	b.WriteString(`<p class="card-desc">Pick the services this connection should see. ` +
		`Everything else stays callable, it just isn't in the list your agent reads every turn.</p>`)
	b.WriteString(`<div class="scope-grid">`)
	for _, s := range services {
		label := service.Label(s)
		b.WriteString(`<label class="scope-item"><input type="checkbox" value="` +
			html.EscapeString(s) + `" onchange="muScope()"> ` + html.EscapeString(label) + `</label>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<pre class="connect-cfg" id="scope-url">` + html.EscapeString(base) + `/mcp</pre>`)
	b.WriteString(`<script>
function muScope(){
  var picked=[].slice.call(document.querySelectorAll('.scope-item input:checked')).map(function(i){return i.value});
  var out=document.getElementById('scope-url');
  out.textContent=` + "`" + html.EscapeString(base) + `/mcp` + "`" + `+(picked.length?'?tools='+picked.join(','):'');
}
</script>`)
	b.WriteString(`</details>`)
	return b.String()
}

// priceLabel is what a call costs, in the one currency the product has:
// credits. A tool that only touches this instance's own storage costs us
// nothing to run and is included.
func priceLabel(t Tool) string {
	if t.WalletOp == "" {
		return `<span class="free">Included</span>`
	}
	n := billing.GetOperationCost(t.WalletOp)
	if n <= 0 {
		return `<span class="free">Included</span>`
	}
	return `<b>` + strconv.Itoa(n) + `</b> ` + creditWord(n)
}

// creditWord keeps "1 credit" from reading as "1 credits" wherever a price is
// rendered.
func creditWord(n int) string {
	if n == 1 {
		return "credit"
	}
	return "credits"
}

// clipDesc keeps a tile to one readable line. The full text is on /mcp.
func clipDesc(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".\n"); i > 30 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 96 {
		return strings.TrimSpace(string(r[:96])) + "…"
	}
	return s
}

type toolGroup struct {
	Label string
	Tools []Tool
}

// groupTools buckets tools by the service in front of the underscore, which is
// what the name is derived from. A tool with no service behind it — quran, pay,
// the platform verbs — falls into Platform rather than inventing a group.
func groupTools() []toolGroup {
	byService := map[string][]Tool{}
	for _, t := range mcpTools() {
		byService[serviceOf(t.Name)] = append(byService[serviceOf(t.Name)], t)
	}

	names := make([]string, 0, len(byService))
	for n := range byService {
		if n != "" {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool { return service.Label(names[i]) < service.Label(names[j]) })

	out := make([]toolGroup, 0, len(byService))
	for _, n := range names {
		out = append(out, toolGroup{Label: service.Label(n), Tools: byService[n]})
	}
	if rest := byService[""]; len(rest) > 0 {
		out = append(out, toolGroup{Label: "Platform", Tools: rest})
	}
	return out
}

// serviceOf returns the registered service a tool belongs to, or "" when the
// tool has none.
func serviceOf(tool string) string {
	name, _, ok := strings.Cut(tool, "_")
	if !ok {
		name = tool
	}
	if _, known := service.SpecFor(name); known {
		return name
	}
	return ""
}

const toolsPageCSS = `<style>
.lens-lead{color:#666;font-size:14px;margin:0 0 18px;max-width:640px}
.service-tile-head{display:flex;align-items:center;gap:8px}
.service-tile-head img{width:18px;height:18px}
/* The wrapper exists so the pin can sit on the tile without sitting inside the
   link — a button inside an anchor cannot be pressed without also following it.
   The tile still fills the cell, so the whole card remains the hit target for
   opening the service. */
.service-tile-wrap{position:relative;display:flex}
.service-tile-wrap>.tool-tile{flex:1;padding-right:34px}
/* The pin sits on the tile here; .pin-btn itself is in mu.css, because /apps
   pins an app to home with the same control and a shared control cannot live
   in one page's style block. */
.pin-form{position:absolute;top:6px;right:6px;margin:0}

.tool-group{margin:0 0 26px}
.tool-group-title{font-size:12px;text-transform:uppercase;letter-spacing:.06em;color:#999;margin:0 0 10px}
.tool-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));gap:10px}
.tool-tile{display:flex;flex-direction:column;gap:4px;padding:12px 14px;border:1px solid #e5e5e5;
  border-radius:8px;background:#fff;text-decoration:none;color:inherit}
.tool-tile:hover{border-color:#bbb}
.tool-tile-name{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;font-weight:600;color:#111}
.tool-tile-desc{font-size:13px;color:#666;line-height:1.4}
.tool-tile-price{font-size:12px;color:#6b7280;font-variant-numeric:tabular-nums;margin-top:2px}
.tool-tile-price .free{color:#9ca3af}
/* .card a sets a dark colour and won on specificity, so the label went black on
   a black button. Scope the rule the same way to outrank it. */
.card a.connect-cta,.card a.connect-cta:visited{display:inline-block;background:#111;color:#fff;
  text-decoration:none;padding:9px 18px;border-radius:8px;font-weight:700;font-size:14px}
.card a.connect-cta:hover{background:#333;color:#fff}
.connect-note{font-size:13px;color:#888;margin-left:8px}
.connect-cfg{background:#f5f5f5;padding:10px 12px;font-size:12px;overflow-x:auto;border-radius:6px;margin:12px 0}
.connect-way{border-top:1px solid #eee;padding-top:12px;margin-top:14px}
.connect-way h4{margin:0 0 6px;font-size:14px}
.scope{margin-top:12px;border-top:1px solid #eee;padding-top:10px}
.scope summary{cursor:pointer;font-size:13px;font-weight:600;color:#555}
.scope-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:4px 12px;margin:10px 0}
.scope-item{font-size:13px;color:#444;display:flex;align-items:center;gap:6px}
@media only screen and (max-width:600px){.tool-grid{grid-template-columns:1fr}}
</style>`
