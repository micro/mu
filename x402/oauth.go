package x402

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"strings"
)

// Consent stays on the developer host; issuance and PKCE remain in auth.
func authorizeHandler(w http.ResponseWriter, r *http.Request) {
	if requireAccount(w, r) == nil {
		return
	}
	if r.Method == http.MethodPost {
		if access := r.FormValue("access"); access != "all" && access != "services" {
			app.BadRequest(w, r, "Choose service access")
			return
		}
		auth.OAuthAuthorizePostHandler(w, r)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	q := r.URL.Query()
	client := auth.GetOAuthClient(q.Get("client_id"))
	if client == nil {
		app.BadRequest(w, r, "Unknown client")
		return
	}
	redirect, err := auth.RedirectFor(client.ClientID, q.Get("redirect_uri"))
	if err != nil {
		app.BadRequest(w, r, err.Error())
		return
	}
	requested := map[string]bool{}
	for _, scope := range strings.Fields(q.Get("scope")) {
		requested[scope] = true
	}
	var b strings.Builder
	b.WriteString(`<p>Allow <strong>` + html.EscapeString(client.Name) + `</strong> to call tools using your credits?</p><form method="POST" action="/oauth/authorize" class="form">` + app.CSRFField(auth.CSRFToken(r)))
	for _, key := range []string{"client_id", "state", "code_challenge", "code_challenge_method"} {
		b.WriteString(`<input type="hidden" name="` + key + `" value="` + html.EscapeString(q.Get(key)) + `">`)
	}
	b.WriteString(`<input type="hidden" name="redirect_uri" value="` + html.EscapeString(redirect) + `"><input type="hidden" name="access" value="services"><fieldset><legend>Allowed services</legend>`)
	selected := false
	for _, s := range service.Specs() {
		if requested[auth.ScopePrefix+s.Name] {
			selected = true
		}
	}
	for _, s := range service.Specs() {
		checked := ""
		if !selected || requested[auth.ScopePrefix+s.Name] {
			checked = " checked"
		}
		b.WriteString(`<label class="choice"><input type="checkbox" name="service" value="` + html.EscapeString(s.Name) + `"` + checked + `>` + html.EscapeString(s.NavLabel()) + `</label>`)
	}
	checked := ""
	if requested["write"] {
		checked = " checked"
	}
	b.WriteString(`</fieldset><label class="choice"><input type="checkbox" name="write" value="yes"` + checked + `>Allow actions</label><p>Access expires after 24 hours.</p><div class="form-actions"><button type="submit">Allow</button><a href="/">Cancel</a></div></form>`)
	app.Respond(w, r, app.Response{Title: "Connect your agent", HTML: b.String()})
}
