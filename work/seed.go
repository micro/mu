package work

import (
	"time"

	"github.com/google/uuid"
)

// seedPosts creates initial work posts
func seedPosts() {
	seeds := []struct {
		kind        string
		title       string
		description string
		link        string
		bounty      int
		tags        string
	}{
		// Tasks — apps that should be built
		{
			kind:        KindTask,
			title:       "Build a Pomodoro Timer App",
			description: "Build a Pomodoro timer app for Mu. 25-minute work sessions with 5-minute breaks. Should show a countdown, play a sound on completion, and track how many sessions completed today. Clean, minimal UI. Deliver as a Mu app.",
			bounty:      500,
			tags:        "app,productivity",
		},
		{
			kind:        KindTask,
			title:       "Build a Currency Converter App",
			description: "Build a currency converter app for Mu. Should support major currencies (GBP, USD, EUR, JPY, etc.) with live exchange rates. Simple interface: pick two currencies, enter an amount, see the result. Use a free exchange rate API. Deliver as a Mu app.",
			bounty:      750,
			tags:        "app,finance",
		},
		{
			kind:        KindTask,
			title:       "Build a Markdown Editor App",
			description: "Build a markdown editor app for Mu with live preview. Split pane: edit on the left, rendered preview on the right. Support common markdown: headings, bold, italic, links, code blocks, lists. Include a toolbar for common formatting. Deliver as a Mu app.",
			bounty:      1000,
			tags:        "app,writing,developer",
		},
		{
			kind:        KindTask,
			title:       "Build a QR Code Generator App",
			description: "Build a QR code generator app for Mu. Enter text or a URL, generate a QR code. Should support downloading the QR code as PNG. No external API required — use a JavaScript QR library. Deliver as a Mu app.",
			bounty:      500,
			tags:        "app,utility",
		},
		{
			kind:        KindTask,
			title:       "Build a JSON Formatter App",
			description: "Build a JSON formatter and validator app for Mu. Paste JSON, format it with proper indentation, validate syntax, and highlight errors. Support minify and prettify. Deliver as a Mu app.",
			bounty:      500,
			tags:        "app,developer",
		},
		{
			kind:        KindTask,
			title:       "Build a Budget Tracker App",
			description: "Build a simple budget tracker app for Mu. Add income and expenses with categories. Show a running balance and breakdown by category. Data stored locally in the browser. Clean, minimal interface. Deliver as a Mu app.",
			bounty:      1000,
			tags:        "app,finance",
		},
		{
			kind:        KindTask,
			title:       "Build an Invoice Generator App",
			description: "Build an invoice generator app for Mu. Fill in business details, client info, line items with quantities and prices. Calculate totals with optional tax. Export as printable HTML or downloadable PDF. Deliver as a Mu app.",
			bounty:      1500,
			tags:        "app,finance,business",
		},
		{
			kind:        KindTask,
			title:       "Build a Regex Tester App",
			description: "Build a regex tester app for Mu. Enter a regex pattern and test string, highlight matches in real-time. Show capture groups. Include a quick reference for common patterns. Deliver as a Mu app.",
			bounty:      500,
			tags:        "app,developer",
		},

		// Showcases — examples of what sharing looks like
		{
			kind:        KindShow,
			title:       "Mu — Apps Without Ads",
			description: "Built a platform that brings together the daily tools people use — news, search, chat, video, email, markets — in one place with zero ads and zero tracking. Single Go binary, self-hostable, open source. AI agents can access everything via MCP with x402 crypto payments.",
			link:        "https://mu.xyz",
			tags:        "platform,go,open-source",
		},
		{
			kind:        KindShow,
			title:       "x402 — HTTP Payments Protocol",
			description: "The x402 protocol adds native payment semantics to HTTP. When a resource requires payment, the server returns 402 with payment requirements. The client pays on-chain and retries. No accounts, no API keys. Designed for AI agents paying for API access.",
			link:        "https://x402.org",
			tags:        "protocol,crypto,payments",
		},
	}

	now := time.Now()

	mutex.Lock()
	for i, s := range seeds {
		post := &Post{
			ID:          uuid.New().String(),
			Kind:        s.kind,
			Title:       s.title,
			Description: s.description,
			Link:        s.link,
			Bounty:      s.bounty,
			AuthorID:    "micro",
			Author:      "Micro",
			Tags:        s.tags,
			Feedback:    []Comment{},
			CreatedAt:   now.Add(-time.Duration(i) * time.Minute),
		}
		if s.kind == KindTask {
			post.Status = StatusOpen
		}
		posts[post.ID] = post
	}
	save()
	mutex.Unlock()
}
