package home

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
)

func serveGuestHome(w http.ResponseWriter, r *http.Request) {
	RefreshCards()

	var b strings.Builder

	b.WriteString(`<div style="text-align:center;padding:32px 0 0">`)
	b.WriteString(`<p style="color:#666;font-size:15px;margin:0 0 20px">Ask anything. Try it free.</p>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div style="max-width:560px;margin:0 auto 24px">`)
	b.WriteString(`<form action="/agent" method="GET" style="position:relative">`)
	b.WriteString(`<textarea name="prompt" placeholder="What do you need?" maxlength="512" rows="1" style="width:100%;padding:14px 44px 14px 16px;border:1px solid #ddd;border-radius:14px;font-size:16px;font-family:inherit;resize:none;box-sizing:border-box;line-height:1.4;overflow:hidden;background:#fff" onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();this.form.submit()}" oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,120)+'px'"></textarea>`)
	b.WriteString(`<button type="submit" style="position:absolute;right:8px;top:50%;transform:translateY(-50%);width:32px;height:32px;background:#000;color:#fff;border:none;border-radius:8px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:16px;padding:0">&#x2192;</button>`)
	b.WriteString(`</form>`)

	// Suggestion pills
	suggestions := []string{"Today's news", "Bitcoin price", "What is Mu?"}
	b.WriteString(`<div style="display:flex;gap:6px;flex-wrap:wrap;justify-content:center;margin-top:10px">`)
	for _, s := range suggestions {
		b.WriteString(fmt.Sprintf(`<a href="/agent?prompt=%s" style="padding:6px 12px;border:1px solid #e0e0e0;border-radius:20px;background:#fff;font-size:13px;color:#555;text-decoration:none;white-space:nowrap">%s</a>`, htmlEsc(url.QueryEscape(s)), htmlEsc(s)))
	}
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	// Public content cards in two-column layout
	var leftHTML, rightHTML []string
	cacheMutex.RLock()
	for _, card := range Cards {
		if card.ID != "news" && card.ID != "markets" && card.ID != "blog" && card.ID != "reminder" {
			continue
		}
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}
		if card.Link != "" {
			content += app.Link("More", card.Link)
		}
		cardHTML := fmt.Sprintf(app.CardTemplate, card.ID, card.ID, card.Title, content)
		if card.Column == "left" {
			leftHTML = append(leftHTML, cardHTML)
		} else {
			rightHTML = append(rightHTML, cardHTML)
		}
	}
	cacheMutex.RUnlock()

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(`<div id="home">`)
		b.WriteString(fmt.Sprintf(Template,
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n")))
		b.WriteString(`</div>`)
	}

	// CTA
	b.WriteString(`<div style="text-align:center;padding:24px 0;border-top:1px solid #eee;margin-top:16px">`)
	b.WriteString(`<p style="font-size:15px;color:#555;margin:0 0 12px">Get the full experience — AI agent with memory, mail, web search, and more.</p>`)
	b.WriteString(`<a href="/pricing" style="display:inline-block;padding:10px 24px;background:#000;color:#fff;text-decoration:none;border-radius:8px;font-size:15px;margin-right:8px">View pricing</a>`)
	b.WriteString(`<a href="/signup" style="display:inline-block;padding:10px 24px;border:1px solid #000;color:#000;text-decoration:none;border-radius:8px;font-size:15px">Sign up</a>`)
	b.WriteString(`</div>`)

	html := app.RenderHTMLWithLangAndBody("Your personal AI", "News, mail, markets, search and more through one AI", b.String(), "en", ` class="page-home"`)
	w.Write([]byte(html))
}
