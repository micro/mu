package home

import (
	"fmt"
	"net/http"
	"net/url"

)

func LandingHandler(w http.ResponseWriter, r *http.Request) {
	page := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mu — Your personal AI</title>
<meta name="description" content="News, mail, markets, search and more through one AI">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/favicon.ico">
<link rel="apple-touch-icon" href="/icon-192.png">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Nunito Sans',sans-serif;background:#fff;color:#111;min-height:100vh;display:flex;flex-direction:column}
.landing{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:0 20px}
.brand{font-size:2.5rem;font-weight:800;letter-spacing:-1px;margin-bottom:8px}
.tagline{color:#666;font-size:16px;margin-bottom:32px}
.prompt-wrap{width:100%%;max-width:560px;margin-bottom:12px}
.prompt-wrap form{position:relative}
.prompt-wrap textarea{width:100%%;padding:14px 48px 14px 16px;border:1px solid #ddd;border-radius:24px;font-size:16px;font-family:inherit;resize:none;line-height:1.4;overflow:hidden;background:#fff;outline:none;transition:border-color 0.2s;box-sizing:border-box}
.prompt-wrap textarea:focus{border-color:#999}
.prompt-wrap button{position:absolute;right:8px;top:50%%;transform:translateY(-50%%);width:36px;height:36px;background:#111;color:#fff;border:none;border-radius:50%%;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:18px}
.pills{display:flex;gap:8px;flex-wrap:wrap;justify-content:center;margin-bottom:40px}
.pills a{padding:8px 16px;border:1px solid #e0e0e0;border-radius:20px;font-size:13px;color:#555;text-decoration:none;white-space:nowrap;transition:background 0.15s}
.pills a:hover{background:#f5f5f5}
.footer{padding:20px;text-align:center;font-size:13px;color:#999}
.footer a{color:#555;text-decoration:none;margin:0 10px}
.footer a:hover{text-decoration:underline}
</style>
</head>
<body>
<div class="landing">
  <div class="brand">Mu</div>
  <div class="tagline">Your personal AI</div>
  <div class="prompt-wrap">
    <form action="/agent" method="GET">
      <textarea name="prompt" placeholder="Ask anything..." maxlength="512" rows="1"
        onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();this.form.submit()}"
        oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,120)+'px'"></textarea>
      <button type="submit">&#x2192;</button>
    </form>
  </div>
  <div class="pills">` +
		landingPills() + `
  </div>
</div>
<div class="footer">
  <a href="/news">News</a>
  <a href="/markets">Markets</a>
  <a href="/blog">Blog</a>
  <a href="/pricing">Pricing</a>
  <a href="/login">Log in</a>
  <a href="/signup">Sign up</a>
</div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}

func landingPills() string {
	suggestions := []string{"Today's news", "Bitcoin price", "What is Mu?"}
	var out string
	for _, s := range suggestions {
		out += fmt.Sprintf(`<a href="/agent?prompt=%s">%s</a>`, htmlEsc(url.QueryEscape(s)), htmlEsc(s))
	}
	return out
}
