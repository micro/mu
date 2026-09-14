package app

// Landing is a minimal, sidebar-less page shell — the clean full-page layout
// used for the signed-out index and the developer portal. It is
// deliberately not the app shell (no nav rail): both are marketing/entry pages,
// not in-app views.
type Index struct {
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

// RenderIndex renders a full, self-contained page outside the app shell.
func RenderIndex(l Index) string {
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
<meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover">
<meta name="referrer" content="no-referrer">
<title>` + l.Title + `</title>
<meta name="description" content="` + l.Description + `">
<meta property="og:title" content="` + l.Title + `">
<meta property="og:description" content="` + l.Description + `">
` + ogImage + `
` + icons + `
<link rel="stylesheet" href="/mu.css?` + Version + `">

</head>
<body class="index-shell">
<div class="index-page">
  <div class="index-head">
    <div class="brand">` + l.Brand + `</div>
    ` + top + `
  </div>
  <div class="index-body">` + tag + sub + l.Body + below + `</div>
</div>
` + footer + l.Tail + `
</body>
</html>`
}
