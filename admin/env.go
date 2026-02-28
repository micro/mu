package admin

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"mu/app"
	"mu/auth"
)

// knownEnvVars lists the environment variables the application may use.
// Values are never shown; only whether each variable is set.
var knownEnvVars = []string{
	// Core
	"MU_DOMAIN",
	"MU_USE_SQLITE",
	"DATA_DIR",
	// LLM providers
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_MODEL",
	"FANAR_API_KEY",
	"FANAR_API_URL",
	"OLLAMA_API_URL",
	"MODEL_API_URL",
	"MODEL_NAME",
	// Search
	"BRAVE_API_KEY",
	// External APIs
	"YOUTUBE_API_KEY",
	"GOOGLE_API_KEY",
	// Mail (outbound relay)
	"SMTP_HOST",
	"SMTP_PORT",
	"SMTP_USER",
	"SMTP_PASS",
	"SMTP_FROM",
	// Mail (inbound)
	"MAIL_DOMAIN",
	"MAIL_PORT",
	"MAIL_SELECTOR",
	"DKIM_PRIVATE_KEY",
	// Auth / passkeys
	"PASSKEY_ORIGIN",
	"PASSKEY_RP_ID",
	// Payments
	"STRIPE_SECRET_KEY",
	"STRIPE_PUBLISHABLE_KEY",
	"STRIPE_WEBHOOK_SECRET",
	"WALLET_SEED",
	"GOCARDLESS_ACCESS_TOKEN",
	"BITCOIN_ADDRESS",
	"ETHEREUM_ADDRESS",
	"SOLANA_ADDRESS",
	// Misc
	"DONATION_URL",
	"DOMAIN",
	"GPG_HOME",
	"GPG_KEYRING",
	"GNUPGHOME",
}

// EnvHandler shows which environment variables are configured (without leaking values).
func EnvHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	var content strings.Builder
	content.WriteString(`<div class="card">`)
	content.WriteString(`<h3>Environment Variables</h3>`)
	content.WriteString(`<p class="text-muted">Shows whether each variable is set. Values are never displayed.</p>`)
	content.WriteString(`<table class="admin-table">`)
	content.WriteString(`<thead><tr><th>Variable</th><th>Status</th></tr></thead><tbody>`)

	for _, name := range knownEnvVars {
		val := os.Getenv(name)
		status := `<span style="color:#c0392b;">✗ not set</span>`
		if val != "" {
			status = fmt.Sprintf(`<span style="color:#27ae60;">✓ set (%d chars)</span>`, len(val))
		}
		content.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td></tr>`, name, status))
	}

	content.WriteString(`</tbody></table>`)
	content.WriteString(`</div>`)
	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	pageHTML := app.RenderHTMLForRequest("Env Vars", "Environment Variables", content.String(), r)
	w.Write([]byte(pageHTML))
}
