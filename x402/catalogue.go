package x402

import (
	"fmt"
	"html"
	"mu/internal/api"
	"mu/internal/quota"
	x402 "mu/x402/payment"
	"strings"
)

func creditWord(n int) string {
	if n == 1 {
		return "credit"
	}
	return "credits"
}
func clipDesc(s string) string {
	r := []rune(s)
	if len(r) > 96 {
		return strings.TrimSpace(string(r[:96])) + "…"
	}
	return s
}
func cataloguePrice(t api.Tool) string {
	if !quota.Charging() {
		return "No usage charge"
	}
	cost := 0
	if t.WalletOp != "" {
		cost = quota.OperationCost(t.WalletOp)
	}
	if cost <= 0 {
		return "No usage charge"
	}
	if !x402.Enabled() {
		return fmt.Sprintf("%d %s", cost, creditWord(cost))
	}
	return fmt.Sprintf("$%.2f / call · %d %s", float64(cost)/100, cost, creditWord(cost))
}

// featuredToolsHTML links a small selection into the live catalogue.
func featuredToolsHTML() string {
	var b strings.Builder
	b.WriteString(`<div class="card-grid">`)
	for _, featured := range []struct{ name, label string }{{"web_search", "Web search"}, {"weather_forecast", "Weather"}, {"markets_list", "Markets"}, {"news_search", "News"}, {"places_search", "Places"}, {"text_translate", "Translation"}} {
		for _, t := range api.Tools() {
			if t.Name != featured.name || t.OperatorOnly || t.RESTOnly {
				continue
			}
			b.WriteString(`<a class="directory-row directory-content" href="/tools/` + html.EscapeString(t.Name) + `"><span class="directory-heading">` + html.EscapeString(featured.label) + `</span><span class="directory-description">` + html.EscapeString(clipDesc(t.Description)) + `</span><span class="tool-tile-price">` + cataloguePrice(t) + `</span></a>`)
			break
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

// toolPricesHTML uses the same registry and rates as the call gate.
func toolPricesHTML() string {
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Tool</th><th>Per call</th></tr></thead><tbody>`)
	for _, g := range api.Groups() {
		for _, t := range g.Tools {
			b.WriteString(`<tr><td><a href="/tools/` + html.EscapeString(t.Name) + `">` + html.EscapeString(t.Name) + `</a></td><td>` + cataloguePrice(t) + `</td></tr>`)
		}
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}
