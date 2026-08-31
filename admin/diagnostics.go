package admin

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"mu/agent/digest"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
	"mu/service/chat"
	"mu/service/markets"
	"mu/service/news"
)

type healthCheck struct {
	Name   string
	Status string // "ok", "warning", "error"
	Detail string
	Fix    string // actionable suggestion
}

func DiagnosticsHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// The digest check can run the pipeline for real to find out why it is
	// stuck, which is a model call — asked for, never on the way past.
	test := r.URL.Query().Get("test")
	checks := runHealthChecks(test == "digest", test == "federation")

	// Count issues
	errors := 0
	warnings := 0
	for _, c := range checks {
		if c.Status == "error" {
			errors++
		} else if c.Status == "warning" {
			warnings++
		}
	}

	var b strings.Builder

	// Summary
	if errors == 0 && warnings == 0 {
		b.WriteString(`<div class="card edge good"><h3 class="text-success">All systems operational</h3></div>`)
	} else {
		color := "#f39c12"
		if errors > 0 {
			color = "#e74c3c"
		}
		b.WriteString(fmt.Sprintf(`<div class="card edge" style="border-left-color:%s"><h3 style="color:%s">%d issue(s) detected</h3></div>`, color, color, errors+warnings))
	}

	b.WriteString(renderChecks(checks))

	// AI Diagnosis button
	if errors > 0 || warnings > 0 {
		if r.URL.Query().Get("diagnose") == "1" {
			diagnosis := aiDiagnose(checks)
			b.WriteString(`<div class="card"><h3>AI Diagnosis</h3>`)
			b.WriteString(fmt.Sprintf(`<div class="text-base">%s</div>`, app.RenderString(diagnosis)))
			b.WriteString(`</div>`)
		} else {
			b.WriteString(`<div class="my-3"><a href="/admin/diagnostics?diagnose=1" class="btn">Run AI Diagnosis</a></div>`)
		}
	}

	b.WriteString(back())

	app.Respond(w, r, app.Response{Title: "Diagnostics", Description: "System health", HTML: b.String()})
}

// renderChecks is one card per check.
//
// Its own function so that what it does to a string can be tested. It was
// inline in the handler, which meant the only way to find out how it rendered
// an error was to cause one in production and read it — and the one error worth
// reading came out blank, because of this.
func renderChecks(checks []healthCheck) string {
	var b strings.Builder
	for _, c := range checks {
		icon := "✓"
		color := "#27ae60"
		if c.Status == "warning" {
			icon = "⚠"
			color = "#f39c12"
		} else if c.Status == "error" {
			icon = "✗"
			color = "#e74c3c"
		}

		b.WriteString(`<div class="card pad-row mb-2">`)
		b.WriteString(fmt.Sprintf(`<div class="d-flex between items-center">
			<strong>%s</strong>
			<span class="text-18" style="color:%s">%s</span>
		</div>`, html.EscapeString(c.Name), color, icon))
		// Escaped, because a Detail is plain text and the interesting ones are
		// error messages. The federation check reported `starttls refused:
		// <required>` and the browser rendered that as "starttls refused:" with
		// nothing after it — the element name, which was the entire diagnosis,
		// parsed as a tag and disappeared. A diagnostics page that silently
		// deletes the part of an error naming what went wrong is worse than one
		// that shows nothing, because it reads as the error being empty.
		//
		// Fix stays raw: it carries deliberate links, app.Link being how the
		// digest and federation checks offer to run themselves.
		b.WriteString(fmt.Sprintf(`<p class="text-sm text-secondary mt-1 m-0">%s</p>`,
			html.EscapeString(c.Detail)))
		if c.Fix != "" {
			b.WriteString(fmt.Sprintf(`<p class="text-xs mt-1 m-0" style="color:%s">→ %s</p>`, color, c.Fix))
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}

func runHealthChecks(testDigest, testFederation bool) []healthCheck {
	// Each of these answers "is this working", not "how many things are in it".
	// A count on its own reads as a claim about the product — "612 assets
	// tracked" on a page showing a dozen — when it is the size of a cache.
	return []healthCheck{
		checkAI(),
		checkNews(),
		checkMarkets(),
		checkDigest(testDigest),
		checkMail(),
		checkFederation(testFederation),
	}
}

// federationPeer is the server the live check dials.
//
// A real, large, long-running deployment somebody else operates, deliberately.
// Checking against another Mu would prove the two agree with each other, which
// is what a loopback test proves and it is not the question.
const federationPeer = "jabber.org"

// checkFederation reports whether the federated port is on, and — only when
// asked — completes a real handshake with somebody else's server.
//
// Behind a click for the same reason the digest test is: it dials out over the
// public internet and waits up to ten seconds on a domain that may be down,
// which is not something a page should do because you opened it.
//
// This exists because federation had no way to be checked short of an account,
// a client, and a person at the other end — three things that can each fail on
// their own and look exactly like federation failing. Dialback needs none of
// them, so the check does not either.
func checkFederation(test bool) healthCheck {
	if _, on := app.ListenAddr("XMPP_S2S_PORT", ":5269"); !on {
		return healthCheck{
			Name:   "XMPP federation",
			Status: "warning",
			Detail: "XMPP_S2S_PORT is off — this instance talks to nobody else",
			Fix:    "Set XMPP_S2S_PORT in /admin/config to accept federated connections",
		}
	}

	if !test {
		return healthCheck{
			Name:   "XMPP federation",
			Status: "ok",
			Detail: "Listening on 5269. Whether the handshake works is a different question",
			Fix: "Not checked. " + app.Link("Dial "+federationPeer+" now",
				"/admin/diagnostics?test=federation"),
		}
	}

	detail, err := chat.CheckFederation(federationPeer)
	if err != nil {
		return healthCheck{
			Name:   "XMPP federation",
			Status: "error",
			Detail: "Could not complete dialback with " + federationPeer + ": " + err.Error(),
			// Named in this order because it is the order they fail in, and the
			// last is the one that gets forgotten: dialback means dialling out
			// to every domain that dials in, so egress closed to 5269 fails
			// every handshake while looking like a broken peer.
			Fix: "Check that " + chat.Domain() + " resolves to this host from outside, that " +
				"5269 is open inbound, and that 5269 is open outbound",
		}
	}
	return healthCheck{
		Name:   "XMPP federation",
		Status: "ok",
		Detail: detail,
	}
}

func checkAI() healthCheck {
	if !ai.Configured() {
		return healthCheck{
			Name:   "AI model",
			Status: "error",
			Detail: "No AI provider configured",
			Fix:    "Set ANTHROPIC_API_KEY, ATLASCLOUD_API_KEY, GEMINI_API_KEY, OPENROUTER_API_KEY or OPENAI_BASE_URL in /admin/config, or install Ollama",
		}
	}

	// The model, not a guess at the provider from which keys happen to be set.
	//
	// This used to read the keys itself and say "Anthropic Claude" whenever an
	// Anthropic key existed — on an instance running DeepSeek, because
	// AI_PROVIDER is what decides and this never looked at it. A diagnostics
	// page confidently naming the wrong vendor is worse than one that says
	// nothing: it ends an investigation instead of starting one.
	//
	// Asking the same functions the runtime asks means the answer is the model
	// that will actually run. Both of them, because they differ on purpose and
	// the difference is the thing an operator gets surprised by — summaries and
	// moderation go to the cheap one, and it is not always the same vendor.
	provider := providerLabel()
	detail := provider + " · " + ai.DefaultModel()
	if bg := ai.BackgroundModel(); bg != ai.DefaultModel() {
		detail += ", and " + bg + " for summaries and moderation"
	}

	return healthCheck{
		Name:   "AI model",
		Status: "ok",
		Detail: detail,
	}
}

// providerLabel is who is actually answering.
//
// AI_PROVIDER first, because that is what the runtime honours. Only when it is
// unset or unreachable does which-key-is-present decide anything, and then in
// the same order the runtime falls back in.
func providerLabel() string {
	if p, _, base, ok := ai.PreferredProvider(); ok {
		switch p {
		case ai.ProviderAnthropic:
			return "Anthropic"
		case ai.ProviderAtlasCloud:
			return "Atlas Cloud"
		case ai.ProviderOpenRouter:
			return "OpenRouter"
		case ai.ProviderLocal:
			if base != "" {
				return "Local model at " + base
			}
			return "Local model"
		}
	}
	switch {
	case settings.Get("ANTHROPIC_API_KEY") != "":
		return "Anthropic"
	// Atlas's own two names. OPENAI_API_KEY was a third and is not one — it is
	// the key for whatever OpenAI-compatible endpoint is configured, so a local
	// Ollama install was diagnosed as running on Atlas Cloud. See getAtlasAPIKey.
	case settings.Get("ATLASCLOUD_API_KEY") != "" || settings.Get("ATLAS_API_KEY") != "":
		return "Atlas Cloud"
	case settings.Get("OPENROUTER_API_KEY") != "":
		return "OpenRouter"
	case settings.Get("OPENAI_BASE_URL") != "":
		return "Local model at " + settings.Get("OPENAI_BASE_URL")
	}
	return "Ollama, detected locally"
}

func checkNews() healthCheck {
	feed := news.GetFeed()
	if len(feed) == 0 {
		return healthCheck{
			Name:   "News Feed",
			Status: "warning",
			Detail: "No articles in feed",
			Fix:    "Check news/feeds.json for valid RSS feeds",
		}
	}

	// A count with the period it covers, because on its own it answers nothing.
	//
	// This said "412 articles" and left the reader to wonder over what — an
	// hour, a week, all time. The number is whatever is in the feed right now,
	// so the honest version says how far back that reaches. What makes it a
	// health check is the age of the newest one.
	newest, oldest := feed[0].PostedAt, feed[0].PostedAt
	for _, p := range feed {
		if p.PostedAt.After(newest) {
			newest = p.PostedAt
		}
		if p.PostedAt.Before(oldest) {
			oldest = p.PostedAt
		}
	}
	age := time.Since(newest)
	held := fmt.Sprintf("%d articles going back %s", len(feed), roughly(newest.Sub(oldest)))

	if age > 24*time.Hour {
		return healthCheck{
			Name:   "News",
			Status: "warning",
			Detail: held + ", but nothing new for " + roughly(age),
			Fix:    "Feeds may not be updating — check RSS source availability",
		}
	}

	return healthCheck{
		Name:   "News",
		Status: "ok",
		Detail: held + ", newest " + roughly(age) + " ago",
	}
}

// roughly is a duration a person would say out loud.
//
// time.Duration prints 74h12m30s, which is a number you have to do arithmetic
// on before it means anything — on a page whose whole job is to be read at a
// glance when something is wrong.
func roughly(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func checkMarkets() healthCheck {
	data := markets.AllPriceData()
	if len(data) == 0 {
		return healthCheck{
			Name:   "Markets",
			Status: "warning",
			Detail: "No price data available",
			Fix:    "Market data sources may be unreachable",
		}
	}

	// Prices, and when they last moved.
	//
	// This said "612 assets tracked", which is the size of a cache nobody sees
	// — the markets page shows a handful — so the number read as a claim about
	// the product rather than about a map. Naming it as prices held says what
	// it is, and the freshness is the part that can actually be wrong.
	//
	// It also reported ok for a price feed that had stopped hours earlier,
	// because the only failure it knew about was an empty cache. A stale cache
	// is the ordinary way this breaks: the fetch fails and the last good prices
	// sit there looking like current ones.
	var newest time.Time
	for _, pd := range data {
		if pd.UpdatedAt.After(newest) {
			newest = pd.UpdatedAt
		}
	}
	held := fmt.Sprintf("%d prices held", len(data))

	if newest.IsZero() {
		return healthCheck{
			Name:   "Markets",
			Status: "warning",
			Detail: held + ", none of them stamped with a time",
			Fix:    "Prices are cached but there is no way to tell how old they are",
		}
	}
	if age := time.Since(newest); age > 2*time.Hour {
		return healthCheck{
			Name:   "Markets",
			Status: "warning",
			Detail: held + ", but the newest is " + roughly(age) + " old",
			Fix:    "The price feed has stopped — what is on the markets page is stale",
		}
	}
	return healthCheck{
		Name:   "Markets",
		Status: "ok",
		Detail: held + ", newest " + roughly(time.Since(newest)) + " ago",
	}
}

// checkDigest reports whether the digest is current, and — only when asked —
// runs the pipeline for real to tell a broken generator from a stuck scheduler.
//
// The live test used to run whenever the digest was *not* ok, under a comment
// saying "if requested", which nothing did. That is a whole model generation on
// the render path, and the one condition that triggers it is exactly the
// condition an operator opens this page under. A page that costs a digest to
// look at, and takes as long as one, is how /admin/diagnostics came to be
// something you avoided loading.
func checkDigest(test bool) healthCheck {
	ok, details := digest.Status()
	if ok {
		return healthCheck{
			Name:   "Daily Digest",
			Status: "ok",
			Detail: details,
		}
	}

	fix := "Check AI provider status. " + app.Link("Test the generator", "/admin/diagnostics?test=digest")
	if test {
		out, err := digest.TestGenerate()
		switch {
		case err != nil:
			fix = "Test failed: " + err.Error()
		case out == "":
			fix = "Test returned empty"
		default:
			fix = fmt.Sprintf("Test succeeded (%d chars) — the pipeline works, the scheduler may be stuck", len(out))
		}
	}
	return healthCheck{
		Name:   "Daily Digest",
		Status: "error",
		Detail: details,
		Fix:    fix,
	}
}

func checkMail() healthCheck {
	domain := settings.Get("MAIL_DOMAIN")
	if domain == "" {
		return healthCheck{
			Name:   "Mail",
			Status: "warning",
			Detail: "Not configured",
			Fix:    "Set MAIL_DOMAIN in /admin/config",
		}
	}

	// What the domain means, not just the domain. A bare "micro.mu" under a
	// heading that says Mail is a fact with no claim attached — the reader has
	// to supply "…is the domain mail is accepted for" themselves.
	return healthCheck{
		Name:   "Mail",
		Status: "ok",
		Detail: "Accepting mail for " + domain,
	}
}

func aiDiagnose(checks []healthCheck) string {
	var issues strings.Builder
	issues.WriteString("System health check results:\n\n")
	for _, c := range checks {
		issues.WriteString(fmt.Sprintf("- %s: %s — %s", c.Name, c.Status, c.Detail))
		if c.Fix != "" {
			issues.WriteString(fmt.Sprintf(" (suggested fix: %s)", c.Fix))
		}
		issues.WriteString("\n")
	}

	result, err := ai.Ask(&ai.Prompt{
		System: `You are a system administrator for Mu, a personal AI platform written in Go.
Analyse the health check results and provide a brief diagnosis:
1. What's likely causing any failures
2. The most important thing to fix first
3. Any connections between issues (e.g. if AI is down, digest will also fail)
Keep it to 3-5 sentences. Be specific and actionable.`,
		Question: issues.String(),
		Model:    ai.BackgroundModel(),
		Priority: ai.PriorityHigh,
		Caller:   "system-diagnosis",
	})
	if err != nil {
		return "Could not run AI diagnosis: " + err.Error()
	}
	return result
}
