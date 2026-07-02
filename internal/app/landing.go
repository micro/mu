package app

// Landing is a minimal, sidebar-less page shell — the clean full-page layout
// used for the logged-out consumer landing and the developer portal. It is
// deliberately not the app shell (no nav rail): both are marketing/entry pages,
// not in-app views.
type Landing struct {
	Title       string // <title> / meta
	Description string
	Brand       string // big wordmark (e.g. "Mu", or a portal's host-derived name)
	Tagline     string
	Subtag      string
	TopRight    string // optional top-right HTML (e.g. a Log in link)
	Body        string // hero content (the chat component, or portal cards)
	Below       string // optional block under the hero (e.g. "also on Discord")
	Footer      string // footer links HTML
	Tail        string // optional scripts appended before </body>
	Image       string // og:image + favicon URL; empty keeps the Mu defaults
}

// RenderLanding renders a full, self-contained landing page.
func RenderLanding(l Landing) string {
	top := ""
	if l.TopRight != "" {
		top = `<div class="login-link">` + l.TopRight + `</div>`
	}
	below := ""
	if l.Below != "" {
		below = `<div class="also">` + l.Below + `</div>`
	}
	footer := ""
	if l.Footer != "" {
		footer = `<div class="footer">` + l.Footer + `</div>`
	}
	sub := ""
	if l.Subtag != "" {
		sub = `<div class="subtag">` + l.Subtag + `</div>`
	}
	tag := ""
	if l.Tagline != "" {
		tag = `<div class="tagline">` + l.Tagline + `</div>`
	}

	// Icons + social preview. A custom Image (the portal's host-derived wordmark)
	// replaces the Mu-branded defaults so a shared link doesn't show the Mu logo.
	icons := `<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/favicon.ico">
<link rel="apple-touch-icon" href="/icon-192.png">`
	ogImage := ""
	if l.Image != "" {
		icons = `<link rel="icon" href="` + l.Image + `">
<link rel="apple-touch-icon" href="` + l.Image + `">`
		ogImage = `<meta property="og:image" content="` + l.Image + `">
<meta name="twitter:card" content="summary_large_image">`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + l.Title + `</title>
<meta name="description" content="` + l.Description + `">
<meta property="og:title" content="` + l.Title + `">
<meta property="og:description" content="` + l.Description + `">
` + ogImage + `
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
` + icons + `
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Nunito Sans',sans-serif;background:#fff;color:#111;min-height:100vh;display:flex;flex-direction:column}
.landing{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:flex-start;padding:14vh 20px 40px;position:relative;width:100%}
.brand{font-size:2.5rem;font-weight:800;letter-spacing:-1px;margin-bottom:8px}
.tagline{color:#111;font-size:18px;font-weight:700;margin-bottom:6px}
.subtag{color:#666;font-size:15px;margin-bottom:32px;max-width:520px;text-align:center;line-height:1.5}
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
  ` + top + `
  <div class="brand">` + l.Brand + `</div>
  ` + tag + sub + l.Body + below + `
</div>
` + footer + l.Tail + `
</body>
</html>`
}
