// Package setup provides the first-run configuration flow for a self-hosted
// instance: a guided web page (and a companion `mu setup` CLI wizard) that
// creates the admin account and selects an AI provider, so a fresh `mu --serve`
// goes from "boots" to "works" without a treasure hunt through /admin/env.
package setup

import (
	"html"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

// Needed reports whether the instance still needs first-run setup — i.e. no
// admin account exists yet. Once an admin exists the flow closes and routing
// stops sending people here.
func Needed() bool {
	return !auth.AdminExists()
}

// Handler serves GET /setup (the form) and POST /setup (apply). It is only open
// while no admin exists; afterwards it redirects to /login so it can't be used
// to mint a second admin.
func Handler(w http.ResponseWriter, r *http.Request) {
	if auth.AdminExists() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		applySetup(w, r)
		return
	}

	w.Write([]byte(render("")))
}

func applySetup(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id := strings.TrimSpace(r.FormValue("username"))
	secret := r.FormValue("password")
	provider := r.FormValue("provider")
	key := strings.TrimSpace(r.FormValue("key"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))

	if id == "" {
		w.Write([]byte(render("Choose a username for your admin account.")))
		return
	}
	if len(secret) < 6 {
		w.Write([]byte(render("Password must be at least 6 characters.")))
		return
	}

	// Resolve the AI provider into the settings keys the runtime reads.
	switch provider {
	case "claude":
		if key == "" {
			w.Write([]byte(render("Enter your Anthropic API key, or pick another provider.")))
			return
		}
		settings.Set("ANTHROPIC_API_KEY", key)
	case "atlas":
		if key == "" {
			w.Write([]byte(render("Enter your Atlas Cloud API key, or pick another provider.")))
			return
		}
		settings.Set("ATLAS_API_KEY", key)
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		settings.Set("OPENAI_BASE_URL", baseURL)
		settings.Set("OPENAI_API_KEY", "ollama")
	default:
		w.Write([]byte(render("Pick an AI provider.")))
		return
	}

	// Create the admin account. auth.Create bootstraps the first account as
	// admin; promote explicitly too in case ADMIN is set to someone else.
	if err := auth.Create(&auth.Account{ID: id, Name: id, Secret: secret, Created: time.Now()}); err != nil {
		w.Write([]byte(render(err.Error())))
		return
	}
	if acc, err := auth.GetAccount(id); err == nil && !acc.Admin {
		acc.Admin = true
		auth.UpdateAccount(acc)
	}

	sess, err := auth.Login(id, secret)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	secure := r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: sess.Token, Path: "/", MaxAge: 2592000,
		Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func render(errMsg string) string {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<p style="color:#c00;margin:0 0 12px">` + html.EscapeString(errMsg) + `</p>`
	}
	body := `<div class="card" style="max-width:520px;margin:0 auto">
  <h1 style="margin:0 0 6px">Welcome to Mu</h1>
  <p style="color:#666;margin:0 0 20px">Two quick things and you're running your own instance.</p>
  ` + errHTML + `
  <form method="POST" action="/setup">
    <h3 style="margin:0 0 8px;font-size:1em">1 · Admin account</h3>
    <input name="username" placeholder="username" autocomplete="username" required
      style="width:100%;padding:10px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:15px">
    <input name="password" type="password" placeholder="password (min 6 chars)" autocomplete="new-password" required
      style="width:100%;padding:10px;margin:0 0 20px;border:1px solid #ddd;border-radius:6px;font-size:15px">

    <h3 style="margin:0 0 8px;font-size:1em">2 · AI provider</h3>
    <label style="display:block;margin:0 0 6px"><input type="radio" name="provider" value="claude" checked> Anthropic Claude</label>
    <label style="display:block;margin:0 0 6px"><input type="radio" name="provider" value="atlas"> Atlas Cloud / DeepSeek</label>
    <label style="display:block;margin:0 0 12px"><input type="radio" name="provider" value="ollama"> Ollama / OpenAI-compatible (local)</label>
    <input name="key" placeholder="API key (Claude or Atlas)"
      style="width:100%;padding:10px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:15px">
    <input name="base_url" placeholder="Ollama base URL (default http://localhost:11434/v1)"
      style="width:100%;padding:10px;margin:0 0 20px;border:1px solid #ddd;border-radius:6px;font-size:15px">

    <button type="submit" style="width:100%;padding:12px;background:#111;color:#fff;border:none;border-radius:6px;font-size:15px;cursor:pointer">Start Mu</button>
  </form>
  <p style="color:#888;font-size:13px;margin:16px 0 0">You can change any of this later at <code>/admin/env</code>. Prefer the terminal? Run <code>mu setup</code>.</p>
</div>`
	return app.RenderHTML("Setup", "Set up your Mu instance", body)
}
