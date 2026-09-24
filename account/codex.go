package account

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/codex"
	"net/http"
)

func codexCard(acc *auth.Account) string {
	if !acc.Admin {
		return ""
	}
	state, label := "on", "Enable for my conversations"
	text := "Off. Your conversations use the site's current provider."
	if acc.CodexPreview {
		state, label = "off", "Use the site provider"
		text = "On. Your direct assistant conversations use GPT-6-Astra through Codex. Other accounts and scheduled work keep the site's provider."
	}
	form := app.Form{Action: "/account/codex", Hidden: map[string]string{"state": state}, Submit: label}.HTML()
	if !acc.CodexPreview && !codex.Checked() {
		form = app.Note("Server setup is required: run mu codex login, then mu codex check as the Micro server user.")
	}
	return app.SectionID("codex", "Codex preview", `<p>`+text+`</p>`, app.Note("Admin testing only. Uses the server's ChatGPT allowance. Micro supplies your conversation and permitted tools; personal Codex history and connectors are excluded."), form)
}

// CodexHandler changes only the signed-in admin's own preference.
func CodexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", 405)
		return
	}
	_, acc, e := auth.RequireAdmin(r)
	if e != nil {
		app.Forbidden(w, r, "Admin account required")
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid form token")
		return
	}
	if e = r.ParseForm(); e != nil {
		app.BadRequest(w, r, "Invalid form")
		return
	}
	state := r.PostForm.Get("state")
	if state != "on" && state != "off" {
		app.BadRequest(w, r, "Choose on or off")
		return
	}
	if state == "on" && !codex.Checked() {
		app.BadRequest(w, r, "Run mu codex check successfully on the server first")
		return
	}
	if e = auth.SetCodexPreview(acc.ID, state == "on"); e != nil {
		http.Error(w, "Could not save preference", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account#codex", http.StatusSeeOther)
}
