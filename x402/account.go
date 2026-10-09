package x402

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/internal/service"
	"mu/x402/billing"
	"net/http"
	"strings"
	"time"
)

func accountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	acc := requireAccount(w, r)
	if acc == nil {
		return
	}
	if app.WantsJSON(r) {
		app.RespondJSON(w, billing.State(acc))
		return
	}
	body := fmt.Sprintf(`<p>Signed in as <strong>%s</strong>.</p><section class="section-card"><h2>Credits</h2><p><strong>%d prepaid credits</strong></p><p>%d signup credits remaining · %d daily credits remaining</p><p>Use an API token with /api/v1 or /mcp to charge your credit balance. Wallet payments use x402 per request.</p><div class="form-actions"><a class="btn" href="/account/topup">Top up credits</a><a class="btn" href="/account/usage">View usage</a></div></section>`, html.EscapeString(acc.ID), billing.Balance(acc.ID), billing.SignupRemaining(acc.ID), billing.IncludedToday(acc.ID))
	body = verificationHTML(r, acc) + body
	body += billing.SubscriptionSummary(r, acc)
	body += `<section class="section-card section-stack"><h2>Connect your agent</h2><p>Create a service token, then send it as an Authorization: Bearer header to this host.</p><a class="btn" href="/account/tokens">Manage API tokens</a></section>`
	app.Respond(w, r, app.Response{Title: "Account", HTML: body})
}

func tokensHandler(w http.ResponseWriter, r *http.Request) {
	acc := requireAccount(w, r)
	if acc == nil {
		return
	}
	if err := auth.CheckCredentialAccess(acc.ID); err != nil {
		app.Respond(w, r, app.Response{Title: "API tokens", HTML: verificationHTML(r, acc)})
		return
	}
	result := ""
	if r.Method == http.MethodPost {
		if id := r.FormValue("delete"); id != "" {
			if err := auth.DeleteToken(id, acc.ID); err != nil {
				app.Forbidden(w, r, err.Error())
				return
			}
			http.Redirect(w, r, "/account/tokens", http.StatusSeeOther)
			return
		}
		if err := auth.CheckCredentialAccess(acc.ID); err != nil {
			app.Forbidden(w, r, err.Error())
			return
		}
		if err := auth.CheckPostRate(acc.ID); err != nil {
			app.TooManyRequests(w, r, err.Error())
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" || len(name) > 100 {
			app.BadRequest(w, r, "Enter a token name of at most 100 characters")
			return
		}
		var names []string
		for _, spec := range service.Specs() {
			names = append(names, spec.Name)
		}
		if len(names) == 0 {
			app.BadRequest(w, r, "No services are available")
			return
		}
		_, raw, err := auth.CreateToken(acc.ID, name, append([]string{"read", "write"}, auth.ScopeFor(names)...), time.Now().AddDate(0, 0, 90))
		if err != nil {
			app.Forbidden(w, r, err.Error())
			return
		}
		result = `<section class="section-card" role="status"><h2>Token created</h2><p>Copy it now. It is shown only once.</p><pre id="new-token">` + html.EscapeString(raw) + `</pre><button type="button" data-copy-token>Copy token</button><span data-token-copy-status aria-live="polite"></span></section>`
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	csrf := app.CSRFField(auth.CSRFToken(r))
	body := `<div class="section-stack">` + result + `<p>Service tokens work with this host’s API and MCP endpoint and use your account’s credits. Tokens created here allow service reads and actions, expire after 90 days, and do not grant access to the consumer APIs.</p><form method="POST" action="/account/tokens" class="form">` + csrf + `<label>Token name<input name="name" required maxlength="100" placeholder="My agent"></label><div class="form-actions"><button type="submit">Create token</button></div></form>`
	body += `<section class="section-stack"><h2>Your service tokens</h2>`
	count := 0
	for _, t := range auth.ListTokens(acc.ID) {
		if len(t.Services()) == 0 {
			continue
		}
		count++
		body += `<div class="section-card"><strong>` + html.EscapeString(t.Name) + `</strong><p>` + html.EscapeString(strings.Join(t.Services(), ", ")) + `</p><form method="POST" action="/account/tokens">` + csrf + `<button name="delete" value="` + html.EscapeString(t.ID) + `">Revoke</button></form></div>`
	}
	if count == 0 {
		body += `<p class="text-muted">No service tokens yet.</p>`
	}
	body += `</section>` + connectionHTML(r)
	app.Respond(w, r, app.Response{Title: "API tokens", HTML: body + `</div>`})
}

func connectionHTML(r *http.Request) string {
	return `<section id="connect" class="section-stack"><h2>Connect your agent</h2><p>MCP endpoint: <code>` + html.EscapeString(origin.URL(r)) + `/mcp</code></p><p>Set the HTTP header <code>Authorization: Bearer YOUR_TOKEN</code> in your MCP client or API request. Calls use your account’s available credits.</p><p><a href="/account/tokens">Get a token</a> · <a href="/account/topup">Add credits</a> · <a href="/account/usage">Check usage</a></p></section>`
}
