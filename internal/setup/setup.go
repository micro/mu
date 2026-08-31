// Package setup provides the first-run configuration flow for a self-hosted
// instance: a guided web page (and a companion `mu setup` CLI wizard) that
// creates the admin account and, if you have one to hand, selects an AI
// provider — so a fresh `mu --serve` goes from "boots" to "works" without a
// treasure hunt through /admin/config.
//
// # The model is optional, and that is the point
//
// It was required: every branch of ApplyProvider refused, so an instance could
// not be finished without an account at a model vendor. That made a utility
// something you cannot start using until you have signed up for somebody
// else's product, which is the opposite of what running your own server is
// for.
//
// It is also not true of most of what is here. Mail, IMAP, SMTP, XMPP, files,
// notes and the record all work with no model at all — that is the utility,
// and the agent is the upgrade. Blocking the front door on the upgrade
// misdescribed the product to every person who ever installed it.
//
// So the provider can be left for later. The agent says what it needs when it
// is asked something, which it already did: see ErrNoProvider in agent/native.go
// — "An agent with no provider cannot answer. Saying so is the whole handling."
package setup

import (
	"errors"
	"html"
	"net/http"
	"strings"
	"time"

	"mu/internal/ai"
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
	// The body only. FormValue would also accept ?password=… and write the
	// first admin's password into the access log of whatever is in front.
	secret := r.PostFormValue("password")
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

	// Only when one was chosen. "later" is a real answer and the default.
	if provider != "" && provider != "later" {
		if err := ApplyProvider(provider, key, baseURL); err != nil {
			w.Write([]byte(render(err.Error())))
			return
		}
	}

	// Create the admin account. auth.Create bootstraps the first account as
	// admin; promote explicitly too in case ADMIN is set to someone else.
	// SecretSet, because this password was typed into the form above by a
	// person. Without it /account tells the first admin of every new instance
	// that their account "signs in with Google or a passkey and has no password
	// you could type" — about the one account on the instance that was
	// definitely created by typing a password.
	if err := auth.Create(&auth.Account{
		ID: id, Name: id, Secret: secret, SecretSet: true, Created: time.Now(),
	}); err != nil {
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
		// And which model, asked of the server rather than guessed.
		//
		// This wrote an endpoint and a key and no model, which was survivable
		// while the model defaulted to gpt-4o-mini — and that default was a
		// model no Ollama has, so the reward for finishing setup was a 404
		// naming something the operator never typed. The default is gone, so
		// without this the reward is an instance that says it is not
		// configured, which is truthful and just as useless.
		//
		// The server knows. Somebody who has just said "use Ollama" should not
		// then be asked to type an id that /models would have answered.
		// Nothing stored when it cannot be reached: an instance that says it
		// needs a model beats one that names a model that is not there.
		if m := ai.DetectLocalModel(baseURL, "ollama"); m != "" {
			settings.Set("OPENAI_MODEL", m)
		}
	case "", "later", "none":
		// Nothing to write. Everything that does not need a model still works,
		// and /admin/config is where one is added when there is one.
		return nil
	default:
		return errors.New("Pick an AI provider, or leave it for later.")
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
  <p class="text-secondary m-0 mb-5">One thing and you're running your own instance.</p>
  ` + errHTML + `
  <form method="POST" action="/setup">
    <h3 class="m-0 mb-2 text-1em">1 · Admin account</h3>
    <input name="username" placeholder="username" autocomplete="username" required
      class="form-input w-full lead-15 mb-2">
    <input name="password" type="password" placeholder="password (min 6 chars)" autocomplete="new-password" required
      class="form-input w-full lead-15 mb-5">

    <h3 class="m-0 mb-2 text-1em">2 · AI provider <span class="text-muted text-sm">— optional</span></h3>
    <p class="text-muted text-sm m-0 mb-3">Mail, chat, files, notes and your inbox work without one.
      A model is what lets the agent answer; add it now or at <code>/admin/config</code> later.</p>
    <label class="d-block m-0 mb-2"><input type="radio" name="provider" value="later" checked> Not yet — start without an agent</label>
    <label class="d-block m-0 mb-2"><input type="radio" name="provider" value="claude"> Anthropic Claude</label>
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
