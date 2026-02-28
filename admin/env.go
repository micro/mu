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
	"BRAVE_API_KEY",
	"OPENAI_API_KEY",
	"SMTP_HOST",
	"SMTP_PORT",
	"SMTP_USER",
	"SMTP_PASS",
	"SMTP_FROM",
	"DKIM_PRIVATE_KEY",
	"DOMAIN",
	"DATA_DIR",
	"STRIPE_SECRET_KEY",
	"STRIPE_WEBHOOK_SECRET",
	"GOCARDLESS_ACCESS_TOKEN",
	"BITCOIN_ADDRESS",
	"ETHEREUM_ADDRESS",
	"SOLANA_ADDRESS",
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
