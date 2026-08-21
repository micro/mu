// Package setup provides the first-run configuration flow for a self-hosted
// instance: a guided web page (and a companion `mu setup` CLI wizard) that
// creates the admin account and selects an AI provider, so a fresh `mu --serve`
// goes from "boots" to "works" without a treasure hunt through /admin/config.
package setup

import (
	"errors"
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

	if err := ApplyProvider(provider, key, baseURL); err != nil {
		w.Write([]byte(render(err.Error())))
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

// ApplyProvider writes the chosen AI provider into the settings keys the
// runtime reads. Shared by the web wizard and `mu setup`.
func ApplyProvider(provider, key, baseURL string) error {
	switch provider {
	case "claude":
		if key == "" {
			return errors.New("Enter your Anthropic API key, or pick another provider.")
		}
		settings.Set("ANTHROPIC_API_KEY", key)
	case "atlas":
		if key == "" {
			return errors.New("Enter your Atlas Cloud API key, or pick another provider.")
		}
		settings.Set("ATLAS_API_KEY", key)
	case "openrouter":
		if key == "" {
			return errors.New("Enter your OpenRouter API key, or pick another provider.")
		}
		settings.Set("OPENROUTER_API_KEY", key)
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		settings.Set("OPENAI_BASE_URL", baseURL)
		settings.Set("OPENAI_API_KEY", "ollama")
	default:
		return errors.New("Pick an AI provider.")
	}
	return nil
}

func render(errMsg string) string {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<p class="text-error m-0 mb-3">` + html.EscapeString(errMsg) + `</p>`
	}
	body := `<div class="card form-col">
  <h1 class="m-0 mb-2">Welcome to Mu</h1>
  <p class="text-secondary m-0 mb-5">Two quick things and you're running your own instance.</p>
  ` + errHTML + `
  <form method="POST" action="/setup">
    <h3 class="m-0 mb-2 text-1em">1 · Admin account</h3>
    <input name="username" placeholder="username" autocomplete="username" required
      class="form-input w-full lead-15 mb-2">
    <input name="password" type="password" placeholder="password (min 6 chars)" autocomplete="new-password" required
      class="form-input w-full lead-15 mb-5">

    <h3 class="m-0 mb-2 text-1em">2 · AI provider</h3>
    <label class="d-block m-0 mb-2"><input type="radio" name="provider" value="claude" checked> Anthropic Claude</label>
    <label class="d-block m-0 mb-2"><input type="radio" name="provider" value="atlas"> Atlas Cloud / DeepSeek</label>
    <label class="d-block m-0 mb-2"><input type="radio" name="provider" value="openrouter"> OpenRouter</label>
    <label class="d-block m-0 mb-3"><input type="radio" name="provider" value="ollama"> Ollama / OpenAI-compatible (local)</label>
    <input name="key" placeholder="API key (Claude, Atlas or OpenRouter)"
      class="form-input w-full lead-15 mb-2">
    <input name="base_url" placeholder="Ollama base URL (default http://localhost:11434/v1)"
      class="form-input w-full lead-15 mb-5">

    <button type="submit" class="btn w-full lead-15">Start Mu</button>
  </form>
  <p class="text-muted text-sm mt-4 m-0">You can change any of this later at <code>/admin/config</code>. Prefer the terminal? Run <code>mu setup</code>.</p>
</div>`
	return app.RenderHTML("Setup", "Set up your Mu instance", body, nil)
}
