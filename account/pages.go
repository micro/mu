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
	"mu/internal/push"
	"mu/internal/quota"
	"mu/internal/usage"
	"sort"

	"mu/internal/auth"
	"mu/internal/client"
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

var LoginTemplate = `
	<p id="auth-status" role="status"></p><form id="login" action="/login%s" method="POST" class="form page-stack">
	  <h1 class="text-center">Log in</h1>
	  %s
	  %s
	  <label class="field-label">Username<input id="id" name="id" autocomplete="username" required></label>
	  <label class="field-label">Password<input id="secret" name="secret" type="password" autocomplete="current-password" required></label>

	  <button>Log in</button>
	</form>
	<details class="disclosure auth-help"><summary>Having trouble signing in?</summary><p>If you joined with Google, use Continue with Google. If you still have a signed-in device, you can set a password in Account. Otherwise, <a href="/contact">contact the server operator</a> for help. Automatic password reset is not available.</p></details>
	<div id="passkey-login" class="d-none text-center mt-5">
	  <p class="text-muted">or</p>
	  <button onclick="loginWithPasskey()">Login with Passkey</button>
	</div>
	<p class="text-center mt-5"><a href="/signup">Sign up</a> if you don't have an account</p>

`

var SignupTemplate = `
	<form id="signup" action="/signup%s" method="POST" class="form page-stack">
	  <h1 class="text-center">Create your account</h1>
	  %s
	  %s
	  <label class="field-label">Username<input id="id" name="id" autocomplete="username" minlength="4" maxlength="24" pattern="[a-z][a-z0-9_]{3,23}" aria-describedby="username-help" required></label><small id="username-help" class="text-muted">4–24 characters. Start with a letter; use lowercase letters, numbers or underscores.</small>
	  <label class="field-label">Password<input id="secret" name="secret" type="password" autocomplete="new-password" minlength="6" aria-describedby="password-help" required></label><small id="password-help" class="text-muted">At least 6 characters.</small>
	  %s
	  %s

	  <button>Create account</button>
	</form>
	<p class="text-center mt-5"><a href="/login">Log in</a> if you have an account</p>
`

// renderSignup renders the signup template with a fresh captcha challenge
// and the given error HTML (or empty string).
func renderSignup(errHTML string) string { return renderSignupTo(errHTML, "") }

// renderSignupTo threads a redirect through the form action. Without it the
// POST goes to a bare /signup and the destination is lost between showing the
// form and submitting it — which is how someone sent here by a client's OAuth
// flow would create an account and then land on /home, with the client still
// waiting.
func renderSignupTo(errHTML, redirectParam string) string {
	return renderSignupInvite(errHTML, redirectParam, "")
}

func renderSignupInvite(errHTML, redirectParam, invite string) string {
	c := app.NewCaptchaChallenge()
	inviteField := ""
	if invite != "" {
		inviteField = fmt.Sprintf(`<input type="hidden" name="invite" value="%s">`, htmlpkg.EscapeString(invite))
	}
	// The button is a template slot, not a search-and-replace on the heading.
	//
	// It used to be injected by replacing the literal `<h1>Signup</h1>`, so
	// rewriting that heading — which happened when these pages were given the
	// landing's copy — silently deleted Sign up with Google from the page. No
	// error, no test, nothing in a diff to notice: the replace simply matched
	// nothing and returned the string unchanged. A slot cannot miss.
	return app.ConsoleHTML("Sign up", fmt.Sprintf(SignupTemplate, redirectParam,
		googleButtonHTML("Sign up with Google"), errHTML, app.CaptchaHTML(c), inviteField), nil)
}

// renderRequestInvitePage shows the "request an invite" form that
// replaces the dead-end "invite only" page. Captcha-protected and
// rate-limited by IP so it can't be flooded.
func renderRequestInvitePage(w http.ResponseWriter, r *http.Request, message string) {
	c := app.NewCaptchaChallenge()
	msg := message
	if msg == "" {
		msg = `<p>Micro is currently invite-only. Leave your email and we'll send you an invite when we open up more seats.</p>`
	}
	body := fmt.Sprintf(`<div class="card w-440 centered">
<h3>Request an invite</h3>
%s
<form method="POST" action="/request-invite" class="form mt-3">
  <input type="email" name="email" placeholder="your@email.com" required class="w-full mb-2">
  <input type="text" name="reason" placeholder="Why you'd like to join (optional)" maxlength="500" class="w-full mb-2">
  %s
  <button type="submit">Request invite</button>
</form>
<p class="text-muted text-sm mt-3">Already have an invite? <a href="/login">Log in</a> or paste your link.</p>
</div>`, msg, app.CaptchaHTML(c))
	app.Respond(w, r, app.Response{Title: "Request an Invite", Description: "Request an invite to Micro", HTML: body})
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
			plain := fmt.Sprintf("%s invited you to join Micro.\n\nSign up here: %s", acc.Name, link)
			html := fmt.Sprintf(`<p>%s invited you to join Micro.</p><p><a href="%s">Sign up here</a></p>`, htmlpkg.EscapeString(acc.Name), link)
			app.EmailSender(email, acc.Name+" invited you to Micro", plain, html, "")
		}
		body := fmt.Sprintf(`<div class="card">
<h4>Invite sent</h4>
<p>Invite sent to <strong>%s</strong></p>
<p><a href="/invite">Invite another</a> · <a href="/">Home</a></p>
</div>`, htmlpkg.EscapeString(email))
		app.Respond(w, r, app.Response{Title: "Invite Sent", Description: "Invite sent", HTML: body})
		return
	}

	body := `<p><a href="/">Home</a></p>
<div class="card">
<h4>Invite someone to Micro</h4>
<p class="text-sm">Enter their email — they'll get a signup link.</p>
<form method="POST" action="/invite" class="form mt-2">
	<input type="email" name="email" placeholder="friend@example.com" required class="form-input w-full">
	<button type="submit" class="mt-2">Send invite</button>
</form>
</div>`
	app.Respond(w, r, app.Response{Title: "Invite", Description: "Invite someone to Micro", HTML: body})
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
<p class="mt-3"><a href="/">Back</a></p>
</div>`, htmlpkg.EscapeString(email))
	app.Respond(w, r, app.Response{Title: "Request Received", Description: "Invite request received", HTML: body})
}

// Login handler
func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, acc := auth.TrySession(r); acc != nil {
			http.Redirect(w, r, safeRedirect(r), http.StatusSeeOther)
			return
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	renderLogin := func(to, msg string) string { return accountFormValues(loginPage(to, msg), r) }
	if r.Method == "GET" {
		// Preserve redirect parameter in form action
		redirectParam := ""
		if redirect := r.URL.Query().Get("redirect"); redirect != "" {
			redirectParam = "?redirect=" + url.QueryEscape(redirect)
		}
		w.Write([]byte(renderLogin(redirectParam, "")))
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
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(renderLogin(redirectParam, `<p class="text-error">Username is required</p>`)))
			return
		}
		if len(secret) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(renderLogin(redirectParam, `<p class="text-error">Password is required</p>`)))
			return
		}

		sess, err := auth.Login(id, secret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(renderLogin(redirectParam, `<p class="text-error">Invalid username or password</p>`)))
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
	// Each form contains a short-lived captcha and optional referral state.
	w.Header().Set("Cache-Control", "private, no-store")
	// Thread the invite code through renders so the hidden field persists.
	invCode := r.URL.Query().Get("invite")
	if r.Method == "POST" {
		if v := r.FormValue("invite"); v != "" {
			invCode = v
		}
	}
	// Keep referral state on this request, never shared between visitors.
	render := func(errHTML, redirectParam string) string {
		return accountFormValues(renderSignupInvite(errHTML, redirectParam, invCode), r)
	}

	// Carried through every render so the POST keeps it — see renderSignupTo.
	redirectParam := ""
	if to := safeRedirect(r); to != "/" {
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
			w.Write([]byte(render(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
			return
		}
	}

	if r.Method == "GET" {
		w.Write([]byte(render("", redirectParam)))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		// Captcha is checked before the IP rate limit so that a failed
		// captcha doesn't burn an attempt against the IP bucket.
		if err := app.VerifyCaptchaRequest(r); err != nil {
			w.Write([]byte(render(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
			return
		}

		// Per-IP signup rate limit (defends against bulk account creation).
		ip := app.ClientIP(r)
		if !SignupRateLimit(ip) {
			app.Log("auth", "Signup rate limit hit for IP: %s", ip)
			w.Write([]byte(render(`<p class="text-error">Too many sign-ups from your network. Please try again later.</p>`, redirectParam)))
			return
		}

		id := r.Form.Get("id")
		name := id
		secret := r.Form.Get("secret")

		const usernamePattern = "^[a-z][a-z0-9_]{3,23}$"

		usernameRegex := regexp.MustCompile(usernamePattern)

		if len(id) == 0 {
			w.Write([]byte(render(`<p class="text-error">Username is required</p>`, redirectParam)))
			return
		}

		if !usernameRegex.MatchString(id) {
			w.Write([]byte(render(`<p class="text-error">Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores</p>`, redirectParam)))
			return
		}

		if reason := auth.ValidateUsername(id); reason != "" {
			w.Write([]byte(render(fmt.Sprintf(`<p class="text-error">%s</p>`, reason), redirectParam)))
			return
		}

		if len(secret) == 0 {
			w.Write([]byte(render(`<p class="text-error">Password is required</p>`, redirectParam)))
			return
		}

		if len(secret) < 6 {
			w.Write([]byte(render(`<p class="text-error">Password must be at least 6 characters</p>`, redirectParam)))
			return
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
					w.Write([]byte(render(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
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
				w.Write([]byte(render(fmt.Sprintf(`<p class="text-error">%s</p>`, err.Error()), redirectParam)))
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
			w.Write([]byte(render(`<p class="text-error">Account created but login failed. Please try logging in.</p>`, redirectParam)))
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

	// Real destinations share the same authenticated mutation and JSON handling.
	accountPath := "/account"
	switch r.URL.Path {
	case "/account/connections":
		accountPath = "/account/connections"
	case "/account/usage", "/account/billing":
		accountPath = "/account/usage"
	}
	if r.Method == http.MethodGet && !app.WantsJSON(r) {
		if r.URL.Query().Get("linked") == "google" || r.URL.Query().Get("connection") != "" {
			accountPath = "/account/connections"
		}
		if r.URL.Path != accountPath {
			target := accountPath
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}
	title := "Account"
	// Handle account settings and email verification.
	if r.Method == "POST" {
		r.ParseForm()

		// Copies of arriving mail, on or off. The way out is also in every
		// forwarded message — see service/mail/unsubscribe.go — because somebody
		// who wants them to stop should not have to find this page. This is the
		// way back on.
		if state := strings.TrimSpace(r.Form.Get("forwarding")); state != "" {
			SetMailForwarding(acc.ID, state == "on")
			http.Redirect(w, r, accountPath, http.StatusSeeOther)
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
			http.Redirect(w, r, accountPath, http.StatusSeeOther)
			return
		}
		if number := strings.TrimSpace(r.Form.Get("confirm_number")); number != "" {
			if err := sms.Confirm(acc.ID, number, r.Form.Get("code")); err != nil {
				app.Error(w, r, http.StatusBadRequest, err.Error())
				return
			}
			http.Redirect(w, r, accountPath, http.StatusSeeOther)
			return
		}
		if number := strings.TrimSpace(r.Form.Get("forget_number")); number != "" {
			sms.Forget(acc.ID, number)
			http.Redirect(w, r, accountPath, http.StatusSeeOther)
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
			http.Redirect(w, r, accountPath+"?saved=name", http.StatusSeeOther)
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
				http.Redirect(w, r, accountPath+"?error="+url.QueryEscape("Those two passwords are not the same."), http.StatusSeeOther)
				return
			}
			if err := auth.SetSecret(acc.ID, pw); err != nil {
				http.Redirect(w, r, accountPath+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, accountPath+"?saved=password", http.StatusSeeOther)
			return
		}

		// Giving up an address that was proved by code.
		//
		// The sign-in address is not removable here — it is what a password
		// reset goes to, and it is changed by verifying a new one, which is a
		// different operation. These are the extras.
		if addr := strings.TrimSpace(r.Form.Get("forget_address")); addr != "" {
			if err := auth.RemoveVerifiedAddress(acc.ID, addr); err != nil {
				http.Redirect(w, r, accountPath+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, accountPath+"?saved=address", http.StatusSeeOther)
			return
		}

		// Chat channel link code generation

		http.Redirect(w, r, accountPath, http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		clientAccount(w, r, acc)
		return
	}
	notice := ""
	switch r.URL.Query().Get("saved") {
	case "converted":
		notice = app.Notice("USDC converted to account credits.")
	case "name":
		notice = app.Notice("Name saved.")
	case "password":
		notice = app.Notice("Password saved.")
	case "address":
		notice = app.Notice("Address removed.")
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		notice = app.Problem(msg)
	}

	content := ""
	switch accountPath {
	case "/account/connections":
		googleCard := renderGoogleCard(r, acc, r.URL.Query().Get("connection"))
		if r.URL.Query().Get("linked") == "google" {
			notice = app.Notice("Google connected.") + notice
		}
		content = renderEmailCard(acc) + renderPhoneCard(acc.ID) + googleCard
		content += app.SectionID("notifications", "Notifications", forwardingToggle(acc), push.Card(r, acc.ID, "This device"))
		content += `<details class="disclosure"><summary>Advanced client setup</summary><p>Connect a mail or XMPP client using an access token.</p><div class="form-actions"><a href="/token">Access tokens</a><a href="/inbox/imap">Mail connection details</a></div>` + xmppConnectionDetails(acc) + `</details>`
	case "/account/usage":
		content = usage.Card(acc.ID) + LedgerSection(acc.ID) +
			app.Section("Billing", `<div class="form-actions"><a href="/account/topup">Add credit</a><a href="/account/transfer">Transfer credit</a></div>`+app.Note("1 credit = 1¢"))
	default:
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

		profile := app.SectionID("profile", "Name",
			`<p><strong><a href="/@`+htmlpkg.EscapeString(acc.ID)+`">`+
				htmlpkg.EscapeString(acc.ID)+`</a></strong> · `+htmlpkg.EscapeString(acc.Name)+
				` · Joined `+acc.Created.Format("January 2, 2006")+`</p>`,
			app.Form{Action: "/account", Inline: true,
				Hidden: map[string]string{"save_name": "1"},
				Fields: []app.Field{{Name: "display_name", Value: acc.Name, Max: 60,
					Placeholder: "Display name"}},
				Submit: "Save"}.HTML(),
			app.Note("Your display name. Your username, @"+acc.ID+
				", is the one in addresses and links and does not change."))

		language := app.Section("Language",
			app.Form{Action: "/account", Inline: true,
				Fields: []app.Field{{Name: "language", Options: langs}},
				Submit: "Save"}.HTML())

		content = profile + language + PlaceCard(r, acc.ID) + `<section class="settings-signin" aria-labelledby="signin-heading"><h2 id="signin-heading">Sign-in</h2>` + passwordCard(acc) + PasskeyListHTML(acc.ID) + `</section>`
	}
	// Forms return to their owning tab; credentials and mutations stay in POST.
	content = strings.ReplaceAll(content, `action="/account"`, `action="`+accountPath+`"`)
	var nav strings.Builder
	nav.WriteString(`<nav class="view-switch" aria-label="Account settings">`)
	for _, tab := range []struct{ path, label string }{{"/account", "Account"}, {"/account/connections", "Connections"}, {"/account/usage", "Usage & billing"}} {
		current := ""
		if accountPath == tab.path {
			current = ` aria-current="page"`
		}
		nav.WriteString(`<a href="` + tab.path + `"` + current + `>` + tab.label + `</a>`)
	}
	nav.WriteString(`</nav>`)
	balance := ""
	if PaymentsEnabled() && !acc.Admin && !acc.Agent {
		balance = `<div class="metadata-row">`
		if daily := quota.DailyCredits(); daily > 0 {
			balance += `<span>Today: <strong>` + thousands(IncludedToday(acc.ID)) + ` of ` + thousands(daily) + ` credits left</strong> · renews at 00:00 UTC</span>`
		}
		balance += `<span>Credit balance: <strong>` + thousands(CreditsOf(acc.ID).Balance) + ` credits</strong></span>`
		if TopUpConfigured() {
			balance += `<a href="/account/topup">Add credit</a>`
		}
		balance += `</div>`
	}

	content = balance + nav.String() + notice + `<div class="page-stack settings-sections">` + content + `</div>`

	// app.RenderHTMLForRequest, not app.RenderHTML: the latter hard-codes a nil account,
	// so every part of the chrome that depends on knowing who is signed in went
	// missing on the one page you reach by being signed in.
	app.Respond(w, r, app.Response{Title: title, Description: title, HTML: content})
}

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
<p>Thanks, <strong>%s</strong>. Your email is verified. Micro can now recognise email from you.</p>
<p><a href="/" class="btn">Go home</a> &nbsp; <a href="/account">Account</a></p>
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
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
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
// finish the thing you were doing. Otherwise Home: the public front door has
// become this person's place, with their assistant and context in it.
func safeRedirect(r *http.Request) string {
	return SafeRedirectTo(r.URL.Query().Get("redirect"))
}

// SafeRedirectTo is where to send somebody after signing in: back where they
// were, or the front door.
//
// Everything here is about one attack. This value arrives on a URL anybody can
// write, and it ends up in a Location header — so a link to our own login page
// can be made to land somebody on a site of the attacker's choosing, wearing
// our domain in the address bar on the way. That is what makes a phishing page
// convincing, and it is why the only thing accepted is a path on this instance.
//
// A leading slash and not two was the whole check, and it is not enough:
//
//   - "//evil.example" is protocol-relative and was already refused.
//   - "/\evil.example" is not, and browsers normalise the backslash to a
//     slash, so it becomes the case above after the check has passed.
//   - "/	//evil.example" and friends: control characters are stripped by the
//     parser, so the string checked is not the string followed.
//
// So: one leading slash, and the character after it may not be another slash
// or a backslash. No control characters anywhere. Nothing that would send
// somebody straight back to a login page, which is a loop rather than a
// vulnerability but is still not a destination.
func SafeRedirectTo(to string) string {
	const home = "/"
	if to == "" || to[0] != '/' {
		return home
	}
	if len(to) > 1 && (to[1] == '/' || to[1] == '\\') {
		return home
	}
	for _, c := range to {
		// Control characters and anything a URL parser might strip or fold.
		// The check has to be on the bytes that were sent, because what a
		// browser follows is what is left after it has removed these.
		if c < 0x20 || c == 0x7f {
			return home
		}
	}
	if path := strings.SplitN(to, "?", 2)[0]; path == "/login" || path == "/logout" || path == "/signup" {
		return home
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
	plain := fmt.Sprintf("Hi %s,\n\nClick the link below to verify your email address for Micro:\n\n%s\n\nThis link expires in 24 hours. If you didn't request this, you can ignore this email.\n\n— Micro", acc.Name, link)
	html := fmt.Sprintf(`<p>Hi %s,</p><p>Click the link below to verify your email address for Micro:</p><p><a href="%s">%s</a></p><p>This link expires in 24 hours. If you didn't request this, you can ignore this email.</p><p>— Micro</p>`, htmlpkg.EscapeString(acc.Name), link, link)

	if err := app.EmailSender(email, "Verify your Micro account", plain, html, ""); err != nil {
		app.Log("auth", "Failed to send verification email to %s: %v", email, err)
		app.ServerError(w, r, "Failed to send verification email. Please try again.")
		return
	}
	app.Log("auth", "Sent verification email to %s for account %s", email, acc.ID)
	http.Redirect(w, r, "/account/connections", http.StatusSeeOther)
}

// Preserve non-secret form fields on errors and the destination between auth pages.
func accountFormValues(page string, r *http.Request) string {
	if token := auth.CSRFToken(r); token != "" {
		page = strings.ReplaceAll(page, "</form>", `<input type="hidden" name="_csrf" value="`+htmlpkg.EscapeString(token)+`"></form>`)
	}
	if r.Method == "POST" {
		for _, name := range []string{"id"} {
			marker := `id="` + name + `" name="` + name + `"`
			page = strings.Replace(page, marker, marker+` value="`+htmlpkg.EscapeString(r.FormValue(name))+`"`, 1)
		}
	}
	if to := r.URL.Query().Get("redirect"); to != "" {
		for _, route := range []string{"/login", "/signup", "/oauth2/google"} {
			page = strings.ReplaceAll(page, `href="`+route+`"`, `href="`+route+`?redirect=`+htmlpkg.EscapeString(url.QueryEscape(SafeRedirectTo(to)))+`"`)
		}
	}
	return page
}

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

func renderPhoneCard(accountID string) string {
	if !sms.Configured() {
		return ""
	}

	mine := sms.Numbers(accountID)
	if len(mine) > 0 {
		var b strings.Builder
		for _, n := range mine {
			b.WriteString(`<div class="form-actions"><span><strong>` + htmlpkg.EscapeString(n) + `</strong> — verified</span>` +
				app.Form{Action: "/account", Inline: true,
					Hidden: map[string]string{"forget_number": n},
					Submit: "Forget"}.HTML() + `</div>`)
		}
		return app.SectionID("phone", "Phone",
			b.String(),
			app.Note("Micro recognises messages from this number as yours, on SMS or WhatsApp."),
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
		return app.SectionID("phone", "Phone",
			`<p>A code was texted to <strong>`+htmlpkg.EscapeString(pending)+`</strong>.</p>`,
			app.Form{Action: "/account", Inline: true,
				Hidden: map[string]string{"confirm_number": pending},
				Fields: []app.Field{{Name: "code", Required: true, Placeholder: "123456"}},
				Submit: "Confirm"}.HTML())
	}

	return app.SectionID("phone", "Phone",
		app.Note("Verify your number so Micro can recognise you on SMS or WhatsApp."),
		app.Form{Action: "/account", Inline: true,
			Fields: []app.Field{{Name: "verify_number", Type: "tel", Required: true,
				Placeholder: "+447700900123"}},
			Submit: "Send me a code"}.HTML())
}

func forwardingToggle(acc *auth.Account) string {
	if !acc.EmailVerified || strings.TrimSpace(acc.Email) == "" {
		return app.Note("Verify your email address above to receive email.")
	}
	on := MailForwardingOn(acc.ID)
	state, label, checked := "on", "Off", "false"
	if on {
		state, label, checked = "off", "On", "true"
	}
	return `<p>Email to <strong>` + htmlpkg.EscapeString(acc.Email) + `</strong>.</p><form method="POST" action="/account" class="form-actions"><input type="hidden" name="forwarding" value="` + state + `"><button type="submit" role="switch" aria-checked="` + checked + `" aria-label="Email me incoming mail">Email me incoming mail · ` + label + `</button></form>` +
		app.Note("Sends a copy of incoming mail, including Micro’s emailed replies and briefs. Other Inbox updates are not emailed yet.")
}

func renderEmailCard(acc *auth.Account) string {

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
			app.Note("Micro recognises email from this address as yours. It is also used for password recovery. Verifying a different address replaces it."),
			app.Form{Action: "/account", Inline: true,
				Fields: []app.Field{{Name: "email", Type: "email", Required: true,
					Placeholder: "you@example.com"}},
				Submit: "Verify a different address"}.HTML(),
			otherAddresses(acc))
	}

	pending := ""
	if acc.Email != "" {
		pending = app.NoteHTML(`A verification link was sent to <strong>` +
			htmlpkg.EscapeString(acc.Email) + `</strong>. Open it to verify your address. ` +
			`Submit again to resend.`)
	}

	return app.Section("Email",
		`<p>Verify your address to talk to Micro by email and recover your account.</p>`,
		pending,
		app.Form{Action: "/account", Inline: true,
			Fields: []app.Field{{Name: "email", Type: "email", Value: acc.Email,
				Required: true, Placeholder: "you@example.com"}},
			Submit: "Send verification"}.HTML())
}

func passwordCard(acc *auth.Account) string {
	note := "Set a password to sign in with your username. You can also use Google or a passkey."
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

// XMPP belongs with connection setup, not the main message composer.
func xmppConnectionDetails(acc *auth.Account) string {
	for _, c := range client.Personal() {
		if c.ID != "chat" {
			continue
		}
		_, domain, ok := strings.Cut(c.Address, "@")
		if !ok {
			continue
		}
		return `<div class="record-card"><h3>XMPP</h3><p>Use an XMPP client signed in to this server.</p><dl><dt>Your address</dt><dd><code>` + htmlpkg.EscapeString(acc.ID+"@"+domain) + `</code></dd><dt>Password</dt><dd>A token with Chat (XMPP) access.</dd><dt>Micro's address</dt><dd><code>` + htmlpkg.EscapeString(c.Address) + `</code></dd></dl><div class="form-actions"><a href="/token">Manage tokens</a><a href="` + htmlpkg.EscapeString(c.Href) + `">Open XMPP client</a></div></div>`
	}
	return ""
}
