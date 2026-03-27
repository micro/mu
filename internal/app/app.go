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

	"github.com/gomarkdown/markdown"
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

// Log prints a formatted log message with a colored package prefix
// and stores it in the in-memory system log ring buffer.
func Log(pkg string, format string, args ...interface{}) {
	color := pkgColors[pkg]
	if color == "" {
		color = colorWhite
	}
	timestamp := time.Now().Format("15:04:05")
	prefix := fmt.Sprintf("%s[%s %s]%s ", color, timestamp, pkg, colorReset)
	fmt.Printf(prefix+format+"\n", args...)
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

	// HTML response
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
      document.addEventListener('click',function(){document.querySelectorAll('.dot-menu>div').forEach(function(m){m.style.display='none'})});
  </script>
  </body>
</html>
`

var CardTemplate = `
<!-- %s -->
<div id="%s" class="card">
  <h4>%s</h4>
  %s
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
	  <br>
	  <button>Signup</button>
	</form>
	<p class="text-center mt-5"><a href="/login">Login</a> if you have an account</p>
      </div>
    </div>
  </body>
</html>
`

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
	if r.Method == "GET" {
		w.Write([]byte(fmt.Sprintf(SignupTemplate, "")))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		name := r.Form.Get("name")
		secret := r.Form.Get("secret")

		const usernamePattern = "^[a-z][a-z0-9_]{3,23}$"

		usernameRegex := regexp.MustCompile(usernamePattern)

		if len(id) == 0 {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p class="text-error">Username is required</p>`)))
			return
		}

		if !usernameRegex.MatchString(id) {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p class="text-error">Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores</p>`)))
			return
		}

		if len(secret) == 0 {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p class="text-error">Password is required</p>`)))
			return
		}

		if len(secret) < 6 {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p class="text-error">Password must be at least 6 characters</p>`)))
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
			w.Write([]byte(fmt.Sprintf(SignupTemplate, fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))))
			return
		}

		// login
		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p class="text-error">Account created but login failed. Please try logging in.</p>`)))
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

	// Handle POST to update language
	if r.Method == "POST" {
		r.ParseForm()
		newLang := r.Form.Get("language")
		if _, ok := SupportedLanguages[newLang]; ok {
			acc.Language = newLang
			auth.UpdateAccount(acc)
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

	content := fmt.Sprintf(`<div class="card">
<h3>Profile</h3>
<p><strong>Username:</strong> %s</p>
<p><strong>Name:</strong> %s</p>
<p><strong>Joined:</strong> %s</p>
<p><a href="/@%s">View public profile →</a></p>
</div>

<div class="card">
<h3>Language</h3>
<form action="/account" method="POST" class="d-flex items-center gap-3">
	<select name="language" class="form-select text-sm">%s</select>
	<button type="submit">Save</button>
</form>
</div>

%s

<div class="card">
<h3>Settings</h3>
<p><a href="/token">API Tokens →</a></p>
%s
<p><a href="/logout" class="text-error">Logout</a></p>
</div>`,
		acc.ID,
		acc.Name,
		acc.Created.Format("January 2, 2006"),
		acc.ID,
		languageOptions,
		PasskeyListHTML(acc.ID),
		adminLinks,
	)

	html := RenderHTML("Account", "Account", content)
	w.Write([]byte(html))
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
	// Raw backslash variants: \( \) \[ \] are math delimiters — strip them
	s = strings.ReplaceAll(s, `\(`, "")
	s = strings.ReplaceAll(s, `\)`, "")
	s = strings.ReplaceAll(s, `\[`, "")
	s = strings.ReplaceAll(s, `\]`, "")
	// Escaped dollar sign: \$ → $
	s = strings.ReplaceAll(s, `\$`, "$")
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

// RenderHTMLForRequest renders the given html in a template using the user's language preference
func RenderHTMLForRequest(title, desc, html string, r *http.Request) string {
	lang := GetUserLanguage(r)
	return RenderHTMLWithLang(title, desc, html, lang)
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
