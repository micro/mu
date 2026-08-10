package app

import (
	"embed"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/service"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// Version for cache busting static assets (generated at startup)
var Version = fmt.Sprintf("%d", time.Now().Unix())

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Package color mapping
var pkgColors = map[string]string{
	"news":  colorCyan,
	"chat":  colorGreen,
	"video": colorPurple,
	"blog":  colorYellow,
	"app":   colorBlue,
	"mail":  colorRed,
}

// cliMode is set at init time when the process is invoked without
// --serve, i.e. as a CLI rather than the server. It suppresses the
// package startup logs so they don't contaminate CLI stdout.
var cliMode bool

func init() {
	// Detect CLI mode without importing the cli package. The rule
	// mirrors isServerMode in main.go: any --serve means server.
	server := false
	for _, a := range os.Args[1:] {
		if a == "--serve" || a == "-serve" ||
			strings.HasPrefix(a, "--serve=") || strings.HasPrefix(a, "-serve=") {
			server = true
			break
		}
	}
	cliMode = !server
}

// Response holds data for responding in either JSON or HTML format
type Response struct {
	Data        interface{} // Data to serialize as JSON or pass to HTML renderer
	HTML        string      // Pre-rendered HTML body (used when Data is nil for HTML)
	Title       string      // Page title for HTML response
	Description string      // Meta description for HTML response
}

// WantsJSON returns true if the request prefers JSON response
func WantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// SendsJSON returns true if the request is sending JSON
func SendsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

// DecodeJSON decodes JSON from request body into the given struct
// Returns error if not JSON content type or decode fails
func DecodeJSON(r *http.Request, v interface{}) error {
	if !SendsJSON(r) {
		return fmt.Errorf("expected application/json content type")
	}
	return json.NewDecoder(r.Body).Decode(v)
}

// RespondJSON writes a JSON response
func RespondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// RespondError writes a JSON error response with the given status code
func RespondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// isFetch reports whether this request came from script rather than from the
// address bar or a form submit.
//
// Only a navigation can display a page, and a fetch() that asks for one gets
// several kilobytes of markup where it expected a sentence — which is exactly
// what happened the first time the console's compose box showed a refusal: it
// printed the <head> of an error page into the notice area. Browsers label the
// difference themselves in Sec-Fetch-Mode ("navigate" for a page load or form
// post, "cors"/"same-origin" for fetch and XHR), so nothing has to be guessed
// and no call site has to remember to set a header. Absent header — curl, an
// old browser — is treated as a navigation, which is the safe assumption for a
// human-readable response.
func isFetch(r *http.Request) bool {
	mode := r.Header.Get("Sec-Fetch-Mode")
	return mode != "" && mode != "navigate"
}

// errorTitle heads the page with what happened in plain words. The HTTP status
// text is written for a proxy log — "Forbidden" over a sentence explaining how
// to start posting reads as a scolding rather than an answer.
func errorTitle(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "Sign in"
	case http.StatusForbidden:
		return "Not yet"
	case http.StatusPaymentRequired:
		return "Needs credit"
	case http.StatusTooManyRequests:
		return "Slow down"
	case http.StatusNotFound:
		return "Not found"
	case http.StatusBadRequest:
		return "That did not work"
	}
	return http.StatusText(status)
}

// errorBackTo picks where "Back" goes: the page the request came from when it
// is one of ours, otherwise home. A refused POST has no useful URL of its own —
// the form lives at the referer.
func errorBackTo(r *http.Request) string {
	ref := r.Referer()
	if strings.HasPrefix(ref, "/") && !strings.HasPrefix(ref, "//") {
		return ref
	}
	if u, err := url.Parse(ref); err == nil && u.Path != "" && u.Host == r.Host {
		return u.Path
	}
	return "/"
}

// Unauthorized writes a 401 error response
func Unauthorized(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusUnauthorized, "Authentication required")
}

// TooManyRequests writes a 429. Use it where a limit is about how often
// something may be done rather than what it costs — a price refuses the caller
// without credits, a rate limit refuses the caller going too fast.
func TooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Too many requests"
	}
	Error(w, r, http.StatusTooManyRequests, message)
}

// NotFound writes a 404 error response
func NotFound(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Not found"
	}
	Error(w, r, http.StatusNotFound, message)
}

// RedirectToLogin redirects to login page with optional redirect back URL
func RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	redirect := r.URL.Path
	if r.URL.RawQuery != "" {
		redirect += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, "/login?redirect="+url.QueryEscape(redirect), http.StatusSeeOther)
}

// MethodNotAllowed writes a 405 error response
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

// Respond writes either JSON or HTML based on the Accept header
// If resp.Data is provided, it will be used for JSON responses
// If resp.HTML is provided, it will be wrapped in the page template for HTML responses
func Respond(w http.ResponseWriter, r *http.Request, resp Response) {
	if WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp.Data)
		return
	}

	// HTML response — RenderHTMLForRequest already prepends the verify
	// banner for unverified users on verification-gated instances.
	html := RenderHTMLForRequest(resp.Title, resp.Description, resp.HTML, r)
	w.Write([]byte(html))
}

//go:embed html/*
var htmlFiles embed.FS

// FooterLinks is the site footer, shared by the app shell and the sidebar-less
// landing shell (/about, /agents) so every page shows the same links.
func FooterLinks() string {
	return `<a href="/tools">Tools</a> · <a href="/pricing">Pricing</a> · <a href="/mcp">MCP</a> · <a href="/help">Help</a> · <a href="/privacy">Privacy</a> · <a href="/status">Status</a> · <a href="https://github.com/micro/mu">Source</a>` + torFooterLink()
}

func torFooterLink() string {
	if onion := os.Getenv("TOR_ONION"); onion != "" {
		return ` · <a href="http://` + onion + `" title="Tor Hidden Service">Tor</a>`
	}
	return ""
}

var Template = `
<html lang="%s">
  <head>
    <title>%s | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="description" content="%s">
    <meta name="referrer" content="no-referrer"/>
    <meta name="theme-color" content="#ffffff">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="default">
    <meta name="apple-mobile-web-app-title" content="Mu">
    <meta name="application-name" content="Mu">
    <link rel="apple-touch-icon" href="/icon-192.png">
    <link rel="preload" href="/home.png?` + Version + `" as="image">
    <link rel="preload" href="/mail.png?` + Version + `" as="image">
    <link rel="preload" href="/chat.png?` + Version + `" as="image">
    <link rel="preload" href="/post.png?` + Version + `" as="image">
    <link rel="preload" href="/news.png?` + Version + `" as="image">
    <link rel="preload" href="/video.png?` + Version + `" as="image">
    <link rel="preload" href="/account.png?` + Version + `" as="image">
    <link rel="preload" href="/weather.png?` + Version + `" as="image">
    <link rel="preload" href="/prayer.svg?` + Version + `" as="image">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="stylesheet" href="/mu.css?` + Version + `">
    <script src="/mu.js?` + Version + `"></script>
  </head>
  <body%s>
    <script>
      // Restore the collapsed sidebar before anything paints. Doing this in a
      // deferred script would show the sidebar for a frame and then yank it.
      try {
        if (localStorage.getItem('mu_nav_collapsed') === '1') {
          document.body.classList.add('nav-collapsed');
        }
      } catch (e) {}
    </script>
    <div id="head">
      <button id="menu-toggle" onclick="toggleMenu()" aria-label="Menu"><span></span><span></span><span></span></button>
      <div id="brand">
        <a href="/">Mu</a>
      </div>
      <!-- One flex cluster, so the items sit next to each other by measuring
           themselves. They used to be three absolutely positioned elements
           nudged apart by hand with sibling combinators, which could not look
           backwards and could not survive a fourth item — the balance is that
           fourth item. Hidden children take no space, so mail appearing and
           disappearing still costs nothing. -->
      <div id="head-right">
        <a id="head-mail" href="/mail" aria-label="Mail"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="16" rx="2"/><polyline points="22,7 12,13 2,7"/></svg><span id="head-mail-badge"></span></a>
        %s
      </div>
    </div>

    <div id="nav-overlay" onclick="toggleMenu()"></div>
    <div id="container">
      <div id="nav-container">
        <div id="nav">
          <!-- The nouns, in the order somebody meets them. Tools is what the
               product is named for and what carries the connect instructions,
               so it comes first; Agents is the showcase — what you can build on
               top of the tools once you have them — and Services is the back
               end, the answer to "what does this actually run".

               Agents used to be second and Tools third, which put the demo in
               front of the thing being demonstrated. Before any of this it was
               one alphabetical list of nineteen services that put Wallet
               eighteenth, between Video and Weather — alphabetical is not an
               ordering, it is the absence of one.

               Context was a fifth row: a page for what an agent remembers and
               what it is watching. It became a second home screen with a card
               picker on it and was removed. Memory, the half that was real, is
               a card on /account.

               Apps was here once, on the argument that it is half the product.
               It is a service with a Spec and a tile in the catalogue like the
               others, so a permanent entry above the fold was the spine
               claiming something the rest of the product does not agree with.
               Anyone who lives in Apps can pin it and it comes back, which is
               what pinning is for. -->
          <a href="/home"><img src="/home.png?` + Version + `"><span class="label">Home</span></a>
          <a href="/tools"><img src="/tools.svg?` + Version + `"><span class="label">Tools</span></a>
          <a href="/agents"><img src="/agent.svg?` + Version + `"><span class="label">Agents</span></a>
          <a href="/services"><img src="/services.svg?` + Version + `"><span class="label">Services</span></a>
          %s
        </div>
        <div class="nav-bottom">
          %s
        </div>
      </div>
      <div id="content">
        <h1 id="page-title">%s</h1>
        %s
      </div>
      <div id="footer">
        ` + FooterLinks() + `
      </div>
    </div>
  <script>
      if (navigator.serviceWorker) {
        navigator.serviceWorker.register (
          '/mu.js',
          {scope: '/'}
        );
      }
      
      // One button, two meanings. On a phone the sidebar is an overlay that
      // slides in, so this opens it. On desktop the sidebar is always there,
      // so this collapses it out of the way and the choice is remembered.
      function toggleMenu() {
        if (window.matchMedia('(min-width: 901px)').matches) {
          var collapsed = document.body.classList.toggle('nav-collapsed');
          try { localStorage.setItem('mu_nav_collapsed', collapsed ? '1' : '0'); } catch (e) {}
          return;
        }
        document.body.classList.toggle('menu-open');
      }
      document.addEventListener('click',function(){document.querySelectorAll('.ctrl-menu').forEach(function(m){m.style.display='none'})});
  </script>
  </body>
</html>
`

var CardTemplate = `
<!-- %s -->
<div id="%s" class="card">
  <h4>%s</h4>
  <div class="card-body">%s</div>
</div>
`

// LinkCodeFunc issues a one-time code for attaching a chat channel to an
// account. Set by main() to auth.GenerateLinkCode.
//
// One code, any channel: the code proves who the caller is, and which chat app
// they carry it to is their business. It replaced a per-channel "link <username>
// <password>", which asked people to type a password into a chat window.
var LinkCodeFunc func(accountID string) string

func Link(name, ref string) string {
	return fmt.Sprintf(`<a href="%s" class="link">%s →</a>`, ref, name)
}

func Head(appName string, refs []string) string {
	sort.Strings(refs)

	var head string

	// Add main link first
	head += fmt.Sprintf(`<a href="/%s" class="head">All</a>`, appName)

	// create head for topics - plain text format with hash
	for _, ref := range refs {
		if strings.EqualFold(ref, "all") {
			continue
		}
		head += fmt.Sprintf(`<a href="/%s#%s" class="head">%s</a>`, appName, ref, ref)
	}

	return head
}

func Card(id, title, content string) string {
	return fmt.Sprintf(CardTemplate, id, id, title, content)
}

// CardWithIcon renders a card with an icon image to the left of the title.
// If icon is empty, it falls back to Card without an icon.
func CardWithIcon(id, title, icon, content string) string {
	if icon == "" {
		return Card(id, title, content)
	}
	titleHTML := `<img src="` + htmlpkg.EscapeString(icon) + `" style="width:24px;height:24px;vertical-align:bottom;margin-right:6px;">` + htmlpkg.EscapeString(title)
	return fmt.Sprintf(CardTemplate, id, id, titleHTML, content)
}

// Render converts untrusted markdown to HTML. Use this for anything a user or
// a remote server authored: blog posts and comments, federated ActivityPub
// content, model output. Raw HTML in the source is dropped and link and image
// destinations are restricted to safe schemes, so a post containing
// <script>…</script> or [x](javascript:…) renders as inert text rather than
// executing on the reader's session.
//
// For repo-shipped markdown that deliberately embeds HTML, use RenderTrusted.
func Render(md []byte) []byte {
	return render(md, false)
}

// RenderTrusted converts markdown to HTML with raw HTML passed through. Only
// for content that ships in the binary (docs, whitepaper) — never for anything
// that arrived over the network.
func RenderTrusted(md []byte) []byte {
	return render(md, true)
}

func render(md []byte, trusted bool) []byte {
	// Strip LaTeX dollar sign escapes and protect plain currency before
	// parsing markdown so downstream MathJax scanners do not treat blog cards
	// or other rendered content as inline math.
	md = []byte(protectCurrencyDollars(StripLatexDollars(string(md))))

	// create markdown parser with extensions. MathJax is intentionally disabled:
	// Mu renders everyday prose more often than formulas, and paired currency
	// amounts such as "$1 billion ... $94,000" must remain readable text.
	extensions := (parser.CommonExtensions &^ parser.MathJax) | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	if !trusted {
		// SkipHTML drops raw HTML blocks and inline tags; Safelink limits link
		// destinations to http/https/mailto and friends. Safelink does not cover
		// image destinations, so those are filtered on the parsed tree below.
		htmlFlags |= html.SkipHTML | html.Safelink
		stripUnsafeImages(doc)
	}
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

// safeImageSchemes are the URL schemes an untrusted image may point at.
// Relative and protocol-relative URLs carry no scheme and are allowed.
var safeImageSchemes = map[string]bool{"http": true, "https": true}

// stripUnsafeImages blanks image destinations whose scheme is not known safe,
// so markdown like ![x](javascript:…) cannot emit an executable src. The
// renderer's Safelink flag only guards links, not images.
func stripUnsafeImages(doc ast.Node) {
	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		img, ok := node.(*ast.Image)
		if !ok {
			return ast.GoToNext
		}
		if !safeURL(string(img.Destination)) {
			img.Destination = nil
		}
		return ast.GoToNext
	})
}

// safeURL reports whether a destination is safe to emit as an image src.
// A URL with no scheme (relative, or protocol-relative) is safe; one with a
// scheme must be on the allowlist. Leading control characters and whitespace
// are stripped first, since browsers ignore them when parsing the scheme.
func safeURL(dest string) bool {
	cleaned := strings.Map(func(r rune) rune {
		if r <= 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, dest)

	colon := strings.IndexByte(cleaned, ':')
	if colon < 0 {
		return true // no scheme — relative
	}
	// A slash, question mark or hash before the colon means the colon is in a
	// path or query, not a scheme (e.g. "/a/b:c" or "?x=a:b").
	if i := strings.IndexAny(cleaned, "/?#"); i >= 0 && i < colon {
		return true
	}
	return safeImageSchemes[strings.ToLower(cleaned[:colon])]
}

// Regex patterns for LaTeX math delimiters around prices.
// LLMs (especially Claude) are heavily trained on LaTeX and frequently wrap
// dollar amounts in math delimiters: $100$, $$94.63$$, \(100\), etc.
var (
	// $$<price>$$ display math around prices: $$94.63$$ → $94.63
	displayPriceRe = regexp.MustCompile(`\$\$(\d[\d,]*\.?\d*(?:\s*(?:billion|trillion|million|thousand|k|m|bn|tn|%))?)\$\$`)
	// $$<text>$$ general display math: strip delimiters
	displayMathRe = regexp.MustCompile(`\$\$(.+?)\$\$`)
	// $<price>$ inline math around prices: $100.50$ → $100.50
	inlinePriceRe = regexp.MustCompile(`(\$\d[\d,]*\.?\d*(?:\s*(?:billion|trillion|million|thousand|k|m|bn|tn|%))?)\$`)
)

// StripLatexDollars removes LaTeX math delimiters that LLMs insert around
// dollar amounts. Handles backslash variants (\$, \(, \)), dollar-sign math
// delimiters ($...$, $$...$$), and HTML-escaped variants.
func StripLatexDollars(s string) string {
	// HTML-escaped backslash variants first (&#92; = \, &#x5c; = \)
	s = strings.ReplaceAll(s, `&#92;(`, "")
	s = strings.ReplaceAll(s, `&#92;)`, "")
	s = strings.ReplaceAll(s, `&#92;[`, "")
	s = strings.ReplaceAll(s, `&#92;]`, "")
	s = strings.ReplaceAll(s, `&#92;$`, "$")
	s = strings.ReplaceAll(s, `&#x5c;(`, "")
	s = strings.ReplaceAll(s, `&#x5c;)`, "")
	s = strings.ReplaceAll(s, `&#x5c;[`, "")
	s = strings.ReplaceAll(s, `&#x5c;]`, "")
	s = strings.ReplaceAll(s, `&#x5c;$`, "$")
	// Escaped dollar sign: \$ → $ (do this BEFORE stripping \( \) to avoid
	// consuming the backslash from \$ and leaving a bare $)
	s = strings.ReplaceAll(s, `\$`, "$")
	// \( or \) before a digit is a dollar sign: \(112 → $112, \)4,703 → $4,703
	s = regexp.MustCompile(`\\\((\d)`).ReplaceAllString(s, "$$$1")
	s = regexp.MustCompile(`\\\)(\d)`).ReplaceAllString(s, "$$$1")
	// \) after a digit is just a closing delimiter: 4,703\) → 4,703
	s = regexp.MustCompile(`(\d)\\\)`).ReplaceAllString(s, "$1")
	// Remaining \( \) \[ \] are math delimiters — strip them
	s = strings.ReplaceAll(s, `\(`, "")
	s = strings.ReplaceAll(s, `\)`, "")
	s = strings.ReplaceAll(s, `\[`, "")
	s = strings.ReplaceAll(s, `\]`, "")
	// \text{...} → content (LaTeX text command)
	s = regexp.MustCompile(`\\text\{([^}]*)\}`).ReplaceAllString(s, "$1")
	// \mathrm{...} → content
	s = regexp.MustCompile(`\\mathrm\{([^}]*)\}`).ReplaceAllString(s, "$1")
	// LaTeX display math around prices: $$94.63$$ → $94.63 (keep one $ as currency)
	s = displayPriceRe.ReplaceAllString(s, `$$$1`)
	// General display math: $$content$$ → content
	s = displayMathRe.ReplaceAllString(s, `$1`)
	// LaTeX inline math around prices: $100$ → $100 (strip trailing $)
	s = inlinePriceRe.ReplaceAllString(s, `$1`)
	// Clean up doubled dollar signs from any overlap
	for strings.Contains(s, "$$") {
		s = strings.ReplaceAll(s, "$$", "$")
	}
	return s
}

// SupportedLanguages maps language codes to their display names
var SupportedLanguages = map[string]string{
	"en": "English",
	"ar": "العربية",
	"zh": "中文",
}

// GetUserLanguage returns the language preference for the current user, defaults to "en"
func GetUserLanguage(r *http.Request) string {
	_, acc := auth.TrySession(r)
	if acc == nil || acc.Language == "" {
		return "en"
	}
	return acc.Language
}

// RenderHTML renders the given html in a template with default language (English)
func RenderHTML(title, desc, html string) string {
	return RenderHTMLWithLangAndAuth(title, desc, html, "en", nil)
}

// RenderHTMLForRequest renders the given html in a template using the
// user's language preference. Prepends the verify-to-post banner if the
// authenticated user has an unverified account on a verification-gated
// instance.
func RenderHTMLForRequest(title, desc, html string, r *http.Request) string {
	lang := GetUserLanguage(r)
	if banner := VerifyBanner(r); banner != "" {
		html = banner + html
	}
	if banner := CreditsBanner(r); banner != "" {
		html = banner + html
	}
	if banner := ConnectBanner(r); banner != "" {
		html = banner + html
	}
	_, acc := auth.TrySession(r)
	out := RenderHTMLWithLangAndAuth(title, desc, html, lang, acc)
	return out
}

// VerifyBanner says, before you write anything, that you cannot post yet and
// what to do about it. Empty for anyone who can.
//
// It asks auth.CanPost rather than re-deriving the rule, because it used to
// carry its own copy — verification only, and only on instances with mail
// configured — while the actual gate also blocked every account under 24 hours
// old, on every instance. The two disagreed in the worst direction: the block
// was wider than the warning, so a new user met it for the first time as a
// rejected POST after writing a post, creating an app, or filling in a form.
// Anything the gate refuses, this now announces.
func VerifyBanner(r *http.Request) string {
	_, acc := auth.TrySession(r)
	if acc == nil {
		return ""
	}
	reason := auth.PostBlockReason(acc.ID)
	if reason == "" {
		return ""
	}
	// Not on the pages that are the way out of it — the form is right there —
	// and not on money at all.
	//
	// This was a list of four exact paths, so /wallet/transfer was not on it,
	// and moving your own credit between accounts was met with "You cannot post
	// yet. Verify your email address before posting." Nothing on that page is a
	// post, so the banner read as a refusal of the transfer. A prefix rather
	// than a fifth path, because the next page under /wallet would have been
	// wrong in the same way.
	switch p := r.URL.Path; {
	case p == "/account" || p == "/verify":
		return ""
	case p == "/wallet" || strings.HasPrefix(p, "/wallet/"):
		return ""
	}
	action, href := "Verify →", "/account"
	if auth.VerificationRequired == nil || !auth.VerificationRequired() {
		// No mail on this instance, so verifying is not on offer: credit is.
		action, href = "Add credit →", "/wallet"
	}
	return `<div class="verify-banner" style="background:#fff8e1;border:1px solid #f1d68c;border-radius:6px;padding:10px 14px;margin:0 0 14px;font-size:14px;color:#5b4a00;display:flex;align-items:center;gap:10px;flex-wrap:wrap">
<strong>You cannot post yet.</strong>
<span>` + htmlpkg.EscapeString(reason) + `</span>
<a href="` + href + `" style="margin-left:auto;background:#000;color:#fff;text-decoration:none;padding:6px 14px;border-radius:6px">` + action + `</a>
</div>`
}

// navOperate closes the top group with what you have used and what you have
// left.
//
// These sit with the product rather than under it. They are not services — you
// do not use the wallet to do something, you use it to manage your use of
// everything else — but they are things you open and read, which is what the
// top of the sidebar is for. The bottom is the account itself: who you are, and
// getting in and out of it.
//
// Usage needs a session to mean anything and its page redirects without one, so
// it is drawn only for a signed-in viewer. Wallet is always drawn — a signed-out
// visitor still needs the way to it, and mu.js rewrites this anchor's href to
// the login redirect, which is why the id lives here.
func navOperate(acc *auth.Account) string {
	out := ""
	if acc != nil {
		out = `<a href="/usage"><img src="/usage.svg?` + Version + `"><span class="label">Usage</span></a>
          `
	}
	return out + `<a id="nav-wallet" href="/wallet"><img src="/wallet.png?` + Version + `"><span class="label">Wallet</span></a>`
}

// navPinned is the reader's own services, under a heading of their own.
//
// The sidebar went from nineteen alphabetical services — which put Wallet
// eighteenth, between Video and Weather — to none of them, because the three
// levels are what the product is and a list of nineteen buried them. That was
// right for arriving and wrong for using: somebody who wanted Video reached for
// the sidebar, found nothing, and had to go to the catalogue and hunt.
//
// The way back is not the old list. This one is chosen, so it is short, it is
// ordered by the person who made it, and it is empty until somebody pins
// something — which means the view a developer arrives at is unchanged. The
// group scrolls if it grows; the account group below it does not move, because
// signing out is not something to scroll for.
//
// Nothing is drawn at all when nothing is pinned. An empty heading over an
// empty list is a worse answer than no heading.
func navPinned(acc *auth.Account) string {
	if acc == nil {
		return ""
	}
	pinned := service.Pinned(acc.PinnedServices())
	if len(pinned) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="nav-group"><div class="nav-heading">Services</div>`)
	for _, s := range pinned {
		b.WriteString(`<a href="` + htmlpkg.EscapeString(s.Page) + `">` +
			`<img src="/` + htmlpkg.EscapeString(s.NavIcon()) + `?` + Version + `">` +
			`<span class="label">` + htmlpkg.EscapeString(s.NavLabel()) + `</span></a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// navBottom is the account: who you are, the page about you, running the place,
// and the way out.
//
// Kept as its own group rather than folded into the account page, because
// signing out is something you reach for directly and a logout that takes two
// clicks is a logout people hunt for. nav-username is a label mu.js corrects
// from the session: a page cached for one viewer and served to another would
// otherwise greet them by the wrong name.
//
// Admin belongs here and not in the top group. The top group is what the
// product is — Home, Tools, Agents, Services, Clients — and it should look the
// same to everybody, in a screenshot, in the docs, on somebody else's instance.
// Admin is a role, and a role sits with identity. It was reachable only as a
// line of text on /account, three clicks from anywhere, which is not where you
// put the page an operator opens most.
//
// It is rendered for an admin and hidden by mu.js for anyone else, so the link
// survives a page with no JavaScript and does not survive a page cached for an
// admin and served to a stranger. /admin checks the session itself either way;
// this is about not showing a door that is not yours.
func navBottom(acc *auth.Account) string {
	if acc == nil {
		return `<a id="nav-login" href="/login"><img src="/account.png?` + Version + `"><span class="label">Login</span></a>`
	}
	username := htmlpkg.EscapeString(acc.ID)
	admin := `<a id="nav-admin" href="/admin" style="display: none;"><img src="/admin.svg?` + Version + `"><span class="label">Admin</span></a>`
	if acc.Admin {
		admin = `<a id="nav-admin" href="/admin"><img src="/admin.svg?` + Version + `"><span class="label">Admin</span></a>`
	}
	return `<div id="nav-username">Signed in as @` + username + `</div>
          <a id="nav-account" href="/account"><img src="/account.png?` + Version + `"><span class="label">Account</span></a>
          ` + admin + `
          <a id="nav-logout" href="/logout"><img src="/logout.png?` + Version + `"><span class="label">Logout</span></a>
          <a id="nav-login" href="/login" style="display: none;"><img src="/account.png?` + Version + `"><span class="label">Login</span></a>`
}

// RenderHTMLWithLang renders the given html in a template with specified language
func RenderHTMLWithLang(title, desc, html, lang string) string {
	return RenderHTMLWithLangAndAuth(title, desc, html, lang, nil)
}

func RenderHTMLWithLangAndAuth(title, desc, html, lang string, acc *auth.Account) string {
	if lang == "" {
		lang = "en"
	}
	title, desc = escapeMeta(title), escapeMeta(desc)
	return (fmt.Sprintf(Template, lang, title, desc, "", headBalance(acc), navOperate(acc)+navPinned(acc), navBottom(acc), title, html))
}

// escapeMeta escapes a page title or description. Handlers pass these through
// from query strings and stored records (a search term, a post title, a
// contact's name), and they land in <title>, a meta content attribute and an
// <h1> — all text or attribute contexts where markup must not survive. The
// body argument is deliberately not escaped: handlers build that as HTML.
func escapeMeta(s string) string {
	return htmlpkg.EscapeString(s)
}

// RenderHTMLWithLangAndBody renders html with a custom body attribute string
// (e.g. ` class="page-home"` to enable page-specific CSS). Pass the viewer's
// account so the sidebar shows Account/Logout when signed in (nil for guests).
func RenderHTMLWithLangAndBody(title, desc, html, lang, bodyAttr string, acc *auth.Account) string {
	if lang == "" {
		lang = "en"
	}
	title, desc = escapeMeta(title), escapeMeta(desc)
	if banner := creditsBannerFor(acc, ""); banner != "" {
		html = banner + html
	}
	return (fmt.Sprintf(Template, lang, title, desc, bodyAttr, headBalance(acc), navOperate(acc)+navPinned(acc), navBottom(acc), title, html))
}

// RenderString renders a markdown string as html
func RenderString(v string) string {
	return string(Render([]byte(v)))
}

// RenderTemplate renders a markdown string in a html template
func RenderTemplate(title string, desc, text string) string {
	body := RenderString(text)
	title, desc = escapeMeta(title), escapeMeta(desc)
	return (fmt.Sprintf(Template, "en", title, desc, "", headBalance(nil), navOperate(nil), navBottom(nil), title, body))
}

func ServeHTML(html string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	})
}

// ServeStatic serves the static content in app/html
func Serve() http.Handler {
	var staticFS = fs.FS(htmlFiles)
	htmlContent, err := fs.Sub(staticFS, "html")
	if err != nil {
		log.Fatal(err)
	}

	fileServer := http.FileServer(http.FS(htmlContent))

	// Wrap with cache headers for static assets
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache headers for static assets
		if strings.HasSuffix(r.URL.Path, ".css") ||
			strings.HasSuffix(r.URL.Path, ".js") ||
			strings.HasSuffix(r.URL.Path, ".png") ||
			strings.HasSuffix(r.URL.Path, ".ico") ||
			strings.HasSuffix(r.URL.Path, ".webmanifest") {
			w.Header().Set("Cache-Control", "public, max-age=86400") // 1 day
		}
		fileServer.ServeHTTP(w, r)
	})
}

// ReturnTo is where to send someone after a control that writes the account is
// used from a page other than the one that owns the write.
//
// The card picker is on /context and pinning is on /apps, but both post here,
// because this is where the account is written. Sending them to /account
// afterwards would answer a click on /context by navigating away from the
// thing they were looking at. The form says where it was; the guard is
// safeRedirect's, since a `return` field is as forgeable as a query parameter.
func ReturnTo(r *http.Request, fallback string) string {
	to := r.Form.Get("return")
	if to == "" || to[0] != '/' || strings.HasPrefix(to, "//") {
		return fallback
	}
	return to
}

// Forbidden writes a 403 error response
func Forbidden(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Forbidden"
	}
	Error(w, r, http.StatusForbidden, message)
}

// Log prints a formatted log message with a colored package prefix
// and stores it in the in-memory system log ring buffer.
func Log(pkg string, format string, args ...interface{}) {
	color := pkgColors[pkg]
	if color == "" {
		color = colorWhite
	}
	timestamp := time.Now().Format("15:04:05")
	prefix := fmt.Sprintf("%s[%s %s]%s ", color, timestamp, pkg, colorReset)
	if !cliMode {
		fmt.Printf(prefix+format+"\n", args...)
	}
	appendSysLog(pkg, format, args...)
}

// Error writes an error response: JSON if the client expects it, otherwise a
// rendered page.
//
// The page matters. This is the single exit for every Forbidden, Unauthorized
// and BadRequest in the product, and it used to be http.Error — so a person
// who hit any of them was dropped onto a white screen with one line of text,
// no nav, and no way back except the browser's own button. The refusal is
// often the most important thing we ever say to a new user ("verify your
// email and you can post"), and it was the one thing said worst.
func Error(w http.ResponseWriter, r *http.Request, status int, message string) {
	if WantsJSON(r) || SendsJSON(r) || isFetch(r) {
		RespondError(w, status, message)
		return
	}
	if message == "" {
		message = http.StatusText(status)
	}
	body := `<div class="notice"><p>` + htmlpkg.EscapeString(message) + `</p></div>` +
		`<p><a class="link" href="` + htmlpkg.EscapeString(errorBackTo(r)) + `">Back</a></p>`
	// Rendered without the standing banners. They exist to interrupt an ordinary
	// page with something you should know; here the something-you-should-know is
	// the page, and a banner repeating the message word for word above it just
	// says the same thing twice.
	_, acc := auth.TrySession(r)
	page := RenderHTMLWithLangAndAuth(errorTitle(status), message, body, GetUserLanguage(r), acc)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(page))
}

// BadRequest writes a 400 error response
func BadRequest(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Bad request"
	}
	Error(w, r, http.StatusBadRequest, message)
}

// ServerError writes a 500 error response
func ServerError(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Internal server error"
	}
	Error(w, r, http.StatusInternalServerError, message)
}

// EmailSender is set by main.go and called to deliver verification
// emails. It's a callback to avoid an import cycle (mail imports app).
// If nil, email verification is unavailable on this instance.
var EmailSender func(to, subject, bodyPlain, bodyHTML string) error

// PublicURL returns the externally-reachable base URL for the instance.
// Falls back to relative paths when not configured.
func PublicURL() string {
	if v := os.Getenv("PUBLIC_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := os.Getenv("MAIL_DOMAIN"); v != "" {
		return "https://" + v
	}
	return ""
}

// validEmail performs minimal sanity checking — the real check is whether
// the verification email actually arrives and is clicked.
func ValidEmail(s string) bool {
	if len(s) < 5 || len(s) > 254 {
		return false
	}
	at := strings.Index(s, "@")
	if at < 1 || at == len(s)-1 {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	if !strings.Contains(s[at+1:], ".") {
		return false
	}
	return true
}
