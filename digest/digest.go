package digest

import (
	"fmt"
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
	mu         sync.Mutex
	running    bool
	lastDigest time.Time
	lastError  string
	lastStatus string // "ok", "error", "running", "pending"
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

	if lastDigest.IsZero() {
		lastStatus = "pending"
	} else {
		lastStatus = "ok"
	}

	go scheduler()
}

// Status returns the current digest state for the status page
func Status() (ok bool, details string) {
	mu.Lock()
	defer mu.Unlock()

	switch lastStatus {
	case "running":
		return true, "Generating..."
	case "error":
		if lastDigest.IsZero() {
			return false, fmt.Sprintf("Failed: %s", lastError)
		}
		return false, fmt.Sprintf("Failed: %s (last success: %s ago)", lastError, time.Since(lastDigest).Round(time.Minute))
	case "ok":
		ago := time.Since(lastDigest).Round(time.Minute)
		return true, fmt.Sprintf("Last: %s (%s ago)", lastDigest.Format("2 Jan 15:04"), ago)
	default:
		return false, "Never run"
	}
}

func scheduler() {
	// If no digest exists for today, generate immediately
	if !sameDay(lastDigest, time.Now()) {
		generate()
	}

	// Then check every hour
	for {
		time.Sleep(time.Hour)
		if !sameDay(lastDigest, time.Now()) {
			generate()
		}
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

	mu.Lock()
	lastStatus = "running"
	mu.Unlock()

	app.Log("digest", "Generating daily digest")

	context := gatherContext()
	if context == "" {
		mu.Lock()
		lastStatus = "error"
		lastError = "no content available"
		mu.Unlock()
		app.Log("digest", "No content available for digest")
		return
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
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		mu.Lock()
		lastStatus = "error"
		lastError = err.Error()
		mu.Unlock()
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	today := time.Now().Format("2 January 2006")
	title := fmt.Sprintf("Daily Digest - %s", today)

	err = blog.CreatePost(title, response, "mu", "mu", "digest", false)
	if err != nil {
		mu.Lock()
		lastStatus = "error"
		lastError = err.Error()
		mu.Unlock()
		app.Log("digest", "Failed to create blog post: %v", err)
		return
	}

	mu.Lock()
	lastDigest = time.Now()
	lastStatus = "ok"
	lastError = ""
	mu.Unlock()

	data.SaveFile("digest_last.txt", lastDigest.Format(time.RFC3339))
	app.Log("digest", "Daily digest published: %s", title)
}

func gatherContext() string {
	var sb strings.Builder

	// News - group by category so all topics are represented
	feed := news.GetFeed()
	if len(feed) > 0 {
		sb.WriteString("## Today's News\n\n")
		byCategory := make(map[string][]*news.Post)
		for _, item := range feed {
			byCategory[item.Category] = append(byCategory[item.Category], item)
		}
		for category, items := range byCategory {
			sb.WriteString(fmt.Sprintf("### %s\n", category))
			count := 3
			if len(items) < count {
				count = len(items)
			}
			for _, item := range items[:count] {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", item.Title, item.Description))
			}
			sb.WriteString("\n")
		}
	}

	// Markets - group by category
	priceData := markets.GetAllPriceData()
	if len(priceData) > 0 {
		sb.WriteString("## Market Prices\n\n")
		categories := []struct {
			name   string
			assets []string
		}{
			{"Crypto", []string{"BTC", "ETH", "SOL", "PAXG"}},
			{"Futures", []string{"OIL", "GOLD", "SILVER", "COPPER"}},
			{"Commodities", []string{"COFFEE", "WHEAT", "CORN"}},
			{"Currencies", []string{"EUR", "GBP", "JPY", "CNY"}},
		}
		for _, cat := range categories {
			var lines []string
			for _, symbol := range cat.assets {
				if pd, ok := priceData[symbol]; ok && pd.Price > 0 {
					change := ""
					if pd.Change24h != 0 {
						change = fmt.Sprintf(" (%+.1f%%)", pd.Change24h)
					}
					lines = append(lines, fmt.Sprintf("- %s: $%.2f%s", symbol, pd.Price, change))
				}
			}
			if len(lines) > 0 {
				sb.WriteString(fmt.Sprintf("### %s\n", cat.name))
				for _, l := range lines {
					sb.WriteString(l + "\n")
				}
				sb.WriteString("\n")
			}
		}
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
