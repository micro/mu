package account

import (
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

func createOAuthClient(w http.ResponseWriter, r *http.Request, accountID string) {
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Reload and try again.")
		return
	}
	name := strings.TrimSpace(r.FormValue("client_name"))
	uris := strings.Fields(r.FormValue("redirect_uris"))
	if name == "" || len(name) > 80 || len(uris) == 0 || len(uris) > 10 {
		app.BadRequest(w, r, "Enter a name and 1–10 callback URLs.")
		return
	}
	for _, uri := range uris {
		if !auth.RegisterableRedirect(uri) {
			app.BadRequest(w, r, "Callback URLs must use HTTPS or HTTP on localhost.")
			return
		}
	}
	if _, err := auth.RegisterOwnedOAuthClient(accountID, name, uris); err != nil {
		app.BadRequest(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/account/clients", http.StatusSeeOther)
}

func oauthClientForm(r *http.Request) string {
	return `<h2>OAuth client</h2><form method="POST" action="/account/clients?create_client=1" class="form">` + app.CSRFField(auth.CSRFToken(r)) + `<label class="field-label">Name<input name="client_name" maxlength="80" required></label><label class="field-label">Callback URLs<textarea name="redirect_uris" rows="3" required></textarea></label><p>One URL per line. HTTPS or localhost.</p><button type="submit">Register</button></form>`
}
