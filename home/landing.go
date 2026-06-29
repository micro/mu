package home

import (
	"net/http"
)

func LandingHandler(w http.ResponseWriter, r *http.Request) {
	// Prefill + auto-submit from a deep link, e.g. /?q=... or /?prompt=...
	prefill := r.URL.Query().Get("q")
	if prefill == "" {
		prefill = r.URL.Query().Get("prompt")
	}
	var prefillScript string
	if prefill != "" {
		prefillScript = `<script>(function(){
var input=document.getElementById('mu-chat-input');
if(input&&window.muChatAsk){input.value=` + jsString(prefill) + `;window.muChatAsk(input.value);}
history.replaceState(null,'','/');
})()</script>`
	}

	page := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mu — An agent for everyday</title>
<meta name="description" content="An agent for everyday — news, mail, search, weather, markets and video, handled by one AI you just talk to.">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/favicon.ico">
<link rel="apple-touch-icon" href="/icon-192.png">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Nunito Sans',sans-serif;background:#fff;color:#111;min-height:100vh;display:flex;flex-direction:column}
.landing{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:flex-start;padding:14vh 20px 40px;position:relative}
.brand{font-size:2.5rem;font-weight:800;letter-spacing:-1px;margin-bottom:8px}
.tagline{color:#111;font-size:18px;font-weight:700;margin-bottom:6px}
.subtag{color:#666;font-size:15px;margin-bottom:32px;max-width:460px;text-align:center;line-height:1.5}
.login-link{position:absolute;top:20px;right:20px}
.login-link a{color:#555;text-decoration:none;font-size:14px;font-weight:600}
.also{text-align:center;margin:32px 0;font-size:14px;color:#888}
.footer{padding:20px;text-align:center;font-size:13px;color:#999}
.footer a{color:#555;text-decoration:none;margin:0 10px}
.footer a:hover{text-decoration:underline}
</style>
</head>
<body>
<div class="landing">
  <div class="login-link"><a href="/login">Log in</a></div>
  <div class="brand">Mu</div>
  <div class="tagline">An agent for everyday</div>
  <div class="subtag">News, mail, search, weather, markets, video — the everyday internet, handled by one agent you just talk to.</div>
  ` + chatComponent(true) + `
  <div class="also">
    <span>Also available on</span>
    <a href="https://discord.gg/WeMU5AGxD" style="color:#5865F2;text-decoration:none;margin-left:8px;font-weight:600">Discord</a>
    <span style="margin:0 4px">·</span>
    <a href="https://t.me/MicroMuBot" style="color:#229ED9;text-decoration:none;font-weight:600">Telegram</a>
  </div>
</div>
<div class="footer">
  <a href="/news">News</a>
  <a href="/markets">Markets</a>
  <a href="/blog">Blog</a>
  <a href="/pricing">Pricing</a>
  <a href="/docs">Docs</a>
  <a href="/login">Log in</a>
</div>
` + prefillScript + `
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}
