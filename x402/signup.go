package x402

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/x402/registration"
	"net/http"
	"net/url"
	"strings"
)

func signupHandler(w http.ResponseWriter, r *http.Request) {
	message := ""
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			app.Forbidden(w, r, "Reopen the signup page and try again.")
			return
		}
		session, err := registration.Create(r)
		if err == nil {
			setSession(w, r, session.Token)
			http.Redirect(w, r, destination(r), http.StatusSeeOther)
			return
		}
		message = `<p role="alert">` + html.EscapeString(err.Error()) + `</p>`
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	invite := r.FormValue("invite")
	inviteField := ""
	if auth.InviteOnly() || invite != "" {
		inviteField = `<label>Invite code<input name="invite" value="` + html.EscapeString(invite) + `" required></label>`
	}
	body := `<div class="section-stack"><p>Create an account to add credits and connect your agent. Verify your email before creating API tokens.</p>` + message + `<form method="POST" action="/signup" class="form section-stack">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="redirect" value="` + html.EscapeString(destination(r)) + `"><label>Username<input name="id" autocomplete="username" required pattern="[a-z][a-z0-9_]{3,23}" value="` + html.EscapeString(r.FormValue("id")) + `"></label><p class="text-muted">4–24 lowercase letters, numbers or underscores. Start with a letter.</p><label>Password<input type="password" name="secret" autocomplete="new-password" minlength="6" required></label>` + inviteField + app.CaptchaHTML(app.NewCaptchaChallenge()) + `<button type="submit">Create account</button></form><p>Already have an account? <a href="/login?redirect=` + url.QueryEscape(destination(r)) + `">Sign in</a>.</p></div>`
	app.Respond(w, r, app.Response{Title: "Create an account", HTML: body})
}
func setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: 2592000, HttpOnly: true, Secure: secureBrowserRequest(r), SameSite: http.SameSiteLaxMode})
}
func verifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, err := auth.ConsumeEmailVerificationToken(r.URL.Query().Get("token")); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		app.Respond(w, r, app.Response{Title: "Email verified", HTML: `<p>You can now create a service token and call tools.</p><a class="btn" href="/account/tokens">Create a token</a>`})
		return
	}
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	acc := requireAccount(w, r)
	if acc == nil {
		return
	}
	if !app.GuestAllowed(r) {
		app.TooManyRequests(w, r, "Too many attempts. Try again later.")
		return
	}
	if err := registration.SendVerification(acc, strings.TrimSpace(r.FormValue("email")), origin.URL(r)); err != nil {
		app.BadRequest(w, r, err.Error())
		return
	}
	app.Respond(w, r, app.Response{Title: "Check your email", HTML: `<p>Follow the verification link in your email, then return here to create a token.</p><a href="/account">Back to account</a>`})
}
func verificationHTML(r *http.Request, acc *auth.Account) string {
	if acc.Admin || acc.Approved || acc.EmailVerified {
		return ""
	}
	if app.EmailSender == nil {
		return `<section class="section-card"><h2>Account approval</h2><p>Ask the instance operator to approve your account before adding credits or creating tokens. Email verification is not configured.</p></section>`
	}
	return `<section class="section-card section-stack"><h2>Verify your email</h2><p>Verify your email before adding credits and creating API tokens.</p><form method="POST" action="/verify" class="form">` + app.CSRFField(auth.CSRFToken(r)) + `<label>Email<input name="email" type="email" autocomplete="email" value="` + html.EscapeString(acc.Email) + `" required></label><button type="submit">Send verification email</button></form></section>`
}
