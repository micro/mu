// Package account is signing in, signing up, and everything on /account.
//
// It lived in internal/app, which is the UI framework: response helpers, the
// page shell, markdown rendering. 127 packages import that framework, and every
// one of them was pulling in the login page, the Google OAuth flow, WebAuthn
// and API token management in order to get app.RespondJSON.
//
// These are pages, and Account is a place in the product, so this is a
// top-level package like home, admin and wallet rather than another corner of
// the framework. What is left in internal/app is what those 127 packages
// actually wanted.

package account

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"mu/internal/app"

	"mu/internal/auth"
)

// SignupRateLimit returns true if the IP is allowed to sign up.
// It also records the attempt against the bucket on success.
// Configurable via SIGNUP_MAX_PER_IP (default 3) and SIGNUP_WINDOW_HOURS (default 24).
func SignupRateLimit(ip string) bool {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return true // never rate-limit localhost (self-hosted, dev)
	}
	maxPerIP := app.EnvInt("SIGNUP_MAX_PER_IP", 3)
	window := time.Duration(app.EnvInt("SIGNUP_WINDOW_HOURS", 24)) * time.Hour

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

var LoginTemplate = `<html lang="en">
  <head>
    <title>Login | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="referrer" content="no-referrer"/>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/mu.css?` + app.Version + `">
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
	  <h1>Log in</h1>
	  <p class="auth-lede">Your agents, your tools, and the app they share.</p>
	  %s
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
	<p class="auth-foot"><a href="/tools">See the tools first &rarr;</a></p>
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
    <link rel="stylesheet" href="/mu.css?` + app.Version + `">
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
    </div>
    <div id="container">
      <div id="content">
	<form id="signup" action="/signup%s" method="POST">
	  <h1>Create your account</h1>
	  <p class="auth-lede">One account, and your agents get every tool on this
	  instance &mdash; news, web search, mail, markets, weather, storage &mdash;
	  over a single MCP server. Free to start.</p>
	  %s
	  %s
	  <input id="id" name="id" placeholder="Username (4-24 chars, lowercase)" required>
	  <input id="name" name="name" placeholder="Name (optional)">
  	  <input id="secret" name="secret" type="password" placeholder="Password (min 6 chars)" required>
	  %s
	  %s
	  <br>
	  <button>Signup</button>
	</form>
	<p class="text-center mt-5"><a href="/login">Log in</a> if you have an account</p>
	<p class="auth-foot"><a href="/tools">See the tools first &rarr;</a></p>
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
func renderSignup(errHTML string) string { return renderSignupTo(errHTML, "") }

// renderSignupTo threads a redirect through the form action. Without it the
// POST goes to a bare /signup and the destination is lost between showing the
// form and submitting it — which is how someone sent here by a client's OAuth
// flow would create an account and then land on /home, with the client still
// waiting.
func renderSignupTo(errHTML, redirectParam string) string {
	c := app.NewCaptchaChallenge()
	inviteField := ""
	if currentInviteCode != "" {
		inviteField = fmt.Sprintf(`<input type="hidden" name="invite" value="%s">`, currentInviteCode)
	}
	// The button is a template slot, not a search-and-replace on the heading.
	//
	// It used to be injected by replacing the literal `<h1>Signup</h1>`, so
	// rewriting that heading — which happened when these pages were given the
	// landing's copy — silently deleted Sign up with Google from the page. No
	// error, no test, nothing in a diff to notice: the replace simply matched
	// nothing and returned the string unchanged. A slot cannot miss.
	return fmt.Sprintf(SignupTemplate, redirectParam,
		googleButtonHTML("Sign up with Google"), errHTML, app.CaptchaHTML(c), inviteField)
}

// renderRequestInvitePage shows the "request an invite" form that
// replaces the dead-end "invite only" page. Captcha-protected and
// rate-limited by IP so it can't be flooded.
func renderRequestInvitePage(w http.ResponseWriter, r *http.Request, message string) {
	c := app.NewCaptchaChallenge()
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
</div>`, msg, app.CaptchaHTML(c))
	w.Write([]byte(app.RenderHTML("Request an Invite", "Request an invite to Mu", body)))
}

// InviteHandler lets any logged-in user invite someone by email.
func InviteHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		email := strings.TrimSpace(r.FormValue("email"))
		if email == "" {
			app.BadRequest(w, r, "Email is required")
			return
		}
		code, err := auth.CreateInvite(email, acc.ID)
		if err != nil {
			app.ServerError(w, r, "Failed to create invite: "+err.Error())
			return
		}
		link := app.PublicURL() + "/signup?invite=" + code
		if app.EmailSender != nil {
			plain := fmt.Sprintf("%s invited you to join Mu.\n\nSign up here: %s", acc.Name, link)
			html := fmt.Sprintf(`<p>%s invited you to join Mu.</p><p><a href="%s">Sign up here</a></p>`, htmlpkg.EscapeString(acc.Name), link)
			app.EmailSender(email, acc.Name+" invited you to Mu", plain, html)
		}
		body := fmt.Sprintf(`<div class="card">
<h4>Invite sent</h4>
<p>Invite sent to <strong>%s</strong></p>
<p><a href="/invite">Invite another</a> · <a href="/home">Home</a></p>
</div>`, htmlpkg.EscapeString(email))
		w.Write([]byte(app.RenderHTML("Invite Sent", "Invite sent", body)))
		return
	}

	body := `<p><a href="/home">← Home</a></p>
<div class="card">
<h4>Invite someone to Mu</h4>
<p class="text-sm">Enter their email — they'll get a signup link.</p>
<form method="POST" action="/invite" style="margin-top:8px">
	<input type="email" name="email" placeholder="friend@example.com" required class="form-input" style="width:100%">
	<button type="submit" class="mt-2">Send invite</button>
</form>
</div>`
	w.Write([]byte(app.RenderHTML("Invite", "Invite someone to Mu", body)))
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

	if err := app.VerifyCaptchaRequest(r); err != nil {
		renderRequestInvitePage(w, r, fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()))
		return
	}

	// Per-IP rate limit reuses the signup bucket — same spam concern.
	ip := app.ClientIP(r)
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
	app.Log("auth", "Invite request from %s (%s)", email, ip)

	body := fmt.Sprintf(`<div class="card" style="max-width:440px;margin:0 auto">
<h3>Thanks — we got your request</h3>
<p>We'll email <strong>%s</strong> if we have a seat for you.</p>
<p class="mt-3"><a href="/">← Back</a></p>
</div>`, htmlpkg.EscapeString(email))
	w.Write([]byte(app.RenderHTML("Request Received", "Invite request received", body)))
}

// Login handler
func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Preserve redirect parameter in form action
		redirectParam := ""
		if redirect := r.URL.Query().Get("redirect"); redirect != "" {
			redirectParam = "?redirect=" + url.QueryEscape(redirect)
		}
		w.Write([]byte(loginPage(redirectParam, "")))
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
			w.Write([]byte(loginPage(redirectParam, `<p class="text-error">Username is required</p>`)))
			return
		}
		if len(secret) == 0 {
			w.Write([]byte(loginPage(redirectParam, `<p class="text-error">Password is required</p>`)))
			return
		}

		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(loginPage(redirectParam, `<p class="text-error">Invalid username or password</p>`)))
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

		http.Redirect(w, r, safeRedirect(r), 302)
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

	// Carried through every render so the POST keeps it — see renderSignupTo.
	redirectParam := ""
	if to := safeRedirect(r); to != "/home" {
		redirectParam = "?redirect=" + url.QueryEscape(to)
	}

	// Invite codes are optional — if one is provided (referral link),
	// it's consumed after signup for tracking. Signup works without one.
	// When INVITE_ONLY=true, a valid code IS required.
	if auth.InviteOnly() && invCode == "" {
		renderRequestInvitePage(w, r, "")
		return
	}
	if auth.InviteOnly() && invCode != "" {
		if err := auth.ValidateInvite(invCode); err != nil {
			w.Write([]byte(renderSignupTo(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
			return
		}
	}

	if r.Method == "GET" {
		w.Write([]byte(renderSignupTo("", redirectParam)))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		// Captcha is checked before the IP rate limit so that a failed
		// captcha doesn't burn an attempt against the IP bucket.
		if err := app.VerifyCaptchaRequest(r); err != nil {
			w.Write([]byte(renderSignupTo(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
			return
		}

		// Per-IP signup rate limit (defends against bulk account creation).
		ip := app.ClientIP(r)
		if !SignupRateLimit(ip) {
			app.Log("auth", "Signup rate limit hit for IP: %s", ip)
			w.Write([]byte(renderSignupTo(`<p class="text-error">Too many sign-ups from your network. Please try again later.</p>`, redirectParam)))
			return
		}

		id := r.Form.Get("id")
		name := r.Form.Get("name")
		secret := r.Form.Get("secret")

		const usernamePattern = "^[a-z][a-z0-9_]{3,23}$"

		usernameRegex := regexp.MustCompile(usernamePattern)

		if len(id) == 0 {
			w.Write([]byte(renderSignupTo(`<p class="text-error">Username is required</p>`, redirectParam)))
			return
		}

		if !usernameRegex.MatchString(id) {
			w.Write([]byte(renderSignupTo(`<p class="text-error">Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores</p>`, redirectParam)))
			return
		}

		if reason := auth.ValidateUsername(id); reason != "" {
			w.Write([]byte(renderSignupTo(fmt.Sprintf(`<p class="text-error">%s</p>`, reason), redirectParam)))
			return
		}

		if len(secret) == 0 {
			w.Write([]byte(renderSignupTo(`<p class="text-error">Password is required</p>`, redirectParam)))
			return
		}

		if len(secret) < 6 {
			w.Write([]byte(renderSignupTo(`<p class="text-error">Password must be at least 6 characters</p>`, redirectParam)))
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
			w.Write([]byte(renderSignupTo(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
			return
		}

		// Consume invite code if present (marks it as used).
		if invCode != "" {
			auth.ConsumeInvite(invCode, id)
		}

		// login
		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(renderSignupTo(`<p class="text-error">Account created but login failed. Please try logging in.</p>`, redirectParam)))
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

		// Back to wherever they were sent from, which for someone arriving
		// through a client's OAuth flow is /oauth/authorize — they now have a
		// session, so it issues the code without asking anything twice.
		http.Redirect(w, r, safeRedirect(r), 302)
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
			if _, ok := app.SupportedLanguages[newLang]; ok {
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

		// App widget preferences — which apps are pinned to the top of home.
		// Posted one app at a time from /apps, where the pin sits next to the
		// app it pins; `pin` and `unpin` name the app rather than restating the
		// whole set, so two tabs cannot silently undo each other.
		if slug := r.Form.Get("pin"); slug != "" {
			acc.Widgets = addWidget(acc.Widgets, slug)
			auth.UpdateAccount(acc)
			http.Redirect(w, r, app.ReturnTo(r, "/account"), http.StatusSeeOther)
			return
		}
		if slug := r.Form.Get("unpin"); slug != "" {
			acc.Widgets = removeWidget(acc.Widgets, slug)
			auth.UpdateAccount(acc)
			http.Redirect(w, r, app.ReturnTo(r, "/account"), http.StatusSeeOther)
			return
		}
		if r.Form.Get("save_widgets") != "" {
			acc.Widgets = r.Form["widgets"]
			auth.UpdateAccount(acc)
			http.Redirect(w, r, app.ReturnTo(r, "/account"), http.StatusSeeOther)
			return
		}

		// The display name, which had no way to be changed.
		//
		// It is set once at signup — optionally — and then shown on the profile,
		// in mail, in invites and on every post, with no form anywhere that
		// edits it. Somebody who skipped it, or typed it wrong, or changed their
		// name, had to ask an admin.
		//
		// Not the username: that is the id, it is in addresses and URLs that
		// other people hold, and renaming it is a different operation with
		// different consequences.
		if r.Form.Get("display_name") != "" || r.Form.Get("save_name") != "" {
			name := strings.TrimSpace(r.Form.Get("display_name"))
			if len(name) > 60 {
				name = strings.TrimSpace(name[:60])
			}
			// Empty is allowed and means "no display name" — the id stands in,
			// which is what an account that never set one already does.
			if name == "" {
				name = acc.ID
			}
			acc.Name = name
			auth.UpdateAccount(acc) //nolint:errcheck
			http.Redirect(w, r, "/account?saved=name", http.StatusSeeOther)
			return
		}

		// Giving up an address that was proved by code.
		//
		// The sign-in address is not removable here — it is what a password
		// reset goes to, and it is changed by verifying a new one, which is a
		// different operation. These are the extras.
		if addr := strings.TrimSpace(r.Form.Get("forget_address")); addr != "" {
			if err := auth.RemoveVerifiedAddress(acc.ID, addr); err != nil {
				http.Redirect(w, r, "/account?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/account?saved=address", http.StatusSeeOther)
			return
		}

		// Chat channel link code generation
		if r.Form.Get("channel_link") != "" {
			if app.LinkCodeFunc != nil {
				code := app.LinkCodeFunc(acc.ID)
				http.Redirect(w, r, "/account?link_code="+code, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/account", http.StatusSeeOther)
			}
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
	for code, name := range app.SupportedLanguages {
		selected := ""
		if code == currentLang {
			selected = " selected"
		}
		languageOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, code, selected, name)
	}

	// Email verification card + Google connect card
	emailCard := renderEmailCard(acc)
	googleCard := renderGoogleCard(acc)
	if r.URL.Query().Get("linked") == "google" {
		googleCard = `<div class="card" style="border-color:#1a7f37"><p style="margin:0;color:#1a7f37">✓ Google connected. You can now sign in with Google.</p></div>` + googleCard
	}
	// What this account has handed over, in one place. The asks live on the
	// pages that earn them; the audit belongs where somebody goes to check.
	googleCard += renderConnectionsCard(r, acc, r.URL.Query().Get("connection"))

	// The clients that reach the agent from somewhere else — Discord, Telegram,
	// WhatsApp, which is what client/ holds. One code works on any of them.
	//
	// This card was headed "Chat", which is a different thing on this instance:
	// chat is the service behind /chat, the live discussion rooms attached to an
	// item. Two unrelated things under one word, and the one on /account was not
	// the one with a page.
	clientsCard := ""
	if app.LinkCodeFunc != nil {
		code := r.URL.Query().Get("link_code")
		if code != "" {
			clientsCard = fmt.Sprintf(`<div class="card">
<h4>Clients</h4>
<p>Your link code: <code style="font-size:18px;font-weight:bold;background:#f0f0f0;padding:4px 12px;border-radius:4px">%s</code></p>
<p class="text-sm text-muted" style="margin-top:4px">Send <code>link %s</code> to the Mu bot on Discord, Telegram or WhatsApp. Expires in 5 minutes, and works once.</p>
</div>`, code, code)
		} else {
			clientsCard = `<div class="card">
<h4>Clients</h4>
<p class="text-sm text-muted">Use the agent from Discord, Telegram or WhatsApp. Generate a code, then send <code>link &lt;code&gt;</code> to the bot. Never send your password to a chat app.</p>
<form action="/account" method="POST" style="margin-top:8px">
<input type="hidden" name="channel_link" value="1">
<button type="submit">Generate Link Code</button>
</form>
</div>`
		}
	}

	notice := ""
	switch r.URL.Query().Get("saved") {
	case "name":
		notice = `<div class="card" style="background:#f0fff0;border-color:#a3d9a5"><p style="color:#27ae60;margin:0">Name saved.</p></div>`
	case "address":
		notice = `<div class="card" style="background:#f0fff0;border-color:#a3d9a5"><p style="color:#27ae60;margin:0">Address removed.</p></div>`
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		notice = `<div class="card" style="border-color:#e0a3a3"><p style="color:#c00;margin:0">` +
			htmlpkg.EscapeString(msg) + `</p></div>`
	}

	// The balance goes directly under the profile, and everything else after it.
	//
	// It is the one thing on this page with a deadline: a display name, a
	// language or a passkey can wait, and an empty balance stops the agent
	// mid-errand. It was a nav item of its own called Wallet, which put a
	// person's money one click further away than their choice of language.
	content := notice + fmt.Sprintf(`<div class="card">
<h4>Profile</h4>
<p><strong><a href="/@%s">%s</a></strong> · %s · Joined %s</p>
<form action="/account" method="POST" class="d-flex items-center gap-3" style="margin-top:8px">
	<input type="hidden" name="save_name" value="1">
	<input name="display_name" value="%s" maxlength="60" placeholder="Display name" style="max-width:280px">
	<button type="submit">Save</button>
</form>
<p class="text-sm text-muted" style="margin:6px 0 0">Shown on your posts and your profile. Your username, <strong>%s</strong>, is the one in addresses and links and does not change.</p>
</div>

%s

%s

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
<p><a href="/token">API Credentials →</a></p>
<p><a href="/user">User prefs →</a></p>
<p style="margin-top:12px"><a href="/logout" class="text-error">Logout</a></p>
</div>

%s`,
		acc.ID,
		acc.ID,
		acc.Name,
		acc.Created.Format("January 2, 2006"),
		htmlpkg.EscapeString(acc.Name),
		htmlpkg.EscapeString(acc.ID),
		BalanceCard(acc.ID),
		emailCard,
		googleCard,
		languageOptions,
		PasskeyListHTML(acc.ID),
		clientsCard,
		LedgerSection(acc.ID),
	)

	// About, Pricing, Help, Privacy, Status and the API reference live in the
	// footer, and there is no footer once you are signed in — deliberately, on
	// the argument that everything in it is in the sidebar or on /account. It
	// was not: Tools was, and the rest were nowhere, so a person with an account
	// could not reach the pricing page or the API docs from anywhere in the
	// product. Support had already been noticed and patched into the sidebar on
	// its own, one link at a time.
	//
	// Here rather than back under every screen, because the reasoning for
	// dropping the footer was sound — a marketing nav under an app screen is the
	// clearest tell that this is a website. Settings is where an app keeps
	// About and Legal, and /account is in the sidebar for everybody.
	content += `<div class="card"><span class="card-title">About Mu</span>` +
		`<p class="card-meta">` + app.FooterLinks() + `</p></div>`

	// app.RenderHTMLForRequest, not app.RenderHTML: the latter hard-codes a nil account,
	// so every part of the chrome that depends on knowing who is signed in went
	// missing on the one page you reach by being signed in.
	html := app.RenderHTMLForRequest("Account", "Account", content, r)
	w.Write([]byte(html))
}

// otherAddresses lists the addresses this account proved by code, with a way to
// give each one up.
//
// They were only visible on /email, which is the service for sending — an
// address you have proved is yours is account identity, and the page that says
// who you are is where somebody looks for it. Nothing lists the sign-in address
// here again; it is above, and it is not removable from this form.
func otherAddresses(acc *auth.Account) string {
	var extra []string
	for _, a := range acc.Verified() {
		if !strings.EqualFold(a, acc.Email) {
			extra = append(extra, a)
		}
	}
	if len(extra) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<p class="text-sm text-muted" style="margin:12px 0 4px">Also proved yours — mail from these reaches your agents:</p>`)
	b.WriteString(`<ul style="font-size:14px;margin:0;padding-left:18px">`)
	for _, a := range extra {
		b.WriteString(`<li style="margin:0 0 4px"><code>` + htmlpkg.EscapeString(a) + `</code>` +
			`<form action="/account" method="POST" style="display:inline">` +
			`<input type="hidden" name="forget_address" value="` + htmlpkg.EscapeString(a) + `">` +
			`<button type="submit" style="background:none;border:0;color:#c00;cursor:pointer;font-size:13px;padding:0 0 0 6px">remove</button>` +
			`</form></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
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

	if app.EmailSender == nil {
		return `<div class="card"><h4>Email</h4><p class="text-muted">Email verification is not configured on this instance.</p></div>`
	}

	if acc.EmailVerified {
		// Verified was a dead end: the address showed with a tick and there was
		// no way to change it, and no way to see or drop the others this account
		// had proved. An address is not yours for ever — people leave jobs and
		// close accounts — and the one a password reset goes to is exactly the
		// one somebody needs to be able to move.
		return fmt.Sprintf(`<div class="card">
<h4>Email</h4>
<p><strong>%s</strong> — verified ✓</p>
<p class="text-sm text-muted">Where a password reset goes. Verifying a different one replaces it.</p>
<form action="/account" method="POST" class="d-flex items-center gap-3" style="margin-top:8px">
	<input type="email" name="email" placeholder="you@example.com" required style="max-width:280px">
	<button type="submit">Verify a different address</button>
</form>
%s
</div>`, htmlpkg.EscapeString(acc.Email), otherAddresses(acc))
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

// Verify handles GET /verify?token=XXX — consumes a verification token
// and marks the account as verified.
func Verify(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		app.BadRequest(w, r, "Missing verification token")
		return
	}
	acc, err := auth.ConsumeEmailVerificationToken(token)
	if err != nil {
		app.BadRequest(w, r, err.Error())
		return
	}
	app.Log("auth", "Email verified for account %s (%s)", acc.ID, acc.Email)

	body := fmt.Sprintf(`<div class="card">
<h4>Email verified ✓</h4>
<p>Thanks, <strong>%s</strong>. Your email is verified and you can now post.</p>
<p><a href="/home" class="btn">Go home</a> &nbsp; <a href="/account">Account →</a></p>
</div>`, htmlpkg.EscapeString(acc.Name))
	html := app.RenderHTMLForRequest("Verified", "Email verified", body, r)
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

// Signup rate limiting per IP — defends against bulk account creation.
// Configurable via SIGNUP_MAX_PER_IP and SIGNUP_WINDOW_HOURS env vars.
var (
	signupMu       sync.Mutex
	signupAttempts = map[string]*signupBucket{}
)

type signupBucket struct {
	count   int
	resetAt time.Time
}

// safeRedirect is where to send someone after they sign in or sign up.
//
// Same-site only: a path starting with a single slash. Anything else — an
// absolute URL, or "//evil.example" which a browser reads as one — falls back
// to /home. An open redirect on a login page is a phishing primitive, and this
// one is reachable from a link an OAuth client hands to a user.
func safeRedirect(r *http.Request) string {
	to := r.URL.Query().Get("redirect")
	if to == "" || to[0] != '/' || strings.HasPrefix(to, "//") {
		return "/home"
	}
	return to
}

// addWidget and removeWidget edit the pinned-app list by name. Order is the
// order they were pinned in, which is the order they render.
func addWidget(have []string, slug string) []string {
	for _, w := range have {
		if w == slug {
			return have
		}
	}
	return append(have, slug)
}

func removeWidget(have []string, slug string) []string {
	out := make([]string, 0, len(have))
	for _, w := range have {
		if w != slug {
			out = append(out, w)
		}
	}
	return out
}

// handleVerifyStart processes the email submission on /account, generates
// a verification token, and sends an email containing the verify link.
func handleVerifyStart(w http.ResponseWriter, r *http.Request, acc *auth.Account, email string) {
	if app.EmailSender == nil {
		app.Forbidden(w, r, "Email verification is not configured on this instance.")
		return
	}
	if !app.ValidEmail(email) {
		app.BadRequest(w, r, "Please enter a valid email address.")
		return
	}

	// Persist the pending email so the UI can show it.
	if err := auth.SetAccountEmail(acc.ID, email); err != nil {
		app.ServerError(w, r, "Failed to save email")
		return
	}

	tok, err := auth.CreateEmailVerificationToken(acc.ID, email)
	if err != nil {
		app.ServerError(w, r, "Failed to create verification token")
		return
	}

	link := app.PublicURL() + "/verify?token=" + tok
	plain := fmt.Sprintf("Hi %s,\n\nClick the link below to verify your email and unlock posting on Mu:\n\n%s\n\nThis link expires in 24 hours. If you didn't request this, you can ignore this email.\n\n— Mu", acc.Name, link)
	html := fmt.Sprintf(`<p>Hi %s,</p><p>Click the link below to verify your email and unlock posting on Mu:</p><p><a href="%s">%s</a></p><p>This link expires in 24 hours. If you didn't request this, you can ignore this email.</p><p>— Mu</p>`, htmlpkg.EscapeString(acc.Name), link, link)

	if err := app.EmailSender(email, "Verify your Mu account", plain, html); err != nil {
		app.Log("auth", "Failed to send verification email to %s: %v", email, err)
		app.ServerError(w, r, "Failed to send verification email. Please try again.")
		return
	}
	app.Log("auth", "Sent verification email to %s for account %s", email, acc.ID)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
