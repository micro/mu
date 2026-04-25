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
	"sync"
	"time"

	"mu/internal/auth"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// Signup rate limiting per IP — defends against bulk account creation.
// Configurable via SIGNUP_MAX_PER_IP and SIGNUP_WINDOW_HOURS env vars.
var (
	signupMu       sync.Mutex
	signupAttempts = map[string]*signupBucket{}
)

type signupBucket struct {
	count    int
	resetAt  time.Time
}

// SignupRateLimit returns true if the IP is allowed to sign up.
// It also records the attempt against the bucket on success.
// Configurable via SIGNUP_MAX_PER_IP (default 3) and SIGNUP_WINDOW_HOURS (default 24).
func SignupRateLimit(ip string) bool {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return true // never rate-limit localhost (self-hosted, dev)
	}
	maxPerIP := envInt("SIGNUP_MAX_PER_IP", 3)
	window := time.Duration(envInt("SIGNUP_WINDOW_HOURS", 24)) * time.Hour

	signupMu.Lock()
	defer signupMu.Unlock()

	now := time.Now()
	b, ok := signupAttempts[ip]
	if !ok || now.After(b.resetAt) {
		b = &signupBucket{count: 0, resetAt: now.Add(window)}
		signupAttempts[ip] = b
	}
	if b.count >= maxPerIP {
		return false
	}
	b.count++

	// Opportunistic GC to avoid unbounded growth.
	if len(signupAttempts) > 10000 {
		for k, v := range signupAttempts {
			if now.After(v.resetAt) {
				delete(signupAttempts, k)
			}
		}
	}
	return true
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}

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

// Error writes an error response, using JSON if the client expects it, otherwise plain text
func Error(w http.ResponseWriter, r *http.Request, status int, message string) {
	if WantsJSON(r) || SendsJSON(r) {
		RespondError(w, status, message)
		return
	}
	http.Error(w, message, status)
}

// Unauthorized writes a 401 error response
func Unauthorized(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusUnauthorized, "Authentication required")
}

// Forbidden writes a 403 error response
func Forbidden(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Forbidden"
	}
	Error(w, r, http.StatusForbidden, message)
}

// BadRequest writes a 400 error response
func BadRequest(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Bad request"
	}
	Error(w, r, http.StatusBadRequest, message)
}

// NotFound writes a 404 error response
func NotFound(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Not found"
	}
	Error(w, r, http.StatusNotFound, message)
}

// ServerError writes a 500 error response
func ServerError(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Internal server error"
	}
	Error(w, r, http.StatusInternalServerError, message)
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
    <link rel="preload" href="/reminder.svg?` + Version + `" as="image">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="stylesheet" href="/mu.css?` + Version + `">
    <script src="/mu.js?` + Version + `"></script>
  </head>
  <body%s>
    <div id="head">
      <button id="menu-toggle" onclick="toggleMenu()" aria-label="Menu"><span></span><span></span><span></span></button>
      <div id="brand">
        <a href="/">Mu</a>
      </div>
      <a id="head-mail" href="/mail" aria-label="Mail"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="16" rx="2"/><polyline points="22,7 12,13 2,7"/></svg><span id="head-mail-badge"></span></a>
    </div>

    <div id="nav-overlay" onclick="toggleMenu()"></div>
    <div id="container">
      <div id="nav-container">
        <div id="nav-search">
          <form action="/search" method="GET">
            <input type="text" name="q" placeholder="Search..." aria-label="Search">
          </form>
        </div>
        <div id="nav">
          <a href="/home"><img src="/home.png?` + Version + `"><span class="label">Home</span></a>
          <a href="/apps"><img src="/apps.svg?` + Version + `"><span class="label">Apps</span></a>
          <a href="/agent"><img src="/agent.svg?` + Version + `"><span class="label">Agent</span></a>
          <a href="/blog"><img src="/post.png?` + Version + `"><span class="label">Blog</span></a>
          <a href="/chat"><img src="/chat.png?` + Version + `"><span class="label">Chat</span></a>
          <a id="nav-mail" href="/mail"><img src="/mail.png?` + Version + `"><span class="label">Mail</span><span id="nav-mail-badge"></span></a>
          <a href="/news"><img src="/news.png?` + Version + `"><span class="label">News</span></a>
          <a href="/social"><img src="/social.svg?` + Version + `"><span class="label">Social</span></a>
          <a href="/video"><img src="/video.png?` + Version + `"><span class="label">Video</span></a>
          <a href="/web"><img src="/search.svg?` + Version + `"><span class="label">Web</span></a>
          <a href="/work"><img src="/work.svg?` + Version + `"><span class="label">Work</span></a>
          <a id="nav-wallet" href="/wallet"><img src="/wallet.png?` + Version + `"><span class="label">Wallet</span></a>

        </div>
        <div class="nav-bottom">
          <div id="nav-username" style="display: none;"></div>
          <a id="nav-account" href="/account" style="display: none;"><img src="/account.png?` + Version + `"><span class="label">Account</span></a>
          <a id="nav-logout" href="/logout" style="display: none;"><img src="/logout.png?` + Version + `"><span class="label">Logout</span></a>
          <a id="nav-login" href="/login"><img src="/account.png?` + Version + `"><span class="label">Login</span></a>
        </div>
      </div>
      <div id="content">
        <h1 id="page-title">%s</h1>
        %s
      </div>
      <div id="footer">
        <a href="/api">API</a> · <a href="/docs">Docs</a> · <a href="/mcp">MCP</a> · <a href="/status">Status</a>` + torFooterLink() + `
      </div>
    </div>
  <script>
      if (navigator.serviceWorker) {
        navigator.serviceWorker.register (
          '/mu.js',
          {scope: '/'}
        );
      }
      
      function toggleMenu() {
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

var LoginTemplate = `<html lang="en">
  <head>
    <title>Login | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="referrer" content="no-referrer"/>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/mu.css?` + Version + `">
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
    </div>
    <div id="container">
      <div id="content">
	<form id="login" action="/login%s" method="POST">
	  <h1>Login</h1>
	  %s
	  <input id="id" name="id" placeholder="Username" required>
	  <input id="secret" name="secret" type="password" placeholder="Password" required>
	  <br>
	  <button>Login</button>
	</form>
	<div id="passkey-login" style="display:none; text-align:center; margin-top:20px;">
	  <p class="text-muted">or</p>
	  <button onclick="loginWithPasskey()">Login with Passkey</button>
	</div>
	<p class="text-center mt-5"><a href="/signup">Sign up</a> if you don't have an account</p>
	<script>
	if (window.PublicKeyCredential) {
	  PublicKeyCredential.isConditionalMediationAvailable && PublicKeyCredential.isConditionalMediationAvailable().then(function(){});
	  document.getElementById('passkey-login').style.display = 'block';
	}

	function base64urlToBuffer(b64) {
	  var pad = b64.length %% 4;
	  if (pad) b64 += '='.repeat(4 - pad);
	  var str = atob(b64.replace(/-/g, '+').replace(/_/g, '/'));
	  var buf = new Uint8Array(str.length);
	  for (var i = 0; i < str.length; i++) buf[i] = str.charCodeAt(i);
	  return buf.buffer;
	}

	function bufferToBase64url(buf) {
	  var bytes = new Uint8Array(buf);
	  var str = '';
	  for (var i = 0; i < bytes.length; i++) str += String.fromCharCode(bytes[i]);
	  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
	}

	async function loginWithPasskey() {
	  try {
	    var beginRes = await fetch('/passkey/login/begin', {method: 'POST'});
	    if (!beginRes.ok) { alert('Passkey login not available'); return; }
	    var options = await beginRes.json();

	    options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
	    if (options.publicKey.allowCredentials) {
	      options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(function(c) {
	        return Object.assign({}, c, {id: base64urlToBuffer(c.id)});
	      });
	    }

	    var assertion = await navigator.credentials.get(options);

	    var body = {
	      id: assertion.id,
	      rawId: bufferToBase64url(assertion.rawId),
	      type: assertion.type,
	      response: {
	        authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
	        clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
	        signature: bufferToBase64url(assertion.response.signature),
	        userHandle: bufferToBase64url(assertion.response.userHandle)
	      }
	    };
	    if (assertion.authenticatorAttachment) {
	      body.authenticatorAttachment = assertion.authenticatorAttachment;
	    }

	    var finishRes = await fetch('/passkey/login/finish', {
	      method: 'POST',
	      headers: {'Content-Type': 'application/json'},
	      body: JSON.stringify(body)
	    });
	    var result = await finishRes.json();
	    if (result.success) {
	      window.location.href = result.redirect || '/home';
	    } else {
	      alert('Login failed');
	    }
	  } catch (e) {
	    if (e.name !== 'NotAllowedError') alert('Error: ' + e.message);
	  }
	}
	</script>
      </div>
    </div>
  </body>
</html>
`

var SignupTemplate = `<html lang="en">
  <head>
    <title>Signup | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="referrer" content="no-referrer"/>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/mu.css?` + Version + `">
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
    </div>
    <div id="container">
      <div id="content">
	<form id="signup" action="/signup" method="POST">
	  <h1>Signup</h1>
	  %s
	  <input id="id" name="id" placeholder="Username (4-24 chars, lowercase)" required>
	  <input id="name" name="name" placeholder="Name (optional)">
  	  <input id="secret" name="secret" type="password" placeholder="Password (min 6 chars)" required>
	  %s
	  %s
	  <br>
	  <button>Signup</button>
	</form>
	<p class="text-center mt-5"><a href="/login">Login</a> if you have an account</p>
      </div>
    </div>
  </body>
</html>
`

// inviteCode is a package-level var used to thread the invite code
// through signup renders without changing every call site.
var currentInviteCode string

// renderSignup renders the signup template with a fresh captcha challenge
// and the given error HTML (or empty string).
func renderSignup(errHTML string) string {
	c := NewCaptchaChallenge()
	inviteField := ""
	if currentInviteCode != "" {
		inviteField = fmt.Sprintf(`<input type="hidden" name="invite" value="%s">`, currentInviteCode)
	}
	return fmt.Sprintf(SignupTemplate, errHTML, CaptchaHTML(c), inviteField)
}

// renderRequestInvitePage shows the "request an invite" form that
// replaces the dead-end "invite only" page. Captcha-protected and
// rate-limited by IP so it can't be flooded.
func renderRequestInvitePage(w http.ResponseWriter, r *http.Request, message string) {
	c := NewCaptchaChallenge()
	msg := message
	if msg == "" {
		msg = `<p>Mu is currently invite-only. Leave your email and we'll send you an invite when we open up more seats.</p>`
	}
	body := fmt.Sprintf(`<div class="card" style="max-width:440px;margin:0 auto">
<h3>Request an invite</h3>
%s
<form method="POST" action="/request-invite" style="margin-top:12px">
  <input type="email" name="email" placeholder="your@email.com" required style="width:100%%;margin-bottom:8px">
  <input type="text" name="reason" placeholder="Why you'd like to join (optional)" maxlength="500" style="width:100%%;margin-bottom:8px">
  %s
  <button type="submit">Request invite</button>
</form>
<p class="text-muted text-sm mt-3">Already have an invite? <a href="/login">Log in</a> or paste your link.</p>
</div>`, msg, CaptchaHTML(c))
	w.Write([]byte(RenderHTML("Request an Invite", "Request an invite to Mu", body)))
}

// RequestInvite handles POST /request-invite — someone is asking to
// join. Validates captcha + rate limit, stores the request for admin
// review.
func RequestInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		renderRequestInvitePage(w, r, "")
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()

	if err := VerifyCaptchaRequest(r); err != nil {
		renderRequestInvitePage(w, r, fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))
		return
	}

	// Per-IP rate limit reuses the signup bucket — same spam concern.
	ip := ClientIP(r)
	if !SignupRateLimit(ip) {
		renderRequestInvitePage(w, r, `<p class="text-error">Too many requests from your network. Please try again later.</p>`)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	reason := strings.TrimSpace(r.FormValue("reason"))
	if email == "" || !strings.Contains(email, "@") {
		renderRequestInvitePage(w, r, `<p class="text-error">Please enter a valid email address.</p>`)
		return
	}

	if err := auth.CreateInviteRequest(email, reason, ip); err != nil {
		renderRequestInvitePage(w, r, fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))
		return
	}
	Log("auth", "Invite request from %s (%s)", email, ip)

	body := fmt.Sprintf(`<div class="card" style="max-width:440px;margin:0 auto">
<h3>Thanks — we got your request</h3>
<p>We'll email <strong>%s</strong> if we have a seat for you.</p>
<p class="mt-3"><a href="/">← Back</a></p>
</div>`, htmlpkg.EscapeString(email))
	w.Write([]byte(RenderHTML("Request Received", "Invite request received", body)))
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

// Login handler
func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Preserve redirect parameter in form action
		redirectParam := ""
		if redirect := r.URL.Query().Get("redirect"); redirect != "" {
			redirectParam = "?redirect=" + url.QueryEscape(redirect)
		}
		w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, "")))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		secret := r.Form.Get("secret")

		// Preserve redirect parameter for error messages
		redirectParam := ""
		if redirect := r.URL.Query().Get("redirect"); redirect != "" {
			redirectParam = "?redirect=" + url.QueryEscape(redirect)
		}

		if len(id) == 0 {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, `<p class="text-error">Username is required</p>`)))
			return
		}
		if len(secret) == 0 {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, `<p class="text-error">Password is required</p>`)))
			return
		}

		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, `<p class="text-error">Invalid username or password</p>`)))
			return
		}

		var secure bool

		if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
			secure = true
		}

		// set a new token
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    sess.Token,
			Path:     "/",
			MaxAge:   2592000,
			Secure:   secure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		// Check for redirect parameter, default to home
		redirectTo := r.URL.Query().Get("redirect")
		if redirectTo == "" || redirectTo[0] != '/' || strings.HasPrefix(redirectTo, "//") {
			redirectTo = "/home"
		}
		http.Redirect(w, r, redirectTo, 302)
		return
	}
}

// Signup handler
func Signup(w http.ResponseWriter, r *http.Request) {
	// Thread the invite code through renders so the hidden field persists.
	invCode := r.URL.Query().Get("invite")
	if r.Method == "POST" {
		if v := r.FormValue("invite"); v != "" {
			invCode = v
		}
	}
	currentInviteCode = invCode

	// Invite-only mode: reject if no valid code is provided, but show
	// a request-invite form instead of a dead end.
	if auth.InviteOnly() {
		if err := auth.ValidateInvite(invCode); err != nil {
			if invCode == "" {
				renderRequestInvitePage(w, r, "")
				return
			}
			w.Write([]byte(renderSignup(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))))
			return
		}
	}

	if r.Method == "GET" {
		w.Write([]byte(renderSignup("")))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		// Captcha is checked before the IP rate limit so that a failed
		// captcha doesn't burn an attempt against the IP bucket.
		if err := VerifyCaptchaRequest(r); err != nil {
			w.Write([]byte(renderSignup(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))))
			return
		}

		// Per-IP signup rate limit (defends against bulk account creation).
		ip := ClientIP(r)
		if !SignupRateLimit(ip) {
			Log("auth", "Signup rate limit hit for IP: %s", ip)
			w.Write([]byte(renderSignup(`<p class="text-error">Too many sign-ups from your network. Please try again later.</p>`)))
			return
		}

		id := r.Form.Get("id")
		name := r.Form.Get("name")
		secret := r.Form.Get("secret")

		const usernamePattern = "^[a-z][a-z0-9_]{3,23}$"

		usernameRegex := regexp.MustCompile(usernamePattern)

		if len(id) == 0 {
			w.Write([]byte(renderSignup(`<p class="text-error">Username is required</p>`)))
			return
		}

		if !usernameRegex.MatchString(id) {
			w.Write([]byte(renderSignup(`<p class="text-error">Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores</p>`)))
			return
		}

		if reason := auth.ValidateUsername(id); reason != "" {
			w.Write([]byte(renderSignup(fmt.Sprintf(`<p class="text-error">%s</p>`, reason))))
			return
		}

		if len(secret) == 0 {
			w.Write([]byte(renderSignup(`<p class="text-error">Password is required</p>`)))
			return
		}

		if len(secret) < 6 {
			w.Write([]byte(renderSignup(`<p class="text-error">Password must be at least 6 characters</p>`)))
			return
		}

		// Use username as name if name is not provided
		if len(name) == 0 {
			name = id
		}

		if err := auth.Create(&auth.Account{
			ID:      id,
			Secret:  secret,
			Name:    name,
			Created: time.Now(),
		}); err != nil {
			w.Write([]byte(renderSignup(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))))
			return
		}

		// Consume invite code if present (marks it as used).
		if invCode != "" {
			auth.ConsumeInvite(invCode, id)
		}

		// login
		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(renderSignup(`<p class="text-error">Account created but login failed. Please try logging in.</p>`)))
			return
		}

		var secure bool

		if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
			secure = true
		}

		// set a new token
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    sess.Token,
			Path:     "/",
			MaxAge:   2592000,
			Secure:   secure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		// return to home
		http.Redirect(w, r, "/home", 302)
		return
	}
}

func Account(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Handle POST to update language or request email verification
	if r.Method == "POST" {
		r.ParseForm()

		// Language update
		if newLang := r.Form.Get("language"); newLang != "" {
			if _, ok := SupportedLanguages[newLang]; ok {
				acc.Language = newLang
				auth.UpdateAccount(acc)
			}
			http.Redirect(w, r, "/account", http.StatusSeeOther)
			return
		}

		// Email verification request
		if email := strings.TrimSpace(r.Form.Get("email")); email != "" {
			handleVerifyStart(w, r, acc, email)
			return
		}

		// Home card preferences
		if r.Form.Get("home_cards") != "" || r.Form.Get("save_cards") != "" {
			selected := r.Form["cards"]
			acc.HomeCards = selected
			auth.UpdateAccount(acc)
			http.Redirect(w, r, "/account", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}

	// Build language options
	currentLang := acc.Language
	if currentLang == "" {
		currentLang = "en"
	}
	languageOptions := ""
	for code, name := range SupportedLanguages {
		selected := ""
		if code == currentLang {
			selected = " selected"
		}
		languageOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, code, selected, name)
	}

	// Admin link
	adminLinks := ""
	if acc.Admin {
		adminLinks = `<p><a href="/admin">Admin Dashboard →</a></p>`
	}

	// Email verification card
	emailCard := renderEmailCard(acc)

	// Home card preferences
	allCards := []struct{ id, label string }{
		{"reminder", "Reminder"}, {"blog", "Blog"}, {"news", "News"},
		{"markets", "Markets"}, {"social", "Social"}, {"video", "Video"},
	}
	activeCards := map[string]bool{}
	if len(acc.HomeCards) > 0 {
		for _, id := range acc.HomeCards {
			activeCards[id] = true
		}
	} else {
		for _, c := range allCards {
			activeCards[c.id] = true
		}
	}
	var cardsCheckboxes string
	for _, c := range allCards {
		checked := ""
		if activeCards[c.id] {
			checked = " checked"
		}
		cardsCheckboxes += fmt.Sprintf(`<label style="display:inline-flex;align-items:center;gap:4px;margin-right:12px;font-size:14px"><input type="checkbox" name="cards" value="%s"%s> %s</label>`, c.id, checked, c.label)
	}
	homeCardsCard := fmt.Sprintf(`<div class="card">
<h4>Home Screen</h4>
<p class="text-sm text-muted">Choose which cards to show on your home overview.</p>
<form action="/account" method="POST" style="margin-top:8px">
<input type="hidden" name="save_cards" value="1">
<div style="display:flex;flex-wrap:wrap;gap:4px 0;margin-bottom:8px">%s</div>
<button type="submit">Save</button>
</form>
</div>`, cardsCheckboxes)

	content := fmt.Sprintf(`<div class="card">
<h4>Profile</h4>
<p><strong>%s</strong> · %s · Joined %s</p>
<p><a href="/@%s">Public profile →</a></p>
</div>

%s

<div class="card">
<h4>Language</h4>
<form action="/account" method="POST" class="d-flex items-center gap-3">
	<select name="language" class="form-select text-sm">%s</select>
	<button type="submit">Save</button>
</form>
</div>

%s

%s

<div class="card">
<h4>Settings</h4>
%s
<p><a href="/token">API Credentials →</a></p>
<p><a href="/app/blocked">Blocked Users →</a></p>
<p><a href="/app/saved">Saved →</a></p>
<p style="margin-top:12px"><a href="/logout" class="text-error">Logout</a></p>
</div>`,
		acc.ID,
		acc.Name,
		acc.Created.Format("January 2, 2006"),
		acc.ID,
		emailCard,
		languageOptions,
		homeCardsCard,
		PasskeyListHTML(acc.ID),
		adminLinks,
	)

	html := RenderHTML("Account", "Account", content)
	w.Write([]byte(html))
}

// renderEmailCard renders the email verification card on the account
// page. The card looks different depending on whether the email is set,
// pending, or verified — and whether email sending is configured at all.
func renderEmailCard(acc *auth.Account) string {
	if acc.Admin || acc.Approved {
		// Admins/approved users don't need verification.
		if acc.EmailVerified {
			return fmt.Sprintf(`<div class="card"><h4>Email</h4><p>%s — verified</p></div>`, htmlpkg.EscapeString(acc.Email))
		}
		return ""
	}

	if EmailSender == nil {
		return `<div class="card"><h4>Email</h4><p class="text-muted">Email verification is not configured on this instance.</p></div>`
	}

	if acc.EmailVerified {
		return fmt.Sprintf(`<div class="card">
<h4>Email</h4>
<p><strong>%s</strong> — verified ✓</p>
</div>`, htmlpkg.EscapeString(acc.Email))
	}

	pending := ""
	if acc.Email != "" {
		pending = fmt.Sprintf(`<p class="text-muted text-sm">A verification link was sent to <strong>%s</strong>. Click it to unlock posting. Submit again to resend.</p>`, htmlpkg.EscapeString(acc.Email))
	}

	return fmt.Sprintf(`<div class="card">
<h4>Verify your email to post</h4>
<p class="text-sm">Verifying your email unlocks status updates, replies, comments and blog posts. We do not share or sell your address.</p>
%s
<form action="/account" method="POST" class="d-flex items-center gap-3" style="margin-top:8px">
	<input type="email" name="email" placeholder="you@example.com" value="%s" required>
	<button type="submit">Send verification</button>
</form>
</div>`, pending, htmlpkg.EscapeString(acc.Email))
}

// handleVerifyStart processes the email submission on /account, generates
// a verification token, and sends an email containing the verify link.
func handleVerifyStart(w http.ResponseWriter, r *http.Request, acc *auth.Account, email string) {
	if EmailSender == nil {
		Forbidden(w, r, "Email verification is not configured on this instance.")
		return
	}
	if !validEmail(email) {
		BadRequest(w, r, "Please enter a valid email address.")
		return
	}

	// Persist the pending email so the UI can show it.
	if err := auth.SetAccountEmail(acc.ID, email); err != nil {
		ServerError(w, r, "Failed to save email")
		return
	}

	tok, err := auth.CreateEmailVerificationToken(acc.ID, email)
	if err != nil {
		ServerError(w, r, "Failed to create verification token")
		return
	}

	link := PublicURL() + "/verify?token=" + tok
	plain := fmt.Sprintf("Hi %s,\n\nClick the link below to verify your email and unlock posting on Mu:\n\n%s\n\nThis link expires in 24 hours. If you didn't request this, you can ignore this email.\n\n— Mu", acc.Name, link)
	html := fmt.Sprintf(`<p>Hi %s,</p><p>Click the link below to verify your email and unlock posting on Mu:</p><p><a href="%s">%s</a></p><p>This link expires in 24 hours. If you didn't request this, you can ignore this email.</p><p>— Mu</p>`, htmlpkg.EscapeString(acc.Name), link, link)

	if err := EmailSender(email, "Verify your Mu account", plain, html); err != nil {
		Log("auth", "Failed to send verification email to %s: %v", email, err)
		ServerError(w, r, "Failed to send verification email. Please try again.")
		return
	}
	Log("auth", "Sent verification email to %s for account %s", email, acc.ID)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// Verify handles GET /verify?token=XXX — consumes a verification token
// and marks the account as verified.
func Verify(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		BadRequest(w, r, "Missing verification token")
		return
	}
	acc, err := auth.ConsumeEmailVerificationToken(token)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	Log("auth", "Email verified for account %s (%s)", acc.ID, acc.Email)

	body := fmt.Sprintf(`<div class="card">
<h4>Email verified ✓</h4>
<p>Thanks, <strong>%s</strong>. Your email is verified and you can now post.</p>
<p><a href="/home" class="btn">Go home</a> &nbsp; <a href="/account">Account →</a></p>
</div>`, htmlpkg.EscapeString(acc.Name))
	html := RenderHTML("Verified", "Email verified", body)
	w.Write([]byte(html))
}

// validEmail performs minimal sanity checking — the real check is whether
// the verification email actually arrives and is clicked.
func validEmail(s string) bool {
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

func Logout(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		http.Redirect(w, r, "/", 302)
		return
	}

	var secure bool

	if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
		secure = true
	}
	// delete the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	auth.Logout(sess.Token)
	http.Redirect(w, r, "/", 302)
}

// Session handler
func Session(w http.ResponseWriter, r *http.Request) {
	sess, acc := auth.TrySession(r)
	if sess == nil {
		// Return guest session instead of error
		guestSess := map[string]interface{}{
			"type": "guest",
		}
		b, _ := json.Marshal(guestSess)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	// Build response with account info
	response := map[string]interface{}{
		"id":      sess.ID,
		"type":    sess.Type,
		"account": sess.Account,
		"created": sess.Created,
	}

	if acc != nil {
		response["admin"] = acc.Admin
	}

	b, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}


// Render a markdown document as html
func Render(md []byte) []byte {
	// Strip LaTeX dollar sign escapes before parsing markdown
	md = []byte(StripLatexDollars(string(md)))

	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
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
	return RenderHTMLWithLang(title, desc, html, "en")
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
	return RenderHTMLWithLang(title, desc, html, lang)
}

// VerifyBanner returns banner HTML inviting the user to verify their
// email address, or an empty string if the banner doesn't apply (no
// session, admin, already verified, or verification not required on
// this instance).
func VerifyBanner(r *http.Request) string {
	if EmailSender == nil {
		return ""
	}
	_, acc := auth.TrySession(r)
	if acc == nil || acc.Admin || acc.Approved || acc.EmailVerified {
		return ""
	}
	// Don't show the banner on the account page itself — the verify
	// form is right there.
	if r.URL.Path == "/account" || r.URL.Path == "/verify" {
		return ""
	}
	return `<div class="verify-banner" style="background:#fff8e1;border:1px solid #f1d68c;border-radius:6px;padding:10px 14px;margin:0 0 14px;font-size:14px;color:#5b4a00;display:flex;align-items:center;gap:10px;flex-wrap:wrap">
<strong>Verify your email to post.</strong>
<span>Add and confirm an email on your account to unlock status updates, replies, comments and posts.</span>
<a href="/account" style="margin-left:auto;background:#000;color:#fff;text-decoration:none;padding:6px 14px;border-radius:6px">Verify →</a>
</div>`
}

// RenderHTMLWithLang renders the given html in a template with specified language
func RenderHTMLWithLang(title, desc, html, lang string) string {
	if lang == "" {
		lang = "en"
	}
	return fmt.Sprintf(Template, lang, title, desc, "", title, html)
}

// RenderHTMLWithLangAndBody renders html with a custom body attribute string
// (e.g. ` class="page-home"` to enable page-specific CSS).
func RenderHTMLWithLangAndBody(title, desc, html, lang, bodyAttr string) string {
	if lang == "" {
		lang = "en"
	}
	return fmt.Sprintf(Template, lang, title, desc, bodyAttr, title, html)
}

// RenderString renders a markdown string as html
func RenderString(v string) string {
	return string(Render([]byte(v)))
}

// RenderTemplate renders a markdown string in a html template
func RenderTemplate(title string, desc, text string) string {
	return fmt.Sprintf(Template, "en", title, desc, "", title, RenderString(text))
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
