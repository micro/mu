package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"mu/auth"

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
func Log(pkg string, format string, args ...interface{}) {
	color := pkgColors[pkg]
	if color == "" {
		color = colorWhite
	}
	timestamp := time.Now().Format("15:04:05")
	prefix := fmt.Sprintf("%s[%s %s]%s ", color, timestamp, pkg, colorReset)
	fmt.Printf(prefix+format+"\n", args...)
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
        <div id="nav">
          <a href="/home"><img src="/home.png?` + Version + `"><span class="label">Home</span></a>
          <a href="/blog"><img src="/post.png?` + Version + `"><span class="label">Blog</span></a>
          <a href="/chat"><img src="/chat.png?` + Version + `"><span class="label">Chat</span></a>
          <a id="nav-mail" href="/mail" style="display: none;"><img src="/mail.png?` + Version + `"><span class="label">Mail</span><span id="nav-mail-badge"></span></a>
          <a href="/news"><img src="/news.png?` + Version + `"><span class="label">News</span></a>
          <a href="/video"><img src="/video.png?` + Version + `"><span class="label">Video</span></a>
          <a href="/markets"><img src="/markets.png?` + Version + `"><span class="label">Markets</span></a>
          <a href="/places"><img src="/places.png?` + Version + `"><span class="label">Places</span></a>
          <a href="/weather"><img src="/weather.png?` + Version + `"><span class="label">Weather</span></a>
          <a id="nav-wallet" href="/wallet" style="display: none;"><img src="/wallet.png?` + Version + `"><span class="label">Wallet</span></a>
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
        <a href="/about">About</a> · <a href="/docs">Docs</a> · <a href="/api">API</a> · <a href="/mcp">MCP</a> · <a href="/plans">Plans</a> · <a href="/status">Status</a>
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
		head += fmt.Sprintf(`<a href="/%s#%s" class="head">%s</a>`, appName, ref, ref)
	}

	return head
}

func Card(id, title, content string) string {
	return fmt.Sprintf(CardTemplate, id, id, title, content)
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
		if redirectTo == "" {
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

// Plans handler - shows available options
func Plans(w http.ResponseWriter, r *http.Request) {
	// Check if user is logged in and their status
	isAdmin := false
	isLoggedIn := false
	_, acc := auth.TrySession(r)
	if acc != nil {
		isLoggedIn = true
		isAdmin = acc.Admin
	}

	var content strings.Builder

	// Philosophy note
	content.WriteString(`<p class="mb-5 text-muted">Pay for what you use, nothing more.</p>`)

	// 2-column pricing grid with responsive class
	content.WriteString(`<div class="pricing-grid">`)

	// Free tier
	content.WriteString(`<div class="card">
<h3>Free</h3>
<p class="text-xl font-bold my-3">£0</p>
<p>10 credits per day</p>
<p>News, video, and chat</p>
<p>Direct message other users</p>
<p>MCP access for AI agents</p>
<p>Resets at midnight UTC</p>`)
	if !isLoggedIn {
		content.WriteString(`<p class="mt-4"><a href="/signup">Sign up →</a></p>`)
	} else if isAdmin {
		content.WriteString(`<p class="mt-4 text-muted">You have full access</p>`)
	} else {
		content.WriteString(`<p class="mt-4 text-muted">Your current plan</p>`)
	}
	content.WriteString(`</div>`)

	// Pay as you go
	content.WriteString(`<div class="card">
<h3>Pay as you go</h3>
<p class="text-xl font-bold my-3">From £5</p>
<p>Top up your wallet</p>
<p>1 credit = 1p</p>
<p>News 1p · Video 2p · Chat 3p · Email 4p · Places 5p</p>
<p>Nearby places 2p</p>
<p>Same rates for agents via MCP</p>
<p>Credits never expire</p>`)
	if isLoggedIn && !isAdmin {
		content.WriteString(`<p class="mt-4"><a href="/wallet/topup">Top up →</a></p>`)
	} else if !isLoggedIn {
		content.WriteString(`<p class="mt-4"><a href="/signup">Sign up first →</a></p>`)
	} else {
		content.WriteString(`<p class="mt-4 text-muted">You have full access</p>`)
	}
	content.WriteString(`</div>`)

	content.WriteString(`</div>`) // end grid

	// Pricing table
	content.WriteString(`<h3>Pricing</h3>
<p class="text-muted mb-4">1 credit = 1p (one penny). Free quota: <strong>10 credits per day</strong>, resets at midnight UTC.</p>
<table class="data-table">
<thead>
<tr><th>Tool / Endpoint</th><th>Description</th><th>Credits</th><th>Cost</th></tr>
</thead>
<tbody>
<tr><td>News feed</td><td>Browse the latest news</td><td>Free</td><td>—</td></tr>
<tr><td>News search</td><td>AI-powered news article search</td><td>1</td><td>1p</td></tr>
<tr><td>News summary</td><td>AI summary of a news article</td><td>1</td><td>1p</td></tr>
<tr><td>Video feed</td><td>Browse latest videos</td><td>Free</td><td>—</td></tr>
<tr><td>Video watch</td><td>Watch a video</td><td>Free</td><td>—</td></tr>
<tr><td>Video search</td><td>Search for videos</td><td>2</td><td>2p</td></tr>
<tr><td>Chat</td><td>Chat with AI assistant</td><td>3</td><td>3p</td></tr>
<tr><td>Blog read</td><td>Read blog posts</td><td>Free</td><td>—</td></tr>
<tr><td>Blog write</td><td>Create or update a blog post</td><td>Free</td><td>—</td></tr>
<tr><td>Mail (internal)</td><td>Message other Mu users</td><td>Free</td><td>—</td></tr>
<tr><td>Mail (external)</td><td>Send email outside Mu (SMTP)</td><td>4</td><td>4p</td></tr>
<tr><td>Places search</td><td>Search for places by name or category</td><td>5</td><td>5p</td></tr>
<tr><td>Places nearby</td><td>Find places of interest near a location</td><td>2</td><td>2p</td></tr>
<tr><td>Weather forecast</td><td>Local weather with hourly &amp; 10-day forecast</td><td>1</td><td>1p</td></tr>
<tr><td>Weather pollen</td><td>Local pollen forecast (add-on)</td><td>1</td><td>1p</td></tr>
<tr><td>Markets</td><td>Live crypto, futures &amp; commodity prices</td><td>Free</td><td>—</td></tr>
<tr><td>Search</td><td>Full-text search across all indexed content</td><td>Free</td><td>—</td></tr>
<tr><td>Wallet</td><td>Check balance and top up</td><td>Free</td><td>—</td></tr>
</tbody>
</table>`)

	// Coming soon
	content.WriteString(`<h3>Coming Soon</h3>
<table class="data-table">
<thead>
<tr><th>Tool / Endpoint</th><th>Description</th><th>Estimated Cost</th></tr>
</thead>
<tbody>
<tr><td>Translate</td><td>AI-powered language translation</td><td>1p</td></tr>
<tr><td>Image search</td><td>Search for images</td><td>2p</td></tr>
<tr><td>Calendar</td><td>Events and reminders</td><td>Free</td></tr>
<tr><td>Directions</td><td>Route planning between locations</td><td>2p</td></tr>
</tbody>
</table>`)

	// Agents / MCP section
	content.WriteString(`<h3>For Agents</h3>
<p>AI agents can connect to Mu via the Model Context Protocol (MCP).</p>
<p>Authenticate with a Bearer token</p>
<p>10 credits per day on the free tier</p>
<p>Same pay-as-you-go rates as human users</p>
<p>Access to chat, news, video, mail and more</p>
<p><a href="/mcp">View MCP tools →</a></p>`)

	// Self-host option
	content.WriteString(`<h3>Self-Host</h3>
<p>Want unlimited and free forever? Run your own instance.</p>
<p>Mu is open source (AGPL-3.0). Your server, your data, no limits.</p>
<p><a href="https://github.com/micro/mu" target="_blank">View on GitHub →</a></p>`)

	// FAQ
	content.WriteString(`<h3>Questions</h3>
<p><strong>Why charge for services?</strong><br>News, video search, chat, and email all rely on APIs and infrastructure that cost money to run. The free quota covers casual daily use.</p>
<p><strong>Do credits expire?</strong><br>No. Once you top up, your credits are yours until you use them.</p>
<p><strong>Why no unlimited subscription?</strong><br>Unlimited tiers incentivize us to maximize your engagement. Pay-as-you-go keeps incentives aligned: we want efficient tools, not sticky products.</p>
<p><strong>Is watching videos free?</strong><br>Yes. We only charge when we add value (search, summaries), not for things YouTube already provides.</p>
<p><strong>Can AI agents use Mu?</strong><br>Yes. Mu supports the <a href="/mcp">Model Context Protocol (MCP)</a>. See the <a href="/mcp">MCP page</a> for setup and available tools.</p>`)

	html := RenderHTMLForRequest("Plans", "Simple, honest pricing", content.String(), r)
	w.Write([]byte(html))
}

// Membership handler
// Donate handler
func Donate(w http.ResponseWriter, r *http.Request) {
	donationURL := os.Getenv("DONATION_URL")

	// Check if donations are enabled
	if donationURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check if coming from payment provider
	referer := r.Header.Get("Referer")
	u, err := url.Parse(donationURL)
	fromPaymentProvider := err == nil && u.Host != "" && strings.Contains(referer, u.Host)

	if fromPaymentProvider {
		content := `<h1>Thank you for your donation!</h1>
			<p>Your generous support helps keep Mu independent and sustainable.</p>
			<p>Every contribution makes a difference in building a better internet.</p>
			<p><a href="/">← Return Home</a></p>`
		html := RenderHTML("Donate", "Thank you!", content)
		w.Write([]byte(html))
		return
	}

	// Show donation page
	content := fmt.Sprintf(`<h2>Support Mu</h2>
		<p>Help us build a better internet, free from ads and algorithms.</p>
		<p>Your one-time donation supports the ongoing development and operation of Mu.</p>
		<h3>Why Donate?</h3>
		<ul>
			<li>Keep Mu ad-free and independent</li>
			<li>Support development of new features</li>
			<li>Help maintain server infrastructure</li>
			<li>Enable us to focus on users, not profits</li>
		</ul>
		<p><a href="%s">Make a Donation →</a></p>`, donationURL)

	html := RenderHTML("Donate", "Support Mu", content)
	w.Write([]byte(html))
}

// Render a markdown document as html
func Render(md []byte) []byte {
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
