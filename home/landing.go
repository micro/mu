package home

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/markets"
	"mu/news"
)

func LandingHandler(w http.ResponseWriter, r *http.Request) {
	// Prefill + auto-submit from a deep link, e.g. /?q=... or /?prompt=...
	prefill := r.URL.Query().Get("q")
	if prefill == "" {
		prefill = r.URL.Query().Get("prompt")
	}
	var tail string
	if prefill != "" {
		tail = `<script>(function(){
var input=document.getElementById('mu-chat-input');
if(input&&window.muChatAsk){input.value=` + jsString(prefill) + `;window.muChatAsk(input.value);}
history.replaceState(null,'','/');
})()</script>`
	}

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — your services, one screen",
		Description: "News, markets, mail, weather, video and more — at a glance, with an agent that acts on them. Open and self-hostable.",
		Brand:       "Mu",
		Tagline:     "Your services, one screen.",
		Subtag:      "News, markets, weather, video — at a glance, with an agent that acts on them. Ask it anything below; sign in to make it yours.",
		TopRight:    `<a href="/login">Log in</a>`,
		Body:        chatComponent(true) + publicGlanceHTML(),
		Below: `<span>Also available on</span>
    <a href="https://discord.gg/WeMU5AGxD" style="color:#5865F2;text-decoration:none;margin-left:8px;font-weight:600">Discord</a>
    <span style="margin:0 4px">·</span>
    <a href="https://t.me/MicroMuBot" style="color:#229ED9;text-decoration:none;font-weight:600">Telegram</a>`,
		Footer: `<a href="/news">News</a>
  <a href="/markets">Markets</a>
  <a href="/blog">Blog</a>
  <a href="/pricing">Pricing</a>
  <a href="/docs">Docs</a>
  <a href="/login">Log in</a>`,
		Tail: tail,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}

// publicGlanceHTML renders a live, self-contained glance for the logged-out
// landing — real headlines and market prices, so a visitor sees the product
// (glance first) before signing in. Inline-styled because the landing shell
// does not load mu.css. Returns "" if there is nothing to show yet.
func publicGlanceHTML() string {
	heads := topHeadlines(6)
	prices := topPrices(6)
	if len(heads) == 0 && len(prices) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<style>
.glance{width:100%;max-width:720px;margin:36px auto 0;display:grid;grid-template-columns:1fr 1fr;gap:28px;text-align:left}
.glance h3{font-size:12px;font-weight:700;letter-spacing:.04em;text-transform:uppercase;color:#999;margin:0 0 12px}
.glance a.head{display:block;color:#111;text-decoration:none;font-size:14px;line-height:1.4;padding:7px 0;border-bottom:1px solid #f0f0f0}
.glance a.head:hover{color:#000;text-decoration:underline}
.glance .price{display:flex;justify-content:space-between;align-items:baseline;font-size:14px;padding:7px 0;border-bottom:1px solid #f0f0f0}
.glance .price .sym{font-weight:600;color:#333}
.glance .price .val{color:#555}
.glance .price .chg{font-weight:600;margin-left:8px}
.glance .up{color:#137333}.glance .down{color:#c5221f}
.glance-cta{max-width:720px;margin:24px auto 0;text-align:center;font-size:14px;color:#888}
.glance-cta a{color:#111;font-weight:600;text-decoration:none}
.glance-cta a:hover{text-decoration:underline}
@media(max-width:640px){.glance{grid-template-columns:1fr;gap:24px}}
</style>`)

	b.WriteString(`<div class="glance">`)
	if len(heads) > 0 {
		b.WriteString(`<div><h3>Today</h3>`)
		for _, h := range heads {
			b.WriteString(fmt.Sprintf(`<a class="head" href="%s"%s>%s</a>`,
				htmlEsc(h.URL), externalLinkAttrs(h.URL), htmlEsc(h.Title)))
		}
		b.WriteString(`</div>`)
	}
	if len(prices) > 0 {
		b.WriteString(`<div><h3>Markets</h3>`)
		for _, p := range prices {
			cls := "up"
			sign := "+"
			if p.Change < 0 {
				cls, sign = "down", ""
			}
			b.WriteString(fmt.Sprintf(`<div class="price"><span class="sym">%s</span><span class="val">%s<span class="chg %s">%s%.2f%%</span></span></div>`,
				htmlEsc(p.Symbol), formatPrice(p.Price), cls, sign, p.Change))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="glance-cta">Sign in to make it yours — your mail, reminders and pinned apps. <a href="/login">Log in</a></div>`)
	return b.String()
}

// externalLinkAttrs adds target/rel for off-site links (headlines), nothing for
// relative ones.
func externalLinkAttrs(url string) string {
	if strings.HasPrefix(url, "http") {
		return ` target="_blank" rel="noopener noreferrer"`
	}
	return ""
}

type headline struct{ Title, URL string }

// topHeadlines returns up to n recent headlines with a title and link.
func topHeadlines(n int) []headline {
	var out []headline
	seen := map[string]bool{}
	for _, p := range news.GetFeed() {
		t := strings.TrimSpace(p.Title)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, headline{Title: t, URL: p.URL})
		if len(out) >= n {
			break
		}
	}
	return out
}

type priceRow struct {
	Symbol string
	Price  float64
	Change float64
}

// topPrices returns a few well-known symbols (if available) so the markets
// glance is stable rather than dominated by whatever moved most.
func topPrices(n int) []priceRow {
	data := markets.GetAllPriceData()
	if len(data) == 0 {
		return nil
	}
	preferred := []string{"BTC", "ETH", "SOL", "XRP", "GOLD", "OIL"}
	var out []priceRow
	used := map[string]bool{}
	for _, sym := range preferred {
		if pd, ok := data[sym]; ok {
			out = append(out, priceRow{sym, pd.Price, pd.Change24h})
			used[sym] = true
			if len(out) >= n {
				return out
			}
		}
	}
	// Fill any remaining slots deterministically (alphabetical) so the list is stable.
	var rest []string
	for sym := range data {
		if !used[sym] {
			rest = append(rest, sym)
		}
	}
	sort.Strings(rest)
	for _, sym := range rest {
		pd := data[sym]
		out = append(out, priceRow{sym, pd.Price, pd.Change24h})
		if len(out) >= n {
			break
		}
	}
	return out
}

// formatPrice renders a USD price with sensible precision for its magnitude.
func formatPrice(p float64) string {
	switch {
	case p >= 1000:
		return "$" + addThousands(fmt.Sprintf("%.0f", p))
	case p >= 1:
		return fmt.Sprintf("$%.2f", p)
	default:
		return fmt.Sprintf("$%.4f", p)
	}
}

// addThousands inserts commas into an integer string.
func addThousands(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if n > pre {
			b.WriteString(",")
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteString(",")
		}
	}
	return b.String()
}
