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
		"ATLAS_MODEL",
		"OPENROUTER_API_KEY",
		"OPENROUTER_MODEL",
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
		"IMAGE_MODEL",
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
		"CRYPTO_TOPUP",
	}},
	{"Trading", []string{
		"TRADE_RPC_URL",
		"TRADE_CHAIN",
	}},
	{"Discord", []string{
		"DISCORD_BOT_TOKEN",
	}},
	{"Telegram", []string{
		"TELEGRAM_BOT_TOKEN",
	}},
	{"WhatsApp", []string{
		"WHATSAPP_TOKEN",
		"WHATSAPP_PHONE_ID",
		"WHATSAPP_VERIFY_TOKEN",
		"WHATSAPP_APP_SECRET",
	}},
	// SMS and WhatsApp-over-Twilio. These were absent, and /sms and /whatsapp
	// both send an operator here by name to set them — a page pointing at a
	// page that could not help, which is worse than no pointer at all.
	{"Twilio — SMS and WhatsApp", []string{
		"TWILIO_ACCOUNT_SID",
		"TWILIO_AUTH_TOKEN",
		"TWILIO_API_KEY",
		"TWILIO_API_SECRET",
		"TWILIO_FROM",
		"TWILIO_MESSAGING_SERVICE_SID",
		"TWILIO_WHATSAPP_FROM",
		"TWILIO_WEBHOOK_URL",
		"SMS_DEFAULT_COUNTRY",
		"SMS_COUNTRIES",
		"SMS_KNOWN_ONLY",
		"SMS_VERIFY_INBOUND",
	}},
	{"Sign-in", []string{
		"GOOGLE_CLIENT_ID",
		"GOOGLE_CLIENT_SECRET",
		"GOOGLE_REDIRECT_URI",
	}},
	{"Storage", []string{
		"S3_ENDPOINT",
		"S3_BUCKET",
		"S3_REGION",
		"S3_ACCESS_KEY",
		"S3_SECRET_KEY",
	}},
	{"Social", []string{
		"SOCIAL_ATPROTO",
	}},
	{"Platform", []string{
		"MU_DOMAIN",
		"PASSKEY_ORIGIN",
		"PASSKEY_RP_ID",
	}},
}

// Settable reports whether a setting can be changed from this page.
//
// Exported so a service can be checked against it: /sms and /whatsapp tell an
// operator to come here for a named variable, and nothing stopped that claim
// from going stale — which is exactly what happened to every Twilio setting.
func Settable(name string) bool {
	for _, g := range settingGroups {
		for _, v := range g.Vars {
			if v == name {
				return true
			}
		}
	}
	return false
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
