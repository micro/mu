package work

import (
	"time"

	"github.com/google/uuid"
)

// seedTasks creates initial bounty tasks for apps that should be built on Mu
func seedTasks() {
	seeds := []struct {
		title       string
		description string
		bounty      int
		tags        string
	}{
		{
			title:  "Build a Pomodoro Timer App",
			description: "Build a Pomodoro timer app for Mu. 25-minute work sessions with 5-minute breaks. Should show a countdown, play a sound on completion, and track how many sessions completed today. Clean, minimal UI. Deliver as a Mu app.",
			bounty: 500,
			tags:   "app,productivity",
		},
		{
			title:  "Build a Currency Converter App",
			description: "Build a currency converter app for Mu. Should support major currencies (GBP, USD, EUR, JPY, etc.) with live exchange rates. Simple interface: pick two currencies, enter an amount, see the result. Use a free exchange rate API. Deliver as a Mu app.",
			bounty: 750,
			tags:   "app,finance",
		},
		{
			title:  "Build a Markdown Editor App",
			description: "Build a markdown editor app for Mu with live preview. Split pane: edit on the left, rendered preview on the right. Support common markdown: headings, bold, italic, links, code blocks, lists. Include a toolbar for common formatting. Deliver as a Mu app.",
			bounty: 1000,
			tags:   "app,writing,developer",
		},
		{
			title:  "Build a QR Code Generator App",
			description: "Build a QR code generator app for Mu. Enter text or a URL, generate a QR code. Should support downloading the QR code as PNG. No external API required — use a JavaScript QR library. Deliver as a Mu app.",
			bounty: 500,
			tags:   "app,utility",
		},
		{
			title:  "Build a Colour Palette Generator App",
			description: "Build a colour palette generator app for Mu. Generate harmonious colour palettes (complementary, analogous, triadic). Click to copy hex values. Show colour swatches with hex and RGB values. Optional: generate from a base colour. Deliver as a Mu app.",
			bounty: 750,
			tags:   "app,design",
		},
		{
			title:  "Build a JSON Formatter App",
			description: "Build a JSON formatter and validator app for Mu. Paste JSON, format it with proper indentation, validate syntax, and highlight errors. Support minify and prettify. Deliver as a Mu app.",
			bounty: 500,
			tags:   "app,developer",
		},
		{
			title:  "Build a Budget Tracker App",
			description: "Build a simple budget tracker app for Mu. Add income and expenses with categories. Show a running balance and breakdown by category. Data stored locally in the browser. Clean, minimal interface. Deliver as a Mu app.",
			bounty: 1000,
			tags:   "app,finance",
		},
		{
			title:  "Build a Workout Log App",
			description: "Build a workout log app for Mu. Log exercises with sets, reps, and weight. View history by date. Track personal bests. Simple interface — not a full fitness platform, just a clean log. Deliver as a Mu app.",
			bounty: 750,
			tags:   "app,health",
		},
		{
			title:  "Build a Regex Tester App",
			description: "Build a regex tester app for Mu. Enter a regex pattern and test string, highlight matches in real-time. Show capture groups. Include a quick reference for common patterns. Deliver as a Mu app.",
			bounty: 500,
			tags:   "app,developer",
		},
		{
			title:  "Build a World Clock App",
			description: "Build a world clock app for Mu. Show current time in multiple timezones. Add and remove cities. Show the time difference from your local timezone. Clean, minimal display. Deliver as a Mu app.",
			bounty: 500,
			tags:   "app,utility",
		},
		{
			title:  "Build an Invoice Generator App",
			description: "Build an invoice generator app for Mu. Fill in business details, client info, line items with quantities and prices. Calculate totals with optional tax. Export as printable HTML or downloadable PDF. Deliver as a Mu app.",
			bounty: 1500,
			tags:   "app,finance,business",
		},
		{
			title:  "Build a Recipe Scaler App",
			description: "Build a recipe scaler app for Mu. Paste a recipe with ingredients, set the original serving size, choose a new serving size, and see all ingredient quantities recalculated. Handle fractions nicely. Deliver as a Mu app.",
			bounty: 500,
			tags:   "app,food",
		},
	}

	now := time.Now()

	mutex.Lock()
	for i, s := range seeds {
		task := &Task{
			ID:          uuid.New().String(),
			Title:       s.title,
			Description: s.description,
			Bounty:      s.bounty,
			PosterID:    "mu",
			Poster:      "mu",
			Status:      StatusOpen,
			Tags:        s.tags,
			CreatedAt:   now.Add(-time.Duration(i) * time.Minute), // stagger creation times
		}
		tasks[task.ID] = task
	}
	save()
	mutex.Unlock()
}
