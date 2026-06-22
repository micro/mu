package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/client/discord"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
	"mu/markets"
	"mu/news"
	"mu/news/digest"
	"mu/client/telegram"
	"mu/client/whatsapp"
)

type healthCheck struct {
	Name    string
	Status  string // "ok", "warning", "error"
	Detail  string
	Fix     string // actionable suggestion
}

func DiagnosticsHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	checks := runHealthChecks()

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
		b.WriteString(`<div class="card" style="border-left:4px solid #27ae60"><h3 style="color:#27ae60">All systems operational</h3></div>`)
	} else {
		color := "#f39c12"
		if errors > 0 {
			color = "#e74c3c"
		}
		b.WriteString(fmt.Sprintf(`<div class="card" style="border-left:4px solid %s"><h3 style="color:%s">%d issue(s) detected</h3></div>`, color, color, errors+warnings))
	}

	// Individual checks
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

		b.WriteString(`<div class="card" style="padding:12px 16px;margin-bottom:8px">`)
		b.WriteString(fmt.Sprintf(`<div style="display:flex;justify-content:space-between;align-items:center">
			<strong>%s</strong>
			<span style="color:%s;font-size:18px">%s</span>
		</div>`, c.Name, color, icon))
		b.WriteString(fmt.Sprintf(`<p style="font-size:13px;color:#666;margin:4px 0 0">%s</p>`, c.Detail))
		if c.Fix != "" {
			b.WriteString(fmt.Sprintf(`<p style="font-size:12px;color:%s;margin:4px 0 0">→ %s</p>`, color, c.Fix))
		}
		b.WriteString(`</div>`)
	}

	// AI Diagnosis button
	if errors > 0 || warnings > 0 {
		if r.URL.Query().Get("diagnose") == "1" {
			diagnosis := aiDiagnose(checks)
			b.WriteString(`<div class="card"><h3>AI Diagnosis</h3>`)
			b.WriteString(fmt.Sprintf(`<div style="font-size:14px;line-height:1.6">%s</div>`, app.RenderString(diagnosis)))
			b.WriteString(`</div>`)
		} else {
			b.WriteString(`<div style="margin:12px 0"><a href="/admin/diagnostics?diagnose=1" class="btn">Run AI Diagnosis</a></div>`)
		}
	}

	b.WriteString(`<p style="margin-top:12px"><a href="/admin">← Back to Admin</a></p>`)

	html := app.RenderHTMLForRequest("Diagnostics", "System health", b.String(), r)
	w.Write([]byte(html))
}

func runHealthChecks() []healthCheck {
	var checks []healthCheck

	// AI Provider
	checks = append(checks, checkAI())

	// News Feed
	checks = append(checks, checkNews())

	// Markets
	checks = append(checks, checkMarkets())

	// Daily Digest
	checks = append(checks, checkDigest())

	// Discord Bot
	checks = append(checks, checkDiscord())

	// Telegram Bot
	checks = append(checks, checkTelegram())

	// WhatsApp Bot
	checks = append(checks, checkWhatsApp())

	// Mail
	checks = append(checks, checkMail())

	// Trading
	checks = append(checks, checkTrading())

	return checks
}

func checkAI() healthCheck {
	key := settings.Get("ANTHROPIC_API_KEY")
	localURL := settings.Get("OPENAI_BASE_URL")

	if key == "" && localURL == "" && !ai.LocalModelAvailable() {
		return healthCheck{
			Name:   "AI Provider",
			Status: "error",
			Detail: "No AI provider configured",
			Fix:    "Set ANTHROPIC_API_KEY or OPENAI_BASE_URL in /admin/env, or install Ollama",
		}
	}

	provider := "Anthropic Claude"
	if key == "" {
		if localURL != "" {
			provider = "Local model (" + localURL + ")"
		} else {
			provider = "Ollama (auto-detected)"
		}
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
	data := markets.GetAllPriceData()
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

func checkDigest() healthCheck {
	ok, details := digest.Status()

	// If requested, run a live test
	testResult := ""
	if !ok {
		test, err := digest.TestGenerate()
		if err != nil {
			testResult = "Test failed: " + err.Error()
		} else if test == "" {
			testResult = "Test returned empty"
		} else {
			testResult = fmt.Sprintf("Test succeeded (%d chars) — the pipeline works, the scheduler may be stuck", len(test))
		}
	}

	if !ok {
		fix := "Check AI provider status"
		if testResult != "" {
			fix = testResult
		}
		return healthCheck{
			Name:   "Daily Digest",
			Status: "error",
			Detail: details,
			Fix:    fix,
		}
	}

	return healthCheck{
		Name:   "Daily Digest",
		Status: "ok",
		Detail: details,
	}
}

func checkDiscord() healthCheck {
	if !discord.Enabled() {
		return healthCheck{
			Name:   "Discord Bot",
			Status: "warning",
			Detail: "Not configured",
			Fix:    "Set DISCORD_BOT_TOKEN in /admin/env",
		}
	}

	return healthCheck{
		Name:   "Discord Bot",
		Status: "ok",
		Detail: "Connected",
	}
}

func checkTelegram() healthCheck {
	if !telegram.Enabled() {
		return healthCheck{
			Name:   "Telegram Bot",
			Status: "warning",
			Detail: "Not configured",
			Fix:    "Set TELEGRAM_BOT_TOKEN in /admin/env",
		}
	}

	return healthCheck{
		Name:   "Telegram Bot",
		Status: "ok",
		Detail: "Connected",
	}
}

func checkWhatsApp() healthCheck {
	if !whatsapp.Enabled() {
		return healthCheck{
			Name:   "WhatsApp Bot",
			Status: "warning",
			Detail: "Not configured",
			Fix:    "Set WHATSAPP_TOKEN and WHATSAPP_PHONE_ID in /admin/env",
		}
	}

	return healthCheck{
		Name:   "WhatsApp Bot",
		Status: "ok",
		Detail: "Configured",
	}
}

func checkMail() healthCheck {
	domain := settings.Get("MAIL_DOMAIN")
	if domain == "" {
		return healthCheck{
			Name:   "Mail",
			Status: "warning",
			Detail: "Not configured",
			Fix:    "Set MAIL_DOMAIN in /admin/env",
		}
	}

	return healthCheck{
		Name:   "Mail",
		Status: "ok",
		Detail: domain,
	}
}

func checkTrading() healthCheck {
	rpc := settings.Get("TRADE_RPC_URL")
	chain := settings.Get("TRADE_CHAIN")
	if chain == "" {
		chain = "ethereum"
	}

	detail := fmt.Sprintf("%s chain", chain)
	if rpc != "" {
		detail += " (custom RPC)"
	} else {
		detail += " (public RPC)"
	}

	return healthCheck{
		Name:   "Trading",
		Status: "ok",
		Detail: detail,
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
