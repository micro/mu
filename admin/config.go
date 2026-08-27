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

// settingGroup is one capability and the values that switch it on.
//
// Does and Needs were added because the page answered the wrong question. It
// grouped 112 settings by vendor — AI, Twilio, S3 — which answers "what are my
// Twilio settings", and nobody has ever asked that. What an operator arrives
// wanting to know is "why doesn't SMS work", and the page could not say: a row
// was a name, a badge and a box, with nothing to explain what the name was for
// or whether the thing it belongs to was working.
type settingGroup struct {
	Name string
	// Does is what this capability is, in the words somebody would use for it.
	Does string
	// Needs is the values without which it does not work at all. Everything
	// else in Vars tunes it; these switch it on. Empty means the capability
	// works unconfigured and these only adjust it.
	Needs []string
	Vars  []string
}

// on reports whether a capability's required values are all present, and names
// whichever are not.
func (g settingGroup) on() (bool, []string) {
	var missing []string
	for _, k := range g.Needs {
		if strings.TrimSpace(settings.Get(k)) == "" {
			missing = append(missing, k)
		}
	}
	return len(missing) == 0, missing
}

var settingGroups = []settingGroup{
	{Name: "AI",
		Does:  "The model behind the agent, chat and summaries. Nothing that thinks works without one of these keys.",
		Needs: []string{"ANTHROPIC_API_KEY"},
		Vars: []string{
			"ANTHROPIC_API_KEY",
			// Which provider, when there are keys for more than one.
			"AI_PROVIDER",
			"ANTHROPIC_MODEL",
			// The agent's own model, which is the one running the tool loop.
			"AGENT_MODEL",
			"ATLAS_API_KEY",
			"ATLAS_MODEL",
			"OPENROUTER_API_KEY",
			"OPENROUTER_MODEL",
			"OPENAI_BASE_URL",
			"OPENAI_API_KEY",
			"OPENAI_MODEL",
			"IMAGE_MODEL",
		}},
	{Name: "Search",
		Does:  "Searching the web, and video and places lookups.",
		Needs: []string{"BRAVE_API_KEY"},
		Vars: []string{
			"BRAVE_API_KEY",
			"YOUTUBE_API_KEY",
			"GOOGLE_API_KEY",
		}},
	{Name: "The search index",
		Does:  "Where search looks. SQLite with FTS5, which is an index. Set to 0 for the old one: a map read end to end on every query.",
		Needs: nil,
		Vars: []string{
			// Here to be turned off, not on. Left visible rather than deleted
			// because it is the way back if the index misbehaves on somebody's
			// data, and a default nobody can undo is worse than a dial.
			"MU_USE_SQLITE",
		}},
	{Name: "Mail",
		Does:  "This instance as a mail server: the address people write to, and what it sends as.",
		Needs: []string{"MAIL_DOMAIN"},
		Vars: []string{
			"MAIL_DOMAIN",
			"MAIL_WHITELIST",
			"MAIL_PORT",
			"MAIL_SELECTOR",
			"DKIM_PRIVATE_KEY",
			// SMTP_HOST, SMTP_PORT, SMTP_USER and SMTP_PASS were here and
			// nothing read them: the relay is SMTP_RELAY_*, below. They were
			// the worst kind of dead setting, because they are exactly what
			// somebody would fill in to relay mail, and filling them in did
			// nothing at all.
			"SMTP_RELAY_HOST",
			"SMTP_RELAY_USER",
			"SMTP_RELAY_PASS",
			"IMAP_PUBLIC",
			"SUBMISSION_PUBLIC",
		}},
	// Object storage. Backups go here first, because a copy on the same disk
	// does not survive losing the disk — and later the same bucket is where
	// files and generated images belong, which is why these are named for the
	// storage rather than for the backup.
	{Name: "Object storage (S3)",
		Does:  "Where files and images are kept, instead of this machine's disk.",
		Needs: []string{"S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY"},
		Vars: []string{
			"S3_ENDPOINT",
			"S3_BUCKET",
			"S3_REGION",
			// internal/blob reads these two. They were in a group described as
			// "the older S3 names, kept so a configured instance keeps working —
			// set the group above instead", which was wrong in the way that
			// costs somebody something: they are not older names for the backup
			// credentials, they are a different subsystem's, and neither falls
			// back to the other. An operator who took the advice configured
			// backups and left file storage with no credentials at all.
			"S3_ACCESS_KEY",
			"S3_SECRET_KEY",
		}},
	{Name: "Backups",
		Does:  "A copy of this instance's data, sent to object storage on a schedule. Uses the endpoint, bucket and region above, with its own credentials.",
		Needs: []string{"BACKUP_S3", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"},
		Vars: []string{
			"BACKUP_S3",
			// internal/backup reads these. Separate credentials from the ones
			// above on purpose — a backup key that can only write is the point
			// of having two — and the names differ by so little that the page
			// has to say which is which.
			"S3_ACCESS_KEY_ID",
			"S3_SECRET_ACCESS_KEY",
			"S3_PREFIX",
		}}, {Name: "Payments",
		Does:  "Taking money: a card through Stripe, or per-request from an agent over x402.",
		Needs: nil,
		Vars: []string{
			"STRIPE_SECRET_KEY",
			"STRIPE_PUBLISHABLE_KEY",
			"STRIPE_WEBHOOK_SECRET",
			"X402_PAY_TO",
			"X402_NETWORK",
			"X402_VERSION",
			"X402_SERVERS",
		}},
	// The node this instance reads balances from. BASE_RPC_URL was readable by
	// the code and settable nowhere, so the only way to point it at Base was an
	// environment edit and a restart.
	{Name: "Chain",
		Does:  "The node balances are read from. Wrong or unset reports every wallet as empty.",
		Needs: nil,
		Vars: []string{
			"BASE_RPC_URL",
			// TRADE_RPC_URL and TRADE_CHAIN were here, from a trading feature
			// that no longer exists. TRADE_CHAIN's only reader was a "Trading"
			// health check returning ok unconditionally — a green light wired
			// to nothing. TRADE_RPC_URL was worse than dead: it was a live
			// fallback for this one, and a node set up for trading is an
			// Ethereum node, so it answered every Base balance with zero.
		}},
	{Name: "Transit",
		Does:  "Departure boards and live buses. Answers from published timetables with no key; these make it live.",
		Needs: nil,
		Vars: []string{
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
	// The basemap. /maps sends an operator here by name when it has no key,
	// so this group has to exist for that sentence to be true — see the note
	// on the Twilio group, which is the same mistake found the same way.
	{Name: "Maps",
		Does:  "The basemap tiles behind /maps.",
		Needs: []string{"OS_MAPS_KEY"},
		Vars: []string{
			"OS_MAPS_KEY",
			"TILE_FETCH_PER_HOUR",
		}},
	// SMS over Twilio. These were absent, and /sms sends an operator here by
	// name to set them — a page pointing at a page that could not help, which
	// is worse than no pointer at all.
	//
	// TWILIO_WHATSAPP_FROM was removed when WhatsApp went with Telegram, and is
	// back — not as a second service this time, but as a channel on this one.
	// It rides the same account, the same webhook and the same number rules;
	// the four things that differ are values. See service/sms/channel.go.
	{Name: "Twilio — SMS and WhatsApp",
		Does: "A phone number, so an agent can text somebody and read what they text back. " +
			"Set TWILIO_WHATSAPP_FROM as well and the same account carries WhatsApp, " +
			"on the same webhook.",
		Needs: []string{"TWILIO_ACCOUNT_SID", "TWILIO_AUTH_TOKEN", "TWILIO_FROM"},
		Vars: []string{
			"TWILIO_ACCOUNT_SID",
			"TWILIO_AUTH_TOKEN",
			"TWILIO_API_KEY",
			"TWILIO_API_SECRET",
			"TWILIO_FROM",
			"TWILIO_MESSAGING_SERVICE_SID",
			"TWILIO_WEBHOOK_URL",
			"SMS_DEFAULT_COUNTRY",
			"SMS_COUNTRIES",
			"SMS_KNOWN_ONLY",
			"SMS_VERIFY_INBOUND",
			"SMS_DAILY_LIMIT",
			"TWILIO_WHATSAPP_FROM",
			"WHATSAPP_DAILY_LIMIT",
		}},
	// "Sending limits and email out" was here, and four of its five settings
	// were read by nothing: EMAIL_DOMAIN and EMAIL_REPLY_DOMAIN, from before
	// the mailbox settled on MAIL_DOMAIN; EMAIL_DAILY_LIMIT, which
	// internal/quota names as one of "two names for one idea in two packages";
	// and WHATSAPP_DAILY_LIMIT, from the deleted channel. Its comment pointed
	// at service/email, which does not exist.
	//
	// The one live setting was SMS_DAILY_LIMIT, and it is now in the Twilio
	// group with the number it bounds. A ceiling on SMS filed away from the SMS
	// settings is how somebody turns SMS on, watches it stop at a hundred
	// messages, and finds nothing on the page that explains it.
	{Name: "Sign-in",
		Does:  "Signing in with Google, as well as with a password or a passkey.",
		Needs: []string{"GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET"},
		Vars: []string{
			"GOOGLE_CLIENT_ID",
			"GOOGLE_CLIENT_SECRET",
			"GOOGLE_REDIRECT_URI",
			// Passkeys are sign-in, and were under "Platform" — which is how
			// somebody looking for why a passkey will not register ends up
			// reading a list about browsers and shutdown timeouts.
			"PASSKEY_ORIGIN",
			"PASSKEY_RP_ID",
		}},
	{Name: "Social",
		Does:  "Posting out to the ATProto network.",
		Needs: nil,
		Vars: []string{
			"SOCIAL_ATPROTO",
		}},
	// The machine an account gets, and the shell door onto it. Every one of
	// these was readable by the code and settable nowhere — the fourth time
	// this file has recorded that sentence, which is why there is now a test
	// rather than another comment. See TestEverySettingIsSettable.
	// SHELL_*, and the service reads SANDBOX_* as a fallback so a running
	// instance keeps its settings across the rename — see service/shell/
	// setting.go. Only the new names are offered here: this page is where a
	// value is set, and offering both would invite an operator to set one of
	// each and then wonder which won.
	{Name: "Shell",
		Does:  "The machine each account gets: how big it may be, how long it may run, and the SSH door onto it.",
		Needs: nil,
		Vars: []string{
			"SHELL_IMAGE",
			"SHELL_MEMORY",
			"SHELL_CPUS",
			"SHELL_PIDS",
			"SHELL_NETWORK",
			"SHELL_SHARED",
			"SHELL_MAX_MACHINES",
			"SHELL_MAX_SECONDS",
			"SHELL_IDLE_MINUTES",
			// Read once at boot, so changing it here needs a restart before
			// anything listens. Shown anyway: an operator has to be able to see
			// what it is set to without shelling onto the box.
			"SHELL_SSH_PORT",
		}},
	// What this instance tells its operator about, and when. Added the same
	// day as the checks themselves and left off this page, which is the bug
	// this group exists to fix — see admin/alert.go.
	{Name: "Alerts",
		Does:  "What this instance tells you about, and how often it is allowed to.",
		Needs: nil,
		Vars: []string{
			"ALERTS",
			"ALERT_COOLDOWN_MINUTES",
			"ALERT_CALLS_PER_HOUR",
			"ALERT_ACCOUNT_CALLS_PER_HOUR",
			"ALERT_DISK_PERCENT",
		}},
	// "Limits and trial" was here. There is no trial — FREE_TURNS and
	// TRIAL_DAILY_TOTAL configured one this instance does not offer — and
	// GENERATE_ADULT switched on something it does not generate, which is a
	// policy and not a dial. The video ceilings that shared the group protect
	// a paid API quota and keep working from their defaults in
	// service/video/searchlimit.go, where the numbers can be read next to what
	// they bound.
	{Name: "Platform",
		Does:  "This instance itself: what it is called.",
		Needs: nil,
		Vars: []string{
			"MU_DOMAIN",
		}},
	// "The agent" was a group of three and is now a group of none. AGENT_NATIVE
	// and AGENT_NATIVE_STREAM chose between two agent loops, and there is one.
	// AGENT_MAX_STEPS was a cost ceiling whose useful range turned out to be a
	// single value — see maxSteps in agent/native.go.
	{Name: "Tools for other clients (MCP)",
		Does:  "The MCP door, and publishing this instance to the registry so other people can find it.",
		Needs: nil,
		Vars: []string{
			"MCP_GATEWAY_ADDR",
			// Proof of domain ownership for the MCP registry, served at
			// /.well-known/mcp-registry-auth. It was readable by the code and
			// settable nowhere, so publishing meant an environment edit and a
			// restart on a box somebody had to have shell on.
			"MCP_REGISTRY_PROOF",
		}},
	{Name: "Browser",
		Does:  "Reading a page the way a person would, for sites that need JavaScript to render.",
		Needs: nil,
		Vars: []string{
			"BROWSER_URL",
			"CHROME_PATH",
		}},
	// Not your notes. This group was called "Notes and the blog" and described
	// as "whether what you write is kept private or published", which is a
	// setting that does not exist and would not belong here if it did —
	// visibility is a property of a note, decided when it is written.
	//
	// What NOTES actually gates is a background loop in service/blog that
	// posts Mu's own story to Mu's own blog on a low cadence. Two unrelated
	// things are called notes in this repository — internal/notes is what you
	// and your agents write down, at /notes — and the label had picked the
	// wrong one.
	{Name: "The blog Mu writes about itself",
		Does:  "Mu posts about its own work to its own blog, occasionally. This is the off switch.",
		Needs: nil,
		Vars: []string{
			"NOTES",
		}}}

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

// ConfigHandler is /admin/config: what this instance is configured with, where each
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
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/admin/config?saved=1", http.StatusSeeOther)
		return
	}

	reveal := r.URL.Query().Get("reveal")

	var b strings.Builder

	if r.URL.Query().Get("saved") == "1" {
		b.WriteString(`<div class="card card-ok"><p class="text-success m-0">Saved. Anything read once at start-up needs a restart to take effect.</p></div>`)
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

	// What is off, at the top, named.
	//
	// A hundred and twelve settings in seventeen groups is a page nobody reads
	// to the bottom of, and the one thing worth knowing — which capabilities
	// are not working — was distributed across all of it one row at a time.
	// Counted here, and linked, so the answer is above the fold.
	var off []settingGroup
	for _, g := range settingGroups {
		if len(g.Needs) == 0 {
			continue
		}
		if ok, _ := g.on(); !ok {
			off = append(off, g)
		}
	}
	if len(off) > 0 {
		b.WriteString(`<div class="card"><h3>Not working yet</h3>`)
		b.WriteString(`<p class="text-sm text-secondary m-0 mb-2">` +
			`These need a value before they do anything. Everything not listed is ` +
			`either working or optional.</p>`)
		b.WriteString(`<ul class="text-sm m-0">`)
		for _, g := range off {
			_, missing := g.on()
			b.WriteString(fmt.Sprintf(`<li><b>%s</b> — %s<br><span class="text-muted">Needs %s</span></li>`,
				html.EscapeString(g.Name), html.EscapeString(g.Does),
				html.EscapeString(strings.Join(missing, ", "))))
		}
		b.WriteString(`</ul></div>`)
	}

	b.WriteString(`<form method="POST" action="/admin/config">`)

	for _, group := range settingGroups {
		b.WriteString(`<div class="card">`)
		b.WriteString(fmt.Sprintf(`<h3>%s</h3>`, html.EscapeString(group.Name)))

		// What this is for, before the values that configure it. A name and a
		// box asks somebody to already know what TWILIO_MESSAGING_SERVICE_SID
		// is for; a sentence lets them decide whether they care.
		if group.Does != "" {
			b.WriteString(fmt.Sprintf(`<p class="text-sm text-secondary m-0 mb-2">%s</p>`,
				html.EscapeString(group.Does)))
		}

		// And whether it works, which is the question the page is actually
		// opened to answer. "Why doesn't SMS work" was unanswerable here: every
		// row said env, saved or not set, and none of them said whether the
		// thing they belong to was on.
		if len(group.Needs) > 0 {
			if ok, missing := group.on(); ok {
				b.WriteString(`<p class="text-sm text-success m-0 mb-3">On.</p>`)
			} else {
				b.WriteString(fmt.Sprintf(
					`<p class="text-sm m-0 mb-3">Not working yet — needs %s.</p>`,
					html.EscapeString(strings.Join(missing, ", "))))
			}
		}

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

			// The row is addressable, which is what makes show and hide usable.
			//
			// Revealing is a round trip rather than a script — see below — and a
			// round trip on a page of a hundred settings lands you at the top,
			// hunting for the one you clicked. The anchor brings the browser back
			// to the row instead. Cheap, and it keeps the reason the round trip
			// exists.
			b.WriteString(`<div class="mb-3" id="` + html.EscapeString(key) + `">`)
			b.WriteString(fmt.Sprintf(
				`<label class="text-xs text-muted d-block mb-1"><code>%s</code> %s`,
				html.EscapeString(key), badge))
			// Revealing is a round trip rather than a script, so a secret is not
			// sitting in the page source of every visit waiting to be read.
			if isSecret && val != "" && !shown {
				b.WriteString(` <a href="/admin/config?reveal=` + url.QueryEscape(key) +
					`#` + url.QueryEscape(key) + `" class="text-2xs text-muted">show</a>`)
			} else if isSecret && shown {
				b.WriteString(` <a href="/admin/config#` + url.QueryEscape(key) +
					`" class="text-2xs text-muted">hide</a>`)
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

	b.WriteString(back())

	app.Respond(w, r, app.Response{Title: "Config", Description: "What this instance is configured with", HTML: b.String()})
}
