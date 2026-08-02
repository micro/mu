package api

import (
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
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
	b.WriteString(`<div class="card"><h2>Tools</h2>`)
	b.WriteString(`<p class="card-desc">Everything an agent can do here. Point any MCP client at ` +
		`<code>/mcp</code> and all of it is available — no API key, no account. ` +
		`Metered tools show the price per call; the rest are included. ` +
		`Your first 10 calls per wallet are free.</p>`)
	b.WriteString(`<p class="card-desc"><a href="/mcp">MCP endpoint &amp; playground →</a> &nbsp;·&nbsp; ` +
		`<a href="/api">REST endpoints →</a> &nbsp;·&nbsp; <a href="/agents">How paying works →</a></p>`)
	b.WriteString(`</div>`)

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
@media only screen and (max-width:600px){.tool-grid{grid-template-columns:1fr}}
</style>`
