package app

import (
	"bytes"
	"compress/gzip"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/auth"
	"mu/internal/service"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

var Version = fmt.Sprintf("%d", time.Now().Unix())

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

var pkgColors = map[string]string{
	"news":  colorCyan,
	"chat":  colorGreen,
	"video": colorPurple,
	"blog":  colorYellow,
	"app":   colorBlue,
	"mail":  colorRed,
}

var cliMode bool

func init() {
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

type Response struct {
	Data        interface{} // Data to serialize as JSON or pass to HTML renderer
	HTML        string      // Pre-rendered HTML body (used when Data is nil for HTML)
	Title       string      // Page title for HTML response
	Description string      // Meta description for HTML response
	BodyClass   string
}

func WantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func SendsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

func DecodeJSON(r *http.Request, v interface{}) error {
	if !SendsJSON(r) {
		return fmt.Errorf("expected application/json content type")
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func RespondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func RespondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func isFetch(r *http.Request) bool {
	mode := r.Header.Get("Sec-Fetch-Mode")
	return mode != "" && mode != "navigate"
}

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

func Unauthorized(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusUnauthorized, "Authentication required")
}

func TooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Too many requests"
	}
	Error(w, r, http.StatusTooManyRequests, message)
}

func NotFound(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Not found"
	}
	Error(w, r, http.StatusNotFound, message)
}

func RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	redirect := r.URL.Path
	if r.URL.RawQuery != "" {
		redirect += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, "/login?redirect="+url.QueryEscape(redirect), http.StatusSeeOther)
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func Respond(w http.ResponseWriter, r *http.Request, resp Response) {
	if WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp.Data)
		return
	}

	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-cache, private")
	}

	w.Write([]byte(renderForRequest(resp.Title, resp.Description, resp.HTML, resp.BodyClass, r))) //nolint:errcheck
}

//go:embed html/*
var htmlFiles embed.FS

func footerFor(acc *auth.Account) string {
	if acc != nil {
		return ""
	}
	return `<div id="footer">` + FooterLinks() + `</div>`
}

func FooterLinks() string {
	return `<a href="/about">About</a> · <a href="/contact">Contact</a> · ` +
		`<a href="/pricing">Pricing</a> · ` +
		`<a href="/privacy">Privacy</a> · <a href="/status">Status</a>` + torFooterLink()
}

func torFooterLink() string {
	if onion := os.Getenv("TOR_ONION"); onion != "" {
		return ` · <a href="http://` + onion + `" title="Tor Hidden Service">Tor</a>`
	}
	return ""
}

var Template = `<!doctype html>
<html lang="%s"><head><meta charset="utf-8"><title>%s</title>
<meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover">
<meta name="apple-mobile-web-app-title" content="Micro"><meta name="application-name" content="Micro">
<meta name="description" content="%s"><meta name="referrer" content="no-referrer"><meta name="theme-color" content="#ffffff">
<link rel="apple-touch-icon" href="/icon-192.png"><link rel="manifest" href="/manifest.webmanifest">
<link rel="stylesheet" href="/mu.css?` + Version + `">
<script src="/mu.js?` + Version + `"></script><script defer src="/shell.js?` + Version + `"></script><script defer src="/viewport.js?` + Version + `"></script>
</head><body%s>
<script>try{if(localStorage.getItem('mu_nav_collapsed')==='1')document.body.classList.add('nav-collapsed')}catch(e){}</script>
<header id="head"><button id="menu-toggle" onclick="toggleMenu()" aria-label="Menu"><span></span><span></span><span></span></button><div id="brand"><a href="/">Micro</a></div><div id="head-right">%s</div></header>
<div id="nav-overlay" onclick="toggleMenu()"></div><div id="container"><aside id="nav-container"><nav id="nav">%s%s</nav><div class="nav-bottom">%s</div></aside><main id="content">%s%s</main></div>%s
</body></html>`

var CardTemplate = `
<!-- %s -->
<div id="%s" class="card">
  <h4>%s</h4>
  <div class="card-body">%s</div>
</div>
`

func Link(name, ref string) string {
	return fmt.Sprintf(`<a href="%s" class="link">%s</a>`, ref, name)
}

func TextLink(name, ref string) string {
	return fmt.Sprintf(`<a href="%s" class="link-text">%s</a>`, ref, name)
}

func Head(appName string, refs []string) string {
	sort.Strings(refs)

	var head string

	head += fmt.Sprintf(`<a href="/%s" class="head">All</a>`, appName)

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

func CardWithIcon(id, title, icon, content string) string {
	if icon == "" {
		return Card(id, title, content)
	}
	titleHTML := `<img src="` + htmlpkg.EscapeString(icon) + `" class="icon-24">` + htmlpkg.EscapeString(title)
	return fmt.Sprintf(CardTemplate, id, id, titleHTML, content)
}

func Render(md []byte) []byte {
	return render(md, false, false, 0)
}

func RenderTrusted(md []byte) []byte {
	return render(md, true, false, 0)
}

func RenderNoImages(md []byte) []byte {
	return render(md, false, true, 0)
}

func RenderLines(md []byte) []byte {
	return render(md, false, false, parser.HardLineBreak)
}

func render(md []byte, trusted, noImages bool, extra parser.Extensions) []byte {
	md = []byte(protectCurrencyDollars(StripLatexDollars(string(md))))

	extensions := (parser.CommonExtensions &^ parser.MathJax) | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock | extra
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	if noImages {
		htmlFlags |= html.SkipImages
	}
	if !trusted {
		htmlFlags |= html.SkipHTML | html.Safelink
		stripUnsafeImages(doc)
	}
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

var safeImageSchemes = map[string]bool{"http": true, "https": true}

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
	if i := strings.IndexAny(cleaned, "/?#"); i >= 0 && i < colon {
		return true
	}
	return safeImageSchemes[strings.ToLower(cleaned[:colon])]
}

var (
	displayPriceRe = regexp.MustCompile(`\$\$(\d[\d,]*\.?\d*(?:\s*(?:billion|trillion|million|thousand|k|m|bn|tn|%))?)\$\$`)
	displayMathRe  = regexp.MustCompile(`\$\$(.+?)\$\$`)
	inlinePriceRe  = regexp.MustCompile(`(\$\d[\d,]*\.?\d*(?:\s*(?:billion|trillion|million|thousand|k|m|bn|tn|%))?)\$`)
)

func StripLatexDollars(s string) string {
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
	s = strings.ReplaceAll(s, `\$`, "$")
	s = regexp.MustCompile(`\\\((\d)`).ReplaceAllString(s, "$$$1")
	s = regexp.MustCompile(`\\\)(\d)`).ReplaceAllString(s, "$$$1")
	s = regexp.MustCompile(`(\d)\\\)`).ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, `\(`, "")
	s = strings.ReplaceAll(s, `\)`, "")
	s = strings.ReplaceAll(s, `\[`, "")
	s = strings.ReplaceAll(s, `\]`, "")
	s = regexp.MustCompile(`\\text\{([^}]*)\}`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\\mathrm\{([^}]*)\}`).ReplaceAllString(s, "$1")
	s = displayPriceRe.ReplaceAllString(s, `$$$1`)
	s = displayMathRe.ReplaceAllString(s, `$1`)
	s = inlinePriceRe.ReplaceAllString(s, `$1`)
	for strings.Contains(s, "$$") {
		s = strings.ReplaceAll(s, "$$", "$")
	}
	return s
}

var SupportedLanguages = map[string]string{
	"en": "English",
	"ar": "العربية",
	"zh": "中文",
}

func UserLanguage(r *http.Request) string {
	_, acc := auth.TrySession(r)
	if acc == nil || acc.Language == "" {
		return "en"
	}
	return acc.Language
}

func RenderHTML(title, desc, html string, acc *auth.Account) string {
	return renderWithLang(title, desc, html, "en", acc)
}

func renderForRequest(title, desc, html, bodyClass string, r *http.Request) string {
	lang := UserLanguage(r)
	if banner := VerifyBanner(r); banner != "" {
		html = banner + html
	}
	if banner := CreditsBanner(r); banner != "" {
		html = banner + html
	}
	_, acc := auth.TrySession(r)
	here := r.URL.Path
	if r.URL.RawQuery != "" {
		here += "?" + r.URL.RawQuery
	}
	return renderShell(lang, title, desc, bodyClass, html, acc, navPath(r.URL.Path), here)
}

func VerifyBanner(r *http.Request) string {
	_, acc := auth.TrySession(r)
	if acc == nil {
		return ""
	}
	reason := auth.PostBlockReason(acc.ID)
	if reason == "" {
		return ""
	}
	switch p := r.URL.Path; {
	case p == "/verify":
		return ""
	case p == "/account" || strings.HasPrefix(p, "/account/"):
		return ""
	case p == "/wallet" || strings.HasPrefix(p, "/wallet/"):
		return ""
	}
	action, href := "Verify", "/account"
	if auth.VerificationRequired == nil || !auth.VerificationRequired() {
		action, href = "Top up", "/account/topup"
	}
	said := htmlpkg.EscapeString(reason)
	for _, l := range []struct{ phrase, href string }{
		{"your Account", "/account"},
		{"your Balance", "/account/billing#balance"},
	} {
		said = strings.ReplaceAll(said, l.phrase,
			`your <a href="`+l.href+`" >`+strings.TrimPrefix(l.phrase, "your ")+`</a>`)
	}
	return `<div class="verify-banner app-banner">
<strong>You cannot post yet.</strong>
<span>` + said + `</span>
<a href="` + href + `" class="btn push-right">` + action + `</a>
</div>`
}

func navMain(acc *auth.Account) string {
	if acc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(navigationLink("nav-home", "/", "Home", "/home.png"))
	b.WriteString(`<div class="nav-secondary">`)
	for _, item := range []struct{ id, href, label, icon string }{
		{"nav-inbox", "/inbox", "Inbox", "/email.svg"},
		{"nav-agents", "/agents", "Agents", "/agent.svg"},
		{"nav-services", "/services", "Services", "/services.svg"},
	} {
		b.WriteString(navigationLink(item.id, item.href, item.label, item.icon))
	}
	b.WriteString(`</div>`)

	return b.String()
}

func navigationLink(id, href, label, icon string) string {
	attr := ""
	if id != "" {
		attr = ` id="` + htmlpkg.EscapeString(id) + `"`
	}
	return `<a` + attr + ` href="` + htmlpkg.EscapeString(href) + `"><img src="` + icon + `" alt="" aria-hidden="true"><span>` + htmlpkg.EscapeString(label) + `</span></a>`
}

var TopUpConfigured func() bool

func navAdmin(acc *auth.Account) string {
	if acc == nil || !acc.Admin {
		return ""
	}
	return `<a id="nav-admin" href="/admin"><img src="/admin.svg?` + Version + `"><span class="label">Admin</span></a>`
}

func navPinned(acc *auth.Account) string {
	if acc == nil {
		return ""
	}
	pinned := service.Pinned(acc.Pinned)
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

func headCorner(acc *auth.Account, here string) string {
	if acc == nil {
		return ""
	}
	return headBalance(acc)
}

func loginBack(here string) string {
	if here == "" || here == "/" {
		return ""
	}
	return "?redirect=" + url.QueryEscape(here)
}

func navBottom(acc *auth.Account, here string) string {
	if acc == nil {
		return `<a id="nav-login" href="/login` + loginBack(here) + `"><img src="/account.png?` + Version + `"><span class="label">Login</span></a>`
	}
	username := htmlpkg.EscapeString(acc.ID)

	return `<details class="nav-account-disclosure"><summary class="nav-me-who" aria-label="Account menu"><img src="/account.png" alt="" aria-hidden="true"><span id="nav-username">@` + username + `</span></summary><div class="nav-account-menu">
          <a id="nav-account" href="/account"><img src="/account.png?` + Version + `"><span class="label">Account</span></a>
          <a id="nav-profile" href="/account/profile"><img src="/account.png?` + Version + `"><span class="label">Profile</span></a>
          <a id="nav-account-billing" href="/account/billing"><img src="/wallet.png?` + Version + `"><span class="label">Billing</span></a>
` + navAdmin(acc) + `
          <a id="nav-logout" href="/logout"><img src="/logout.png?` + Version + `"><span class="label">Logout</span></a></div></details>
          <a id="nav-login" href="/login" class="d-none"><img src="/account.png?` + Version + `"><span class="label">Login</span></a>`
}

func renderWithLang(title, desc, html, lang string, acc *auth.Account) string {
	if lang == "" {
		lang = "en"
	}
	title, desc = escapeMeta(title), escapeMeta(desc)
	return renderShell(lang, title, desc, "", html, acc, "", "")
}

func escapeMeta(s string) string {
	return htmlpkg.EscapeString(s)
}

func RenderString(v string) string {
	return string(Render([]byte(v)))
}

func RenderTemplate(title string, desc, text string) string {
	body := RenderString(text)
	title, desc = escapeMeta(title), escapeMeta(desc)
	return renderShell("en", title, desc, "", body, nil, "", "")
}

func ServeHTML(html string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	})
}

func Serve() http.Handler {
	var staticFS = fs.FS(htmlFiles)
	htmlContent, err := fs.Sub(staticFS, "html")
	if err != nil {
		log.Fatal(err)
	}

	fileServer := http.FileServer(http.FS(htmlContent))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mu.css" {
			serveStyles(w, r)
			return
		}
		switch {
		case r.URL.Path == "/mu.js" || strings.HasSuffix(r.URL.Path, "/mu.js"):
			w.Header().Set("Cache-Control", "no-cache")
			if r.URL.RawQuery == Version && r.Header.Get("Service-Worker") != "script" && r.Header.Get("Sec-Fetch-Dest") != "serviceworker" {
				w.Header().Set("Cache-Control", "public, max-age=86400")
			}
		case strings.HasSuffix(r.URL.Path, ".css"),
			strings.HasSuffix(r.URL.Path, ".js"),
			strings.HasSuffix(r.URL.Path, ".png"),
			strings.HasSuffix(r.URL.Path, ".ico"),
			strings.HasSuffix(r.URL.Path, ".webmanifest"):
			w.Header().Set("Cache-Control", "public, max-age=86400") // 1 day
		}
		if compressed(w, r, htmlContent) {
			return
		}
		if strings.HasSuffix(r.URL.Path, ".webmanifest") {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func compressed(w http.ResponseWriter, r *http.Request, files fs.FS) bool {
	name := strings.TrimPrefix(r.URL.Path, "/")
	switch {
	case name == "", !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"):
		return false
	case !strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".js") &&
		!strings.HasSuffix(name, ".svg") && !strings.HasSuffix(name, ".webmanifest"):
		return false
	}

	gzipOnce.RLock()
	body, ok := gzipped[name]
	gzipOnce.RUnlock()
	if !ok {
		raw, err := fs.ReadFile(files, name)
		if err != nil {
			return false
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if _, err := zw.Write(raw); err != nil || zw.Close() != nil {
			return false
		}
		body = buf.Bytes()
		gzipOnce.Lock()
		gzipped[name] = body
		gzipOnce.Unlock()
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Type", contentType(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return true
	}
	w.Write(body) //nolint:errcheck
	return true
}

var (
	gzipOnce sync.RWMutex
	gzipped  = map[string][]byte{}
)

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".webmanifest"):
		return "application/manifest+json"
	}
	return "application/octet-stream"
}

func ReturnTo(r *http.Request, fallback string) string {
	to := r.Form.Get("return")
	if to == "" || to[0] != '/' || strings.HasPrefix(to, "//") {
		return fallback
	}
	return to
}

func Forbidden(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Forbidden"
	}
	Error(w, r, http.StatusForbidden, message)
}

func Log(pkg string, format string, args ...interface{}) {
	logLine(pkg, format, args...)
	appendSysLog(pkg, format, args...)
}

func logLine(pkg string, format string, args ...interface{}) {
	color := pkgColors[pkg]
	if color == "" {
		color = colorWhite
	}
	timestamp := time.Now().Format("15:04:05")
	if cliMode {
		return
	}
	if w := logDest(); w != os.Stdout {
		fmt.Fprintf(w, "[%s %s] "+format+"\n", append([]interface{}{timestamp, pkg}, args...)...)
		return
	}
	prefix := fmt.Sprintf("%s[%s %s]%s ", color, timestamp, pkg, colorReset)
	fmt.Printf(prefix+format+"\n", args...)
}

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
	_, acc := auth.TrySession(r)
	page := renderWithLang(errorTitle(status), message, body, UserLanguage(r), acc)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(page))
}

func BadRequest(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Bad request"
	}
	Error(w, r, http.StatusBadRequest, message)
}

func ServerError(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Internal server error"
	}
	Error(w, r, http.StatusInternalServerError, message)
}

var EmailSender func(to, subject, bodyPlain, bodyHTML, replyTo string) error

func PublicURL() string {
	if v := os.Getenv("PUBLIC_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := os.Getenv("MAIL_DOMAIN"); v != "" {
		return "https://" + v
	}
	return ""
}

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

func renderShell(lang, title, desc, bodyAttr, body string, acc *auth.Account, path, here string) string {
	template := Template
	browserTitle := title
	if title != "" && title != "Micro" {
		browserTitle += " | Micro"
	} else {
		browserTitle = "Micro"
	}
	heading := ""
	if title != "" {
		heading = `<h1 id="page-title">` + htmlpkg.EscapeString(title) + `</h1>`
	}
	return fmt.Sprintf(template,
		lang, htmlpkg.EscapeString(browserTitle), desc, bodyAttr,
		headCorner(acc, here),
		navMain(acc),
		navPinned(acc),
		navBottom(acc, here),
		heading, body, footerFor(acc))
}
