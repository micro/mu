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
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"

	"mu/internal/auth"
	"mu/internal/push"
	"mu/service/sms"
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
	  %s
	  %s
	  <input id="id" name="id" placeholder="Username" required>
	  <input id="secret" name="secret" type="password" placeholder="Password" required>
	  <br>
	  <button>Login</button>
	</form>
	<div id="passkey-login" class="d-none text-center mt-5">
	  <p class="text-muted">or</p>
	  <button onclick="loginWithPasskey()">Login with Passkey</button>
	</div>
	<p class="text-center mt-5"><a href="/signup">Sign up</a> if you don't have an account</p>
	<script>
	if (window.PublicKeyCredential) {
	  PublicKeyCredential.isConditionalMediationAvailable && PublicKeyCredential.isConditionalMediationAvailable().then(function(){});
	  // classList, not style.display: .d-none is display:none !important and an
	  // inline style loses to it, which is why this button was never once seen.
	  document.getElementById('passkey-login').classList.remove('d-none');
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
	body := fmt.Sprintf(`<div class="card w-440 centered">
<h3>Request an invite</h3>
%s
<form method="POST" action="/request-invite" class="mt-3">
  <input type="email" name="email" placeholder="your@email.com" required class="w-full mb-2">
  <input type="text" name="reason" placeholder="Why you'd like to join (optional)" maxlength="500" class="w-full mb-2">
  %s
  <button type="submit">Request invite</button>
</form>
<p class="text-muted text-sm mt-3">Already have an invite? <a href="/login">Log in</a> or paste your link.</p>
</div>`, msg, app.CaptchaHTML(c))
	app.Respond(w, r, app.Response{Title: "Request an Invite", Description: "Request an invite to Mu", HTML: body})
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
			app.EmailSender(email, acc.Name+" invited you to Mu", plain, html, "")
		}
		body := fmt.Sprintf(`<div class="card">
<h4>Invite sent</h4>
<p>Invite sent to <strong>%s</strong></p>
<p><a href="/invite">Invite another</a> · <a href="/home">Home</a></p>
</div>`, htmlpkg.EscapeString(email))
		app.Respond(w, r, app.Response{Title: "Invite Sent", Description: "Invite sent", HTML: body})
		return
	}

	body := `<p><a href="/home">← Home</a></p>
<div class="card">
<h4>Invite someone to Mu</h4>
<p class="text-sm">Enter their email — they'll get a signup link.</p>
<form method="POST" action="/invite" class="mt-2">
	<input type="email" name="email" placeholder="friend@example.com" required class="form-input w-full">
	<button type="submit" class="mt-2">Send invite</button>
</form>
</div>`
	app.Respond(w, r, app.Response{Title: "Invite", Description: "Invite someone to Mu", HTML: body})
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

	body := fmt.Sprintf(`<div class="card w-440 centered">
<h3>Thanks — we got your request</h3>
<p>We'll email <strong>%s</strong> if we have a seat for you.</p>
<p class="mt-3"><a href="/">← Back</a></p>
</div>`, htmlpkg.EscapeString(email))
	app.Respond(w, r, app.Response{Title: "Request Received", Description: "Invite request received", HTML: body})
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

		// Claiming, where there is something to claim.
		//
		// Somebody who emailed agent@ already has an account: unclaimed, no
		// password, holding the conversation they had. The invite mailed to
		// them at the end of their free exchanges names their address, so
		// signing up with it takes over that account rather than making a
		// second one — which is the difference between "everything we have said
		// is saved" being true and being a line in an email.
		//
		// Creating instead would leave the conversation filed under an id
		// nobody can sign in to, and the person would arrive at an empty
		// account having been invited to keep a full one.
		claimed := false
		if invCode != "" {
			if existing := auth.UnclaimedFor(auth.InviteEmail(invCode)); existing != nil {
				if err := auth.Claim(existing.ID, id, secret); err != nil {
					w.Write([]byte(renderSignupTo(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
					return
				}
				claimed = true
			}
		}
		if !claimed {
			if err := auth.Create(&auth.Account{
				ID:        id,
				Secret:    secret,
				SecretSet: true,
				Name:      name,
				Created:   time.Now(),
			}); err != nil {
				w.Write([]byte(renderSignupTo(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
				return
			}
		}

		// Consume invite code if present (marks it as used).
		if invCode != "" {
			auth.ConsumeInvite(invCode, id)
		}

		// Something to spend, so the first question can be asked. See Welcome.
		Welcome(id)

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

		// Copies of arriving mail, on or off. The way out is also in every
		// forwarded message — see service/mail/unsubscribe.go — because somebody
		// who wants them to stop should not have to find this page. This is the
		// way back on.
		if state := strings.TrimSpace(r.Form.Get("forwarding")); state != "" {
			SetMailForwarding(acc.ID, state == "on")
			http.Redirect(w, r, "/account", http.StatusSeeOther)
			return
		}

		// Email verification request
		if email := strings.TrimSpace(r.Form.Get("email")); email != "" {
			handleVerifyStart(w, r, acc, email)
			return
		}

		// And the same three for a phone number. Handled here rather than
		// posted at /sms so that the section behaves like every other one on
		// this page: submit, land back here, see the result.
		if number := strings.TrimSpace(r.Form.Get("verify_number")); number != "" {
			if err := sms.StartVerify(acc.ID, number); err != nil {
				app.Error(w, r, http.StatusBadRequest, err.Error())
				return
			}
			http.Redirect(w, r, "/account", http.StatusSeeOther)
			return
		}
		if number := strings.TrimSpace(r.Form.Get("confirm_number")); number != "" {
			if err := sms.Confirm(acc.ID, number, r.Form.Get("code")); err != nil {
				app.Error(w, r, http.StatusBadRequest, err.Error())
				return
			}
			http.Redirect(w, r, "/account", http.StatusSeeOther)
			return
		}
		if number := strings.TrimSpace(r.Form.Get("forget_number")); number != "" {
			sms.Forget(acc.ID, number)
			http.Redirect(w, r, "/account", http.StatusSeeOther)
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

		// Setting a password, which nothing could do before — see
		// internal/auth/password.go for why that mattered more than it sounds.
		//
		// No current password asked for. The session is the authority, and it has
		// to be: the accounts that most need this are the ones whose password is
		// a random string they were never shown.
		if pw := r.Form.Get("new_secret"); pw != "" || r.Form.Get("save_secret") != "" {
			if pw != r.Form.Get("confirm_secret") {
				http.Redirect(w, r, "/account?error="+url.QueryEscape("Those two passwords are not the same."), http.StatusSeeOther)
				return
			}
			if err := auth.SetSecret(acc.ID, pw); err != nil {
				http.Redirect(w, r, "/account?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/account?saved=password", http.StatusSeeOther)
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

		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}

	// The languages this instance speaks, as options rather than as markup.
	currentLang := acc.Language
	if currentLang == "" {
		currentLang = "en"
	}
	langs := make([]app.Option, 0, len(app.SupportedLanguages))
	for code, name := range app.SupportedLanguages {
		langs = append(langs, app.Option{Value: code, Label: name, On: code == currentLang})
	}
	sort.Slice(langs, func(i, j int) bool { return langs[i].Label < langs[j].Label })

	emailCard := renderEmailCard(acc)

	// One card for Google: signing in with it, and what of it this account has
	// handed over. The asks live on the pages that earn them — the calendar on
	// /events, contacts on /contacts — and the audit belongs where somebody goes
	// to check. It was two cards, "Google" and "Connected accounts", stacked,
	// and the first one's name claimed the subject of the second.
	googleCard := renderGoogleCard(r, acc, r.URL.Query().Get("connection"))
	if r.URL.Query().Get("linked") == "google" {
		googleCard = app.Notice("Google connected. You can now sign in with Google.") + googleCard
	}

	// No Clients card. It offered a link code to send to a Mu bot on Discord,
	// Telegram or WhatsApp, and all three are gone — 2,100 lines and three
	// third-party APIs carrying no traffic. Mail is the client that matters and
	// needs no linking: the address is the account.

	notice := ""
	switch r.URL.Query().Get("saved") {
	case "name":
		notice = app.Notice("Name saved.")
	case "address":
		notice = app.Notice("Address removed.")
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		notice = app.Problem(msg)
	}

	// The balance goes directly under the profile, and everything else after it.
	//
	// It is the one thing on this page with a deadline: a display name, a
	// language or a passkey can wait, and an empty balance stops the agent
	// mid-errand. It was a nav item of its own called Wallet, which put a
	// person's money one click further away than their choice of language.
	// The page, as a list of sections.
	//
	// It was one fmt.Sprintf with fourteen %s in it and a template above them,
	// so reading it meant counting placeholders down one list and arguments up
	// another to find out which card was which. Every block below says its own
	// name, and the order on the screen is the order in the code.
	//
	// The balance goes directly under the profile because it is the one thing
	// here with a deadline: a display name, a language or a passkey can wait,
	// and an empty balance stops the agent mid-errand.
	profile := app.Section("Profile",
		`<p><strong><a href="/@`+htmlpkg.EscapeString(acc.ID)+`">`+
			htmlpkg.EscapeString(acc.ID)+`</a></strong> · `+htmlpkg.EscapeString(acc.Name)+
			` · Joined `+acc.Created.Format("January 2, 2006")+`</p>`,
		app.Form{Action: "/account", Inline: true,
			Hidden: map[string]string{"save_name": "1"},
			Fields: []app.Field{{Name: "display_name", Value: acc.Name, Max: 60,
				Placeholder: "Display name"}},
			Submit: "Save"}.HTML(),
		app.Note("Shown on your posts and your profile. Your username, @"+acc.ID+
			", is the one in addresses and links and does not change."))

	language := app.Section("Language",
		app.Form{Action: "/account", Inline: true,
			Fields: []app.Field{{Name: "language", Options: langs}},
			Submit: "Save"}.HTML())

	// No Settings section, and no About card.
	//
	// "Settings" was a section named after the page it was on, which is a name
	// that can absorb anything — and it had: a link to your tokens, a link to
	// the three piles you accumulate by using the product, and Log out. Nothing
	// they share, and not one of them a setting. "About Mu" was the footer in a
	// card, titled for one of its four links while holding Privacy and Status,
	// and one of the four (Tools) is a sidebar item already.
	//
	// Both are the same tell: a card whose name means "miscellaneous" is where
	// things go when nobody decided where they belong. They are destinations,
	// and destinations belong in the menu with your name on it — see
	// app.navBottom. Log out was already there.

	// The money is not here any more. Balance, the usage graph and the ledger
	// were the three cards under the profile, on the reasoning — written above,
	// twice — that a balance has a deadline and a language picker does not. That
	// reasoning was right and the conclusion was wrong: the answer to "this
	// matters more than the rest of the page" is its own page, not the top of
	// this one. /wallet is that page and Wallet is in the menu beside Account.
	//
	// Not even a line pointing at it. There was one for a few minutes, on the
	// reasoning that somebody would look for money here first — and Wallet is
	// in the same menu this page is reached from, one item below it. A section
	// whose only content is a link to its neighbour is the thing being removed
	// everywhere else on this page.
	//
	// Notifications last, because it is a thing you do rather than a thing you
	// read, and on a phone it is what makes the product work with the page
	// closed. It used to render below the Settings section — which ended with
	// Log out, so the control sat under the link that ends the session, where a
	// page has plainly finished.
	content := notice + profile +
		mailClientCard() +
		passwordCard(acc) +
		PlaceCard(r, acc.ID) +
		emailCard +
		renderPhoneCard(acc.ID) +
		googleCard +
		language +
		PasskeyListHTML(acc.ID) +
		push.Card(r, acc.ID)

	// About, Privacy, Status — a line, not a card.
	//
	// These have to be reachable and are worth very little. They were a card
	// headed "About Mu", which titled the group for one of its four links while
	// two of the others were Privacy and Status, and gave a marketing nav the
	// same weight as the balance. About is in the account menu now; this is the
	// rest of it at the weight it deserves, at the foot of the page where a
	// footer would be if there were one.
	//
	// It is here rather than nowhere because the footer is not rendered for a
	// signed-in account — see footerFor — so with no line at all /privacy and
	// /status become unreachable from inside the product. TestEveryFooterLink-
	// IsReachableSignedIn caught exactly that when About left for the menu.
	content += `<p class="account-legal">` + app.FooterLinks() + `</p>`

	// app.RenderHTMLForRequest, not app.RenderHTML: the latter hard-codes a nil account,
	// so every part of the chrome that depends on knowing who is signed in went
	// missing on the one page you reach by being signed in.
	app.Respond(w, r, app.Response{Title: "Account", Description: "Account", HTML: content})
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
	b.WriteString(app.Note("Also proved yours — mail from these reaches your agents:"))
	b.WriteString(`<ul class="addr-list">`)
	for _, a := range extra {
		b.WriteString(`<li><code>` + htmlpkg.EscapeString(a) + `</code>` +
			app.Form{Action: "/account", Class: "addr-drop",
				Hidden: map[string]string{"forget_address": a},
				Extra:  []app.Button{{Label: "remove", Kind: app.Danger}}}.HTML() +
			`</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// renderPhoneCard is the number you have proved is yours.
//
// Beside Email because it is the same kind of fact — an address you claimed —
// and because of what it now decides. A text from a verified number wakes your
// agent; a text from any other number is filed and answered by nobody. That is
// an account-level consequence, so the claiming belongs on the account page
// rather than folded inside a <details> on /sms, which is where it lived and
// where nobody looking for it would think to open.
//
// Absent entirely on an instance with no number to text from. A form that can
// only fail reads as broken rather than as unconfigured — the same call the
// wallet's convert form makes.
// agentNumber is the number the agent texts from, for saving as a contact.
//
// Drawn only when the instance has one. A line saying where texts come from,
// on an instance that cannot send any, is furniture that teaches the page is
// broken.
func agentNumber() string {
	from := strings.TrimSpace(sms.From())
	if from == "" {
		return ""
	}
	return app.NoteHTML("It texts you from <code>" + htmlpkg.EscapeString(from) +
		"</code>. Save that as " + htmlpkg.EscapeString(auth.MicroName) +
		" and you can write to it from your phone.")
}

func renderPhoneCard(accountID string) string {
	if !sms.Configured() {
		return ""
	}

	mine := sms.Numbers(accountID)
	if len(mine) > 0 {
		var b strings.Builder
		for _, n := range mine {
			b.WriteString(`<p><strong>` + htmlpkg.EscapeString(n) + `</strong> — verified ✓ ` +
				app.Form{Action: "/account", Inline: true,
					Hidden: map[string]string{"forget_number": n},
					Submit: "Forget"}.HTML() + `</p>`)
		}
		return app.Section("Phone",
			b.String(),
			app.Note("A text from here reaches your agent, and it answers. "+
				"Texts from anywhere else are filed and answered by nobody."),
			// The other direction, which this card never mentioned.
			//
			// Every section on this page asks the same question — prove this is
			// yours, so we will listen to it — and none of them says how the
			// agent reaches you or what number it comes from. So the one thing
			// somebody needs in order to save the contact was on the page
			// nowhere, and the number arrived unannounced in a text about a
			// code. See #1485.
			agentNumber(),
			app.Form{Action: "/account", Inline: true,
				Fields: []app.Field{{Name: "verify_number", Type: "tel", Required: true,
					Placeholder: "+447700900123"}},
				Submit: "Verify another"}.HTML())
	}

	// Waiting for the code it just texted.
	if pending, ok := sms.Pending(accountID); ok {
		return app.Section("Phone",
			`<p>A code was texted to <strong>`+htmlpkg.EscapeString(pending)+`</strong>.</p>`,
			app.Form{Action: "/account", Inline: true,
				Hidden: map[string]string{"confirm_number": pending},
				Fields: []app.Field{{Name: "code", Required: true, Placeholder: "123456"}},
				Submit: "Confirm"}.HTML())
	}

	return app.Section("Phone",
		app.Note("Prove a number is yours and you can text your agent from it, "+
			"like any other contact. It replies on the same number."),
		app.Form{Action: "/account", Inline: true,
			Fields: []app.Field{{Name: "verify_number", Type: "tel", Required: true,
				Placeholder: "+447700900123"}},
			Submit: "Send me a code"}.HTML())
}

// forwardingToggle is whether mail arriving here is copied to that address.
//
// On this card rather than one of its own, because it is a fact about the
// verified address: it is the thing that address is used for besides a password
// reset, and a section elsewhere asking about "forwarding" would be a setting
// with no visible subject.
//
// The way out is also in every forwarded message — see
// service/mail/unsubscribe.go — because somebody who wants these to stop should
// not have to find this page, or sign in, to say so. This is the way back on.
func forwardingToggle(acc *auth.Account) string {
	on := MailForwardingOn(acc.ID)
	state, submit := "off", "Turn off"
	note := "Mail sent to your Mu address is also copied to you here."
	if !on {
		state, submit = "on", "Turn on"
		note = "Mail sent to your Mu address is not copied to you here."
	}
	// Posted to /account with a named field, the same as every other control on
	// this page — submit, land back here, see the result.
	return app.Note(note) +
		`<form method="POST" action="/account" class="d-inline">` +
		`<input type="hidden" name="forwarding" value="` + state + `">` +
		`<button type="submit" class="btn-link">` + submit + `</button></form>`
}

// renderEmailCard renders the email verification card on the account
// page. The card looks different depending on whether the email is set,
// pending, or verified — and whether email sending is configured at all.
func renderEmailCard(acc *auth.Account) string {
	if acc.Admin || acc.Approved {
		// Admins/approved users don't need verification.
		if acc.EmailVerified {
			// The toggle here too. An admin gets forwarded mail like anybody
			// else and had no way to turn it off from this page — only the link
			// at the bottom of a message, which is the way out for somebody who
			// does not want to come here and the wrong only way for somebody
			// who is already on the page.
			return app.Section("Email",
				`<p>`+htmlpkg.EscapeString(acc.Email)+` — verified</p>`,
				forwardingToggle(acc))
		}
		return ""
	}

	if app.EmailSender == nil {
		return app.Section("Email",
			app.Note("Email verification is not configured on this instance."))
	}

	if acc.EmailVerified {
		// Verified was a dead end: the address showed with a tick and there was
		// no way to change it, and no way to see or drop the others this account
		// had proved. An address is not yours for ever — people leave jobs and
		// close accounts — and the one a password reset goes to is exactly the
		// one somebody needs to be able to move.
		return app.Section("Email",
			`<p><strong>`+htmlpkg.EscapeString(acc.Email)+`</strong> — verified ✓</p>`,
			app.Note("Where a password reset goes. Verifying a different one replaces it."),
			forwardingToggle(acc),
			app.Form{Action: "/account", Inline: true,
				Fields: []app.Field{{Name: "email", Type: "email", Required: true,
					Placeholder: "you@example.com"}},
				Submit: "Verify a different address"}.HTML(),
			otherAddresses(acc))
	}

	pending := ""
	if acc.Email != "" {
		pending = app.NoteHTML(`A verification link was sent to <strong>` +
			htmlpkg.EscapeString(acc.Email) + `</strong>. Click it to unlock posting. ` +
			`Submit again to resend.`)
	}

	return app.Section("Verify your email to post",
		`<p>Verifying your email unlocks status updates, replies, comments and blog posts. `+
			`We do not share or sell your address.</p>`,
		pending,
		app.Form{Action: "/account", Inline: true,
			Fields: []app.Field{{Name: "email", Type: "email", Value: acc.Email,
				Required: true, Placeholder: "you@example.com"}},
			Submit: "Send verification"}.HTML())
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
	app.Respond(w, r, app.Response{Title: "Verified", Description: "Email verified", HTML: body})
}

// passwordCard is where a password gets set, which was nowhere.
//
// Two headings for one form, because the two situations are genuinely
// different. An account made through Google has a password — a random 24
// characters it was never told — so the export page asked for it, refused
// whatever was typed, and suggested setting one, which could not be done. That
// account needs to be told it has none. An account that chose one needs to know
// this replaces it.
// A third state, because there are three and there were two.
//
// HasSecret is false for two different accounts: one made through Google, which
// genuinely has no password its owner could type, and one made before SecretSet
// existed, which may well have a chosen password. Nothing distinguishes them —
// no field records how an account was created, which is the reason the flag was
// added — so the note for that case has to describe what the form does without
// asserting which of the two the reader is.
//
// It asserted. "This account signs in with Google or a passkey and has no
// password you could type" is a definite claim about the reader's account, and
// it is wrong for every account that predates the flag, told to somebody who
// knows perfectly well they typed a password at signup. Being wrong about
// somebody's own credentials is a good way to make them doubt the rest of the
// page.
// mailClientCard is the way to /inbox/imap.
//
// It lived on the inbox, beside New, as a text link. That is the screen an
// account owner opens every day and this is a thing they do once in the life of
// the account, if ever — so it was read past several thousand times to be used
// never again, and it made the two controls at the top of the inbox look like
// the same size of decision.
//
// Here instead, with the password, the passkeys and the phone: the things you
// set up once and then forget. It has to be somewhere — /inbox/imap is served,
// and a served page with nothing linking to it is a page nobody can find, which
// TestTheMailClientPageIsReachable exists to prevent.
func mailClientCard() string {
	return app.Section("Mail client",
		`<p>Read and send this account's mail from Apple Mail, Thunderbird, `+
			`Gmail on Android — anything that speaks IMAP.</p>`+
			app.Link("Server settings and addresses", "/inbox/imap"))
}

func passwordCard(acc *auth.Account) string {
	note := "Signing up with Google or a passkey leaves no password you could type. " +
		"Setting one here lets you sign in with your username, and unlocks exporting " +
		"your wallet key. If you already have a password, this replaces it."
	if auth.HasSecret(acc.ID) {
		note = "Replaces the one you have. You stay signed in here; other devices are unaffected."
	}

	// "Password", not "Set a password" or "Change password". Both of those are
	// claims about whether the reader already has one, and the flag they rest on
	// is only reliable for accounts created after it existed. The other headings
	// on this page name the thing they are about — Email, Location — and this one
	// can too. The note below still says which of the two this will do.
	return app.Section("Password",
		app.Form{Action: "/account",
			Hidden: map[string]string{"save_secret": "1"},
			Fields: []app.Field{
				{Name: "new_secret", Type: "password", Label: "New password", Wide: true,
					Placeholder: "At least 6 characters"},
				{Name: "confirm_secret", Type: "password", Label: "Again", Wide: true,
					Placeholder: "The same one"},
			},
			Submit: "Save"}.HTML(),
		app.Note(note))
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
// safeRedirect is where signing in lands you.
//
// Where you were going, when you were going somewhere — signing in should
// finish the thing you were doing. Otherwise the front door.
//
// It was the dashboard, and that was right while the front door was a pitch a
// signed-in account had no use for. It is the same page for everybody now, and
// landing on /home instead put a rail and a grid of sixteen cards in front of
// somebody whose next move is to type a question. The dashboard is a place to
// go and look at things, reached from the corner when that is what you came
// for. See home.Index.
func safeRedirect(r *http.Request) string {
	to := r.URL.Query().Get("redirect")
	if to == "" || to[0] != '/' || strings.HasPrefix(to, "//") {
		return "/"
	}
	return to
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

	if err := app.EmailSender(email, "Verify your Mu account", plain, html, ""); err != nil {
		app.Log("auth", "Failed to send verification email to %s: %v", email, err)
		app.ServerError(w, r, "Failed to send verification email. Please try again.")
		return
	}
	app.Log("auth", "Sent verification email to %s for account %s", email, acc.ID)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
