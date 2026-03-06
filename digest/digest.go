package digest

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"mu/ai"
	"mu/app"
	"mu/blog"
	"mu/data"
	"mu/markets"
	"mu/news"
	"mu/reminder"
	"mu/video"
)

var (
	mu        sync.Mutex
	running   bool
	lastDigest time.Time
)

// Load starts the daily digest scheduler
func Load() {
	// Check when last digest was created
	if b, err := data.LoadFile("digest_last.txt"); err == nil {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
		if err == nil {
			lastDigest = t
		}
	}

	go scheduler()
}

func scheduler() {
	// Check every hour if we need to generate a digest
	for {
		now := time.Now()

		// Generate at 6am if we haven't today
		if now.Hour() >= 6 && !sameDay(lastDigest, now) {
			generate()
		}

		// Sleep until next check
		time.Sleep(time.Hour)
	}
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func generate() {
	mu.Lock()
	if running {
		mu.Unlock()
		return
	}
	running = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	app.Log("digest", "Generating daily digest")

	context := gatherContext()
	if context == "" {
		app.Log("digest", "No content available for digest")
		return
	}

	model := os.Getenv("ANTHROPIC_PREMIUM_MODEL")
	if model == "" {
		model = "claude-sonnet-4-5-20250514"
	}

	prompt := &ai.Prompt{
		System: `You are a writer producing a daily digest blog post.
You will be given today's data from various sources: news headlines, market prices, videos, and an Islamic reminder.
Write a concise, well-structured digest summarising the key information. Use markdown formatting.

Structure:
1. A brief opening paragraph with the overall theme of the day
2. **News** - Summarise the top stories (3-5 bullet points with key takeaways)
3. **Markets** - Brief overview of notable price movements and trends
4. **Videos** - Mention any notable new content if available
5. **Reminder** - Include the Islamic reminder naturally

Keep it informative but concise. Write in a neutral, clear tone. Do not invent information - only summarise what is provided.
The total length should be around 300-500 words.`,
		Question: context,
		Priority: ai.PriorityLow,
		Provider: ai.ProviderAnthropic,
		Model:    model,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	today := time.Now().Format("2 January 2006")
	title := fmt.Sprintf("Daily Digest - %s", today)

	err = blog.CreatePost(title, response, "mu", "mu", "digest", false)
	if err != nil {
		app.Log("digest", "Failed to create blog post: %v", err)
		return
	}

	lastDigest = time.Now()
	data.SaveFile("digest_last.txt", lastDigest.Format(time.RFC3339))
	app.Log("digest", "Daily digest published: %s", title)
}

func gatherContext() string {
	var sb strings.Builder

	// News
	feed := news.GetFeed()
	if len(feed) > 0 {
		sb.WriteString("## Today's News\n\n")
		count := 10
		if len(feed) < count {
			count = len(feed)
		}
		for _, item := range feed[:count] {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", item.Title, item.Category, item.Description))
		}
		sb.WriteString("\n")
	}

	// Markets
	priceData := markets.GetAllPriceData()
	if len(priceData) > 0 {
		sb.WriteString("## Market Prices\n\n")
		for symbol, pd := range priceData {
			change := ""
			if pd.Change24h != 0 {
				change = fmt.Sprintf(" (24h: %+.1f%%)", pd.Change24h)
			}
			sb.WriteString(fmt.Sprintf("- %s: $%.2f%s\n", symbol, pd.Price, change))
		}
		sb.WriteString("\n")
	}

	// Videos
	videos := video.GetLatestVideos(5)
	if len(videos) > 0 {
		sb.WriteString("## Latest Videos\n\n")
		for _, v := range videos {
			sb.WriteString(fmt.Sprintf("- **%s** by %s\n", v.Title, v.Channel))
		}
		sb.WriteString("\n")
	}

	// Reminder
	if rem := reminder.GetReminderData(); rem != nil {
		sb.WriteString("## Islamic Reminder\n\n")
		if rem.Name != "" {
			sb.WriteString(fmt.Sprintf("**Name of Allah:** %s\n\n", rem.Name))
		}
		if rem.Verse != "" {
			sb.WriteString(fmt.Sprintf("**Verse:** %s\n\n", rem.Verse))
		}
		if rem.Hadith != "" {
			sb.WriteString(fmt.Sprintf("**Hadith:** %s\n\n", rem.Hadith))
		}
	}

	return sb.String()
}
