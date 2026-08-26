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
	var checks []healthCheck

	// AI Provider
	checks = append(checks, checkAI())

	// News Feed
	checks = append(checks, checkNews())

	// Markets
	checks = append(checks, checkMarkets())

	// Daily Digest
	checks = append(checks, checkDigest(testDigest))

	// Mail
	checks = append(checks, checkMail())

	// Federation
	checks = append(checks, checkFederation(testFederation))

	// Trading

	return checks
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
			Name:   "Federation",
			Status: "warning",
			Detail: "XMPP_S2S_PORT is off — this instance talks to nobody else",
			Fix:    "Set XMPP_S2S_PORT in /admin/config to accept federated connections",
		}
	}

	if !test {
		return healthCheck{
			Name:   "Federation",
			Status: "ok",
			Detail: "Listening on 5269. Whether the handshake works is a different question",
			Fix: "Not checked. " + app.Link("Dial "+federationPeer+" now",
				"/admin/diagnostics?test=federation"),
		}
	}

	detail, err := chat.CheckFederation(federationPeer)
	if err != nil {
		return healthCheck{
			Name:   "Federation",
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
		Name:   "Federation",
		Status: "ok",
		Detail: detail,
	}
}

func checkAI() healthCheck {
	if !ai.Configured() {
		return healthCheck{
			Name:   "AI Provider",
			Status: "error",
			Detail: "No AI provider configured",
			Fix:    "Set ANTHROPIC_API_KEY, ATLAS_API_KEY, OPENROUTER_API_KEY or OPENAI_BASE_URL in /admin/config, or install Ollama",
		}
	}

	provider := "Anthropic Claude"
	switch {
	case settings.Get("ANTHROPIC_API_KEY") != "":
		provider = "Anthropic Claude"
	case settings.Get("ATLAS_API_KEY") != "":
		provider = "Atlas Cloud"
	case settings.Get("OPENROUTER_API_KEY") != "":
		provider = "OpenRouter"
	case settings.Get("OPENAI_BASE_URL") != "":
		provider = "Local model (" + settings.Get("OPENAI_BASE_URL") + ")"
	default:
		provider = "Ollama (auto-detected)"
	}

	return healthCheck{
		Name:   "AI Provider",
		Status: "ok",
		Detail: provider,
	}
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

	latest := feed[0]
	age := time.Since(latest.PostedAt)
	if age > 24*time.Hour {
		return healthCheck{
			Name:   "News Feed",
			Status: "warning",
			Detail: fmt.Sprintf("%d articles, latest is %s old", len(feed), age.Round(time.Hour)),
			Fix:    "Feeds may not be updating — check RSS source availability",
		}
	}

	return healthCheck{
		Name:   "News Feed",
		Status: "ok",
		Detail: fmt.Sprintf("%d articles, latest: %s ago", len(feed), age.Round(time.Minute)),
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

	return healthCheck{
		Name:   "Markets",
		Status: "ok",
		Detail: fmt.Sprintf("%d assets tracked", len(data)),
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

	return healthCheck{
		Name:   "Mail",
		Status: "ok",
		Detail: domain,
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
