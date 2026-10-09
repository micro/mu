package x402

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/x402/billing"
)

const sessionCookie = "x402_session"

// Browser sessions are host-only. Explicit API credentials retain precedence.
func browserSession(r *http.Request) *http.Request {
	clone := r.Clone(r.Context())
	clone.Header.Del("Cookie")
	for _, c := range r.Cookies() {
		if c.Name != "session" && c.Name != sessionCookie {
			clone.AddCookie(c)
		}
	}
	if r.Header.Get("Authorization") == "" && r.Header.Get(api.TokenHeader) == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			clone.AddCookie(&http.Cookie{Name: "session", Value: c.Value})
		}
	}
	return clone
}

func destination(r *http.Request) string {
	to := r.URL.Query().Get("redirect")
	if to == "" {
		to = r.FormValue("redirect")
	}
	u, err := url.Parse(to)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.ContainsAny(to, "\\\r\n\t") {
		return "/account"
	}
	switch u.Path {
	case "/account", "/account/topup", "/account/tokens", "/account/usage", "/oauth/authorize":
		return to
	}
	return "/account"
}
func requireAccount(w http.ResponseWriter, r *http.Request) *auth.Account {
	sess, acc, err := auth.RequireSession(r)
	if err != nil || sess.Type != "account" || acc.Banned {
		if app.WantsJSON(r) || r.Header.Get("Authorization") != "" || r.Header.Get(api.TokenHeader) != "" {
			app.RespondError(w, 401, "Sign in to manage your account")
		} else {
			http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		}
		return nil
	}
	return acc
}
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if session, _, err := auth.RequireSession(r); err == nil && session.Type == "account" {
			http.Redirect(w, r, destination(r), http.StatusSeeOther)
			return
		}
	}

	message := ""
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			app.Forbidden(w, r, "Reopen the sign-in page and try again.")
			return
		}
		if !app.GuestAllowed(r) {
			app.TooManyRequests(w, r, "Too many attempts. Try again later.")
			return
		}
		sess, err := auth.Login(r.FormValue("id"), r.FormValue("secret"))
		if err == nil {
			acc, _ := auth.GetAccount(sess.Account)
			if acc == nil || acc.Banned {
				auth.Logout(sess.Token)
				app.Forbidden(w, r, "Account unavailable")
				return
			}
			billing.RetrySignup(sess.Account)
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sess.Token, Path: "/", MaxAge: 2592000, HttpOnly: true, Secure: secureBrowserRequest(r), SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, destination(r), http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		message = `<p role="alert">Invalid username or password.</p>`
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	body := `<p>Sign in with your existing account to manage credits, tokens and usage.</p>` + message + `<form method="POST" action="/login" class="form">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="redirect" value="` + html.EscapeString(destination(r)) + `"><label>Username<input name="id" autocomplete="username" required></label><label>Password<input type="password" name="secret" autocomplete="current-password" required></label><div class="form-actions"><button type="submit">Sign in</button></div></form>`
	app.Respond(w, r, app.Response{Title: "Sign in", HTML: body})
}
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if c, err := r.Cookie("session"); err == nil {
		auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureBrowserRequest(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func secureBrowserRequest(r *http.Request) bool {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	// A same-origin browser submission can also identify upstream TLS.
	u, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && u.Scheme == "https" && r.Header.Get("Sec-Fetch-Site") == "same-origin"
}
