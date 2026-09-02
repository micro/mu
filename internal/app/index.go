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
.index-page{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:flex-start;padding:0 20px 40px;position:relative;width:100%}
/* The same header the app has: name centred, the way in on the right.
 *
 * This page and the app were two different headers — the app centres its brand
 * with the corner floated right, and this had the name on the left. So signing
 * in moved the wordmark across the screen, which is the discrepancy that shows
 * once everything else has stopped moving.
 *
 * Centred by the row, with the link taken out of the flow — the same shape as
 * #head/#head-right in mu.css, arrived at for the same reason: a name centred
 * against a variable-width link on the other side is not centred on the page,
 * it is centred on what is left over. */
.index-head{position:relative;width:100%;max-width:760px;margin:0 auto;
  display:flex;align-items:center;justify-content:center;padding:20px 0 0}
.brand{font-size:1.05rem;font-weight:800;letter-spacing:-.2px;line-height:1.25}
/* Air above the box, which the header no longer provides by being enormous. */
.index-body{padding-top:12vh}
.tagline{color:#111;font-size:18px;font-weight:700;margin-bottom:6px}
.subtag{color:#666;font-size:15px;margin-bottom:32px;max-width:520px;text-align:center;line-height:1.5}
/* The corner. It held one link and now holds two controls, so it is a row —
   and buttons and links take different defaults, which is 3px of misalignment
   side by side unless both are told the same. */
.login-link{position:absolute;right:0;top:50%;transform:translateY(-50%);
  display:flex;align-items:center;gap:14px}
.login-link a,.login-link button{color:#555;text-decoration:none;font-size:14px;font-weight:600;
  background:none;border:0;padding:0;font-family:inherit;line-height:20px;cursor:pointer}
.login-link a:hover,.login-link button:hover{color:#111}
/* An author rule beats the browser's own [hidden]{display:none} whatever its
   specificity, and the rule above sets a display on buttons via the flex row.
   Without this the install control is on the page in every browser that cannot
   install anything. */
.login-link [hidden]{display:none}
.also{text-align:center;margin:32px 0;font-size:14px;color:#888}
.footer{padding:20px;text-align:center;font-size:13px;color:#999}
/* No extra margin: FooterLinks already spaces the links with separators,
   so adding margin here made the same six links wrap where the app shell fits
   them on one line. */
.footer a{color:#555;text-decoration:none}
.footer a:hover{text-decoration:underline}
/* 14vh of air above the wordmark is right on a tall window and is a fifth of a
   short one — a 1280x600 laptop spent 84px on padding and then scrolled by 23.
   Height is the axis that decides here, so the query is on height. */
@media (max-height:720px){.index-body{padding-top:6vh}}
@media (max-width:600px){.index-body{padding-top:8vh}}
/* The hero cards on these pages are capped at ~240px so three sit in a row on
   desktop. Below that the cap left them stranded mid-screen, so let them fill
   the column like every card elsewhere in the app. */
/* Descendant selectors so these beat the per-page rules, which are emitted
   after this block and would otherwise win on source order alone. */
@media (max-width:600px){
  .lcards,.pcards{flex-direction:column;align-items:stretch;gap:10px}
  /* flex:1 1 220px sizes the main axis, which is the *height* once stacked —
     that left every card padded out to 220px tall. Size to content instead. */
  .lcards .lcard,.pcards .pcard{flex:0 0 auto;max-width:none;min-width:0;width:100%;box-sizing:border-box}
  .lctas{flex-direction:column}
  .lctas .lcta{width:100%;box-sizing:border-box;text-align:center}
}
</style>
</head>
<body>
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
