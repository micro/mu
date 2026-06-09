package admin

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

type settingGroup struct {
	Name string
	Vars []string
}

var settingGroups = []settingGroup{
	{"AI", []string{
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_MODEL",
		"ATLAS_API_KEY",
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
	}},
	{"Search", []string{
		"BRAVE_API_KEY",
		"YOUTUBE_API_KEY",
		"GOOGLE_API_KEY",
	}},
	{"Mail", []string{
		"MAIL_DOMAIN",
		"MAIL_PORT",
		"MAIL_SELECTOR",
		"DKIM_PRIVATE_KEY",
		"SMTP_HOST",
		"SMTP_PORT",
		"SMTP_USER",
		"SMTP_PASS",
	}},
	{"Payments", []string{
		"STRIPE_SECRET_KEY",
		"STRIPE_PUBLISHABLE_KEY",
		"STRIPE_WEBHOOK_SECRET",
		"X402_PAY_TO",
	}},
	{"Trading", []string{
		"TRADE_RPC_URL",
		"TRADE_CHAIN",
	}},
	{"Discord", []string{
		"DISCORD_BOT_TOKEN",
	}},
	{"Platform", []string{
		"MU_DOMAIN",
		"DATA_DIR",
		"PASSKEY_ORIGIN",
		"PASSKEY_RP_ID",
	}},
}

func EnvHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		for _, group := range settingGroups {
			for _, key := range group.Vars {
				val := r.FormValue(key)
				if val == "••••••" || val == "" {
					continue
				}
				settings.Set(key, val)
			}
		}
		http.Redirect(w, r, "/admin/env?saved=1", http.StatusSeeOther)
		return
	}

	var b strings.Builder

	if r.URL.Query().Get("saved") == "1" {
		b.WriteString(`<div class="card" style="background:#f0fff0;border-color:#a3d9a5"><p style="color:#27ae60;margin:0">Settings saved. Restart to apply changes to env-loaded services.</p></div>`)
	}

	b.WriteString(`<form method="POST" action="/admin/env">`)

	for _, group := range settingGroups {
		b.WriteString(`<div class="card">`)
		b.WriteString(fmt.Sprintf(`<h3>%s</h3>`, group.Name))

		for _, key := range group.Vars {
			source := settings.Source(key)
			val := settings.Get(key)

			displayVal := ""
			badge := `<span style="font-size:11px;color:#c00">not set</span>`
			if source == "env" {
				displayVal = "••••••"
				badge = `<span style="font-size:11px;color:#27ae60">env</span>`
			} else if source == "saved" {
				displayVal = "••••••"
				badge = `<span style="font-size:11px;color:#2980b9">saved</span>`
			}

			_ = val

			isSecret := strings.Contains(strings.ToUpper(key), "KEY") ||
				strings.Contains(strings.ToUpper(key), "SECRET") ||
				strings.Contains(strings.ToUpper(key), "TOKEN") ||
				strings.Contains(strings.ToUpper(key), "PASS")

			inputType := "text"
			if isSecret {
				inputType = "password"
			}

			b.WriteString(fmt.Sprintf(`<div style="margin-bottom:10px">
				<label style="font-size:12px;color:#888;display:block;margin-bottom:2px"><code>%s</code> %s</label>
				<input type="%s" name="%s" value="%s" placeholder="not set" autocomplete="off"
					style="width:100%%;padding:6px 8px;border:1px solid #ddd;border-radius:4px;font-size:13px;box-sizing:border-box;font-family:monospace">
				</div>`, key, badge, inputType, key, displayVal))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<button type="submit" class="btn" style="margin-bottom:16px">Save Settings</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	html := app.RenderHTMLForRequest("Settings", "Platform configuration", b.String(), r)
	w.Write([]byte(html))
}
