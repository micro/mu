package home

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/wallet"
)

func PricingHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	// Hero
	b.WriteString(`<div style="max-width:560px;margin:0 auto;text-align:center;padding:24px 0 0">`)
	b.WriteString(`<h2 style="font-size:1.6rem;margin:0 0 8px">An agent for everyday</h2>`)
	b.WriteString(`<p style="color:#666;font-size:15px;margin:0 0 24px">News, mail, search, weather, markets, video — the everyday internet, handled by one agent you just talk to.</p>`)
	b.WriteString(`</div>`)

	// Plans
	b.WriteString(`<div style="display:flex;gap:16px;flex-wrap:wrap;justify-content:center;margin:0 0 24px">`)

	// Starter plan
	b.WriteString(`<div class="card" style="flex:1;min-width:240px;max-width:300px;text-align:center">`)
	b.WriteString(`<h3 style="margin:0 0 4px">Starter</h3>`)
	b.WriteString(`<p style="font-size:2rem;font-weight:700;margin:8px 0">£5<span style="font-size:14px;font-weight:400;color:#888">/month</span></p>`)
	b.WriteString(`<p style="color:#666;font-size:14px;margin:0 0 16px">500 credits</p>`)
	b.WriteString(`<ul style="text-align:left;list-style:none;padding:0;margin:0 0 16px;font-size:14px;line-height:2">`)
	b.WriteString(`<li>&#10003; AI agent with memory</li>`)
	b.WriteString(`<li>&#10003; News, markets, weather</li>`)
	b.WriteString(`<li>&#10003; Mail and messaging</li>`)
	b.WriteString(`<li>&#10003; Web search</li>`)
	b.WriteString(`<li>&#10003; Build apps with AI</li>`)
	b.WriteString(`</ul>`)
	b.WriteString(`<a href="/signup" class="btn" style="display:block">Get started</a>`)
	b.WriteString(`</div>`)

	// Pro plan
	b.WriteString(`<div class="card" style="flex:1;min-width:240px;max-width:300px;text-align:center;border:2px solid #000">`)
	b.WriteString(`<h3 style="margin:0 0 4px">Pro</h3>`)
	b.WriteString(`<p style="font-size:2rem;font-weight:700;margin:8px 0">£10<span style="font-size:14px;font-weight:400;color:#888">/month</span></p>`)
	b.WriteString(`<p style="color:#666;font-size:14px;margin:0 0 16px">1,200 credits</p>`)
	b.WriteString(`<ul style="text-align:left;list-style:none;padding:0;margin:0 0 16px;font-size:14px;line-height:2">`)
	b.WriteString(`<li>&#10003; Everything in Starter</li>`)
	b.WriteString(`<li>&#10003; More credits per month</li>`)
	b.WriteString(`<li>&#10003; Priority AI models</li>`)
	b.WriteString(`</ul>`)
	b.WriteString(`<a href="/signup" class="btn" style="display:block">Get started</a>`)
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)

	// What's included (free)
	b.WriteString(`<div class="card" style="max-width:560px;margin:0 auto 16px">`)
	b.WriteString(`<h3>Included for everyone</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666">Browse without an account. No ads, no tracking.</p>`)
	b.WriteString(`<ul style="list-style:none;padding:0;font-size:14px;line-height:2;margin:8px 0 0">`)
	b.WriteString(`<li>&#10003; News headlines and feeds</li>`)
	b.WriteString(`<li>&#10003; Market prices</li>`)
	b.WriteString(`<li>&#10003; Blog posts and social</li>`)
	b.WriteString(`<li>&#10003; Video</li>`)
	b.WriteString(`<li>&#10003; 3 free AI questions as a guest</li>`)
	b.WriteString(`</ul>`)
	b.WriteString(`</div>`)

	// Credit costs
	b.WriteString(`<div class="card" style="max-width:560px;margin:0 auto 16px">`)
	b.WriteString(`<h3>Credit costs</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 8px">1 credit = 1p. Pay for what you use.</p>`)
	b.WriteString(`<table class="stats-table" style="font-size:14px">`)
	b.WriteString(`<tr><td>AI agent</td><td>` + fmt.Sprintf("%d", wallet.CostAgentQuery) + `</td></tr>`)
	b.WriteString(`<tr><td>Chat</td><td>` + fmt.Sprintf("%d", wallet.CostChatQuery) + `</td></tr>`)
	b.WriteString(`<tr><td>Web search</td><td>` + fmt.Sprintf("%d", wallet.CostWebSearch) + `</td></tr>`)
	b.WriteString(`<tr><td>Weather</td><td>` + fmt.Sprintf("%d", wallet.CostWeatherForecast) + `</td></tr>`)
	b.WriteString(`<tr><td>Mail</td><td>` + fmt.Sprintf("%d", wallet.CostMailSend) + `</td></tr>`)
	b.WriteString(`<tr><td>News search</td><td>` + fmt.Sprintf("%d", wallet.CostNewsSearch) + `</td></tr>`)
	b.WriteString(`</table>`)
	b.WriteString(`</div>`)

	// Self-host
	b.WriteString(`<div class="card" style="max-width:560px;margin:0 auto 16px">`)
	b.WriteString(`<h3>Self-host</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666">Run your own instance with no limits. Single Go binary, bring your own AI provider.</p>`)
	b.WriteString(`<p style="font-size:14px"><a href="https://github.com/micro/mu">github.com/micro/mu</a></p>`)
	b.WriteString(`</div>`)

	// Login link (only when not logged in)
	if sess, _ := auth.TrySession(r); sess == nil {
		b.WriteString(`<p style="text-align:center;font-size:14px;color:#888;margin:16px 0">Already have an account? <a href="/login">Log in</a></p>`)
	}

	html := app.RenderHTMLForRequest("Pricing", "Personal AI — plans and pricing", b.String(), r)
	w.Write([]byte(html))
}
