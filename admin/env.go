package admin

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
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
		"MAIL_WHITELIST",
		"MAIL_PORT",
		"MAIL_SELECTOR",
		"DKIM_PRIVATE_KEY",
		"SMTP_HOST",
		"SMTP_PORT",
		"SMTP_USER",
		"SMTP_PASS",
	}},
	// Object storage. Backups go here first, because a copy on the same disk
	// does not survive losing the disk — and later the same bucket is where
	// files and generated images belong, which is why these are named for the
	// storage rather than for the backup.
	{"Object storage (S3)", []string{
		"S3_BUCKET",
		"S3_REGION",
		"S3_ENDPOINT",
		"S3_ACCESS_KEY_ID",
		"S3_SECRET_ACCESS_KEY",
		"S3_PREFIX",
		"BACKUP_S3",
	}},
	{"Payments", []string{
		"STRIPE_SECRET_KEY",
		"STRIPE_PUBLISHABLE_KEY",
		"STRIPE_WEBHOOK_SECRET",
		"X402_PAY_TO",
		"X402_BAZAAR",
	}},
	// The node this instance reads balances from. BASE_RPC_URL was readable by
	// the code and settable nowhere, so the only way to point it at Base was an
	// environment edit and a restart — and until it was set, BaseRPCURL fell
	// back to TRADE_RPC_URL, which is for trading and may be on another chain
	// entirely. That silently reported every balance as zero.
	{"Chain", []string{
		"BASE_RPC_URL",
		"TRADE_RPC_URL",
		"TRADE_CHAIN",
	}},
	{"Transit", []string{
		"TRANSIT_FEEDS",
		// Optional everywhere: transit answers with no key at all. This only
		// raises TfL's rate limit, and it was readable by the code and
		// settable nowhere — the same gap the Twilio group below records.
		"TFL_APP_KEY",
		// The two that make transit live outside London: buses from the DfT,
		// trains from National Rail. Both are free to register for and both
		// were the reason /transit could only say what the timetable promised.
		"BODS_API_KEY",
		"LDBWS_TOKEN",
	}},
	// The basemap. /tiles sends an operator here by name when it has no key,
	// so this group has to exist for that sentence to be true — see the note
	// on the Twilio group, which is the same mistake found the same way.
	{"Maps", []string{
		"OS_MAPS_KEY",
		"TILE_FETCH_PER_HOUR",
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
	// Email that leaves the instance, which is a different thing from the
	// mailbox — see service/email. /email names all of these and sends an
	// operator here, so they have to be here.
	{"Sending limits and email out", []string{
		"EMAIL_DOMAIN",
		"EMAIL_REPLY_DOMAIN",
		"EMAIL_DAILY_LIMIT",
		"SMS_DAILY_LIMIT",
		"WHATSAPP_DAILY_LIMIT",
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
		// Proof of domain ownership for the MCP registry, served at
		// /.well-known/mcp-registry-auth. It was readable by the code and
		// settable nowhere, so publishing meant an environment edit and a
		// restart on a box somebody had to have shell on.
		"MCP_REGISTRY_PROOF",
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

// secret reports whether a value should be kept out of the page by default.
//
// By name, because that is all there is to go on and it is what an operator
// reading the page assumes too: anything called a key, a secret, a token or a
// password. Wrong in the safe direction — MAIL_SELECTOR is not a secret and
// nothing breaks by masking it — where a name that ought to be on this list and
// is not is a credential printed in full on a page somebody may be sharing.
func secret(key string) bool {
	up := strings.ToUpper(key)
	// RPC_URL is here because every hosted provider puts the credential in the
	// path — Alchemy, Infura and QuickNode all hand you a URL ending in the key
	// itself. Nothing in the name says "key", so the old list printed somebody's
	// paid endpoint in full on a page they might be sharing a screenshot of.
	for _, w := range []string{"KEY", "SECRET", "TOKEN", "PASS", "DKIM", "RPC_URL"} {
		if strings.Contains(up, w) {
			return true
		}
	}
	return false
}

// hint is enough of a value to recognise it by and not enough to use.
//
// The point is identification: an operator with three Stripe keys in three
// places needs to know which one this is, and "••••••" answers nothing. A short
// value gets no hint, because half of a six-character secret is most of it.
func hint(v string) string {
	if len(v) <= 12 {
		return strings.Repeat("•", 6)
	}
	return v[:4] + "…" + v[len(v)-4:]
}

// EnvHandler is /admin/env: what this instance is configured with, where each
// value came from, and which of them this page can change.
//
// It used to answer none of those. Every value that was set at all rendered as
// "••••••" — the handler fetched the real one and threw it away on the next
// line, `_ = val` — so the page could say a thing was configured and never what
// it was configured to. On an instance with some settings in a .env file, some
// in the store, and the same names in both, "which is which" was unanswerable
// from the one page that exists to answer it.
//
// Worse, a value coming from the environment was rendered in an editable box.
// The environment wins in settings.Get, permanently, so typing a new value
// there and pressing Save stored something that would never be read — and the
// page said "Settings saved". The environment is now shown as what it is: in
// force here, changeable where it was set.
func EnvHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		r.ParseForm() //nolint:errcheck
		for _, group := range settingGroups {
			for _, key := range group.Vars {
				// The environment wins whatever is stored, so writing one of
				// these would be storing a value nothing reads.
				if settings.Source(key) == "env" {
					continue
				}
				if r.FormValue("clear_"+key) != "" {
					settings.Set(key, "")
					continue
				}
				val := strings.TrimSpace(r.FormValue(key))
				// A secret's box starts empty, so empty means "leave it alone"
				// and clearing one is the checkbox beside it. A plain value's
				// box holds the value, so empty means empty — which is the only
				// way to unset anything, and there was not one before.
				if val == "" && secret(key) {
					continue
				}
				settings.Set(key, val)
			}
		}
		http.Redirect(w, r, "/admin/env?saved=1", http.StatusSeeOther)
		return
	}

	reveal := r.URL.Query().Get("reveal")

	var b strings.Builder

	if r.URL.Query().Get("saved") == "1" {
		b.WriteString(`<div class="card" class="card-ok"><p class="text-success m-0">Saved. Anything read once at start-up needs a restart to take effect.</p></div>`)
	}

	// The count first, because "which of these is coming from where" is asked
	// about the instance and a per-row badge answers it one row at a time.
	var fromEnv, saved, unset int
	for _, g := range settingGroups {
		for _, key := range g.Vars {
			switch settings.Source(key) {
			case "env":
				fromEnv++
			case "saved":
				saved++
			default:
				unset++
			}
		}
	}
	b.WriteString(`<div class="card"><h3>Where these come from</h3>`)
	b.WriteString(fmt.Sprintf(`<p class="text-base text-secondary m-0 mb-2">`+
		`<b>%d</b> from the environment · <b>%d</b> saved here · <b>%d</b> not set</p>`,
		fromEnv, saved, unset))
	b.WriteString(`<p class="text-sm text-muted m-0">` +
		`The environment wins. A value set in the shell, a <code>.env</code> file or a ` +
		`systemd unit cannot be changed from this page — it is shown, and marked, and its ` +
		`box is locked, because saving over it would store something nothing reads.</p>`)
	b.WriteString(`</div>`)

	b.WriteString(`<form method="POST" action="/admin/env">`)

	for _, group := range settingGroups {
		b.WriteString(`<div class="card">`)
		b.WriteString(fmt.Sprintf(`<h3>%s</h3>`, html.EscapeString(group.Name)))

		for _, key := range group.Vars {
			source := settings.Source(key)
			val := settings.Get(key)
			isSecret := secret(key)
			shown := reveal == key

			var badge string
			switch source {
			case "env":
				badge = `<span class="text-2xs text-success">environment</span>`
			case "saved":
				badge = `<span class="text-2xs link-colour">saved here</span>`
			default:
				badge = `<span class="text-2xs text-faint">not set</span>`
			}

			b.WriteString(`<div class="mb-3">`)
			b.WriteString(fmt.Sprintf(
				`<label class="text-xs text-muted d-block mb-1"><code>%s</code> %s`,
				html.EscapeString(key), badge))
			// Revealing is a round trip rather than a script, so a secret is not
			// sitting in the page source of every visit waiting to be read.
			if isSecret && val != "" && !shown {
				b.WriteString(` <a href="/admin/env?reveal=` + url.QueryEscape(key) +
					`" class="text-2xs text-muted">show</a>`)
			} else if isSecret && shown {
				b.WriteString(` <a href="/admin/env" class="text-2xs text-muted">hide</a>`)
			}
			b.WriteString(`</label>`)

			switch {
			case source == "env":
				// Read-only and unnamed, so the POST above cannot see it even if
				// somebody edits the DOM.
				display := val
				if isSecret && !shown {
					display = hint(val)
				}
				b.WriteString(fmt.Sprintf(
					`<input type="text" value="%s" readonly
						class="form-input w-full text-sm mono readonly">`,
					html.EscapeString(display)))
			case isSecret:
				// Empty box, so the value is not in the page and typing means
				// replace. What it currently is goes in the placeholder.
				ph := "not set"
				if val != "" {
					ph = hint(val)
					if shown {
						ph = val
					}
				}
				b.WriteString(fmt.Sprintf(
					`<input type="text" name="%s" value="" placeholder="%s" autocomplete="off"
						class="form-input w-full text-sm mono">`,
					html.EscapeString(key), html.EscapeString(ph)))
				if val != "" {
					b.WriteString(fmt.Sprintf(
						`<label class="text-2xs text-muted"><input type="checkbox" name="clear_%s" value="1"> clear</label>`,
						html.EscapeString(key)))
				}
			default:
				b.WriteString(fmt.Sprintf(
					`<input type="text" name="%s" value="%s" placeholder="not set" autocomplete="off"
						class="form-input w-full text-sm mono">`,
					html.EscapeString(key), html.EscapeString(val)))
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<button type="submit" class="btn mb-4">Save</button>`)
	b.WriteString(`</form>`)

	b.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	app.Respond(w, r, app.Response{Title: "Settings", Description: "What this instance is configured with", HTML: b.String()})
}
