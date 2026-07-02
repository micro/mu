package home

import (
	"net/http"

	"mu/internal/app"
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
		Title:       "Mu — An agent for everyday",
		Description: "An agent for everyday — news, mail, search, weather, markets and video, handled by one AI you just talk to.",
		Brand:       "Mu",
		Tagline:     "An agent for everyday",
		Subtag:      "News, mail, search, weather, markets, video — the everyday internet, handled by one agent you just talk to.",
		TopRight:    `<a href="/login">Log in</a>`,
		Body:        chatComponent(true),
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
