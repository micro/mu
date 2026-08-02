package api

import (
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/wallet"
)

// ToolsPageHandler renders the tool catalogue: everything an agent pointed at
// this instance can do, grouped by service, with what each call costs.
//
// This is the browse, not the reference. An agent never reads it — it calls
// tools/list and gets the schemas. The reader is a person deciding whether to
// point an agent here at all, and that decision is "what is in it" and "what
// does it cost", not parameter types. Each card links through to its entry on
// /mcp, which already carries the schema, an example request and a playground.
func ToolsPageHandler(w http.ResponseWriter, r *http.Request) {
	groups := groupTools()

	var b strings.Builder
	b.WriteString(connectSection(r))

	for _, g := range groups {
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

	b.WriteString(toolsPageCSS)
	app.Respond(w, r, app.Response{
		Title:       "Tools",
		Description: "Every tool an agent can call on this instance, with what each one costs",
		HTML:        b.String(),
	})
}

// connectSection is the step the page was missing: how to actually point an
// agent at this instance.
//
// Landing, browsing the catalogue and then having nowhere to go was the gap —
// someone convinced by "tools for agents" had no next move. Getting a token and
// pasting one config block is that move, and it leads because it is the one a
// person signing in will take. Paying per call with no account is the
// alternative, not the headline.
func connectSection(r *http.Request) string {
	base := app.BaseURL(r)
	_, acc := auth.TrySession(r)

	cfg := `{
  "mcpServers": {
    "mu": {
      "url": "` + base + `/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}`

	var b strings.Builder
	b.WriteString(`<div class="card" id="connect">`)
	b.WriteString(`<span class="card-title">Connect your agent</span>`)

	if acc != nil {
		b.WriteString(`<p class="card-desc">Create a token, paste it into your MCP client, and every tool below is available to it.</p>`)
		b.WriteString(`<p><a class="connect-cta" href="/token">Create a token →</a></p>`)
	} else {
		b.WriteString(`<p class="card-desc">Create an account and a token, paste it into your MCP client, and every tool below is available to it. Calls are charged to your credits.</p>`)
		b.WriteString(`<p><a class="connect-cta" href="/signup">Create an account →</a> <span class="connect-note">then get a token at <a href="/token">/token</a></span></p>`)
	}

	b.WriteString(`<pre class="connect-cfg">` + html.EscapeString(cfg) + `</pre>`)
	b.WriteString(`<p class="card-desc">Works with Claude Desktop, Cursor, or anything that speaks ` +
		`<a href="https://modelcontextprotocol.io">MCP</a>. Try it first in the ` +
		`<a href="/mcp">playground</a>, or over <a href="/api">plain HTTP</a>.</p>`)
	b.WriteString(`<p class="card-desc connect-alt">No account? An agent can pay per call in USDC instead — ` +
		`it sends no token, gets an <code>HTTP 402</code> with a price, pays and retries. ` +
		`<a href="/#paying">How paying works →</a></p>`)
	b.WriteString(`</div>`)
	return b.String()
}

// priceLabel is what a call costs: a stablecoin price when x402 is configured,
// otherwise credits, otherwise nothing to pay.
func priceLabel(t Tool) string {
	if t.WalletOp == "" {
		return `<span class="free">Included</span>`
	}
	if p := wallet.X402PriceFor(t.WalletOp); p != "" {
		return `<b>` + html.EscapeString(p) + `</b> / call`
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
.connect-alt{color:#888;font-size:13px;border-top:1px solid #eee;padding-top:10px;margin-top:12px}
@media only screen and (max-width:600px){.tool-grid{grid-template-columns:1fr}}
</style>`
