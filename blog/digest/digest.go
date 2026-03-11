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
	"mu/news"
	"mu/news/markets"
	"mu/news/reminder"
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

// Generate triggers digest generation. Returns false if already running.
func Generate() bool {
	mu.Lock()
	if running {
		mu.Unlock()
		return false
	}
	mu.Unlock()
	go generate()
	return true
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

	context, refs := gatherContext()
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
You will be given today's data from various sources: news headlines, market prices, videos, and a reminder.
Write a concise, well-structured digest summarising the key information. Use markdown formatting.

Structure:
1. A brief opening paragraph with the overall theme of the day
2. **News** - Summarise the top stories (3-5 bullet points with key takeaways)
3. **Markets** - Brief overview of notable price movements and trends
4. **Videos** - Mention any notable new content if available
5. **Reminder** - Include the reminder as its own section with a ## heading, followed by a blank line, then the content

Keep it informative but concise. Write in a neutral, clear tone. Do not invent information - only summarise what is provided.
Do NOT start with a title or top-level heading - the blog post title is set separately. Jump straight into the opening paragraph.
Do NOT include any preamble, meta-commentary, or introductory text like "Here is the digest". Output ONLY the digest content.
Do NOT include a references section - references will be appended separately.
The total length should be around 300-500 words.`,
		Question: context,
		Priority: ai.PriorityLow,
	}

	draft, err := ai.Ask(prompt)
	if err != nil {
		mu.Lock()
		lastStatus = "error"
		lastError = err.Error()
		mu.Unlock()
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	// Editorial review pass - vet the draft against sources
	reviewPrompt := &ai.Prompt{
		System: `You are a senior editor reviewing a daily digest before publication.
You have access to the original source material and the draft written by a journalist.

Review the draft for:
1. **Accuracy** - Does it faithfully represent the source data? Flag anything invented or misrepresented.
2. **Coherence** - Does it flow well? Is the opening paragraph a good summary of the day's theme?
3. **Completeness** - Are important items from the sources missing or underrepresented?
4. **Tone** - Is it neutral and clear? Flag any editorialising or sensationalism.
5. **Structure** - Are sections well-balanced? Is anything too long or too short?

Provide specific, actionable feedback as bullet points. Be concise and direct.
If the draft is good, say so briefly and suggest only minor tweaks if any.`,
		Question: fmt.Sprintf("## Source Material\n\n%s\n\n## Draft\n\n%s", context, draft),
		Priority: ai.PriorityLow,
	}

	feedback, err := ai.Ask(reviewPrompt)
	if err != nil {
		app.Log("digest", "Editorial review failed, using draft as-is: %v", err)
		feedback = ""
	}

	// Final pass - rewrite incorporating editorial feedback
	var response string
	if feedback != "" {
		finalPrompt := &ai.Prompt{
			System: `You are a writer producing the final version of a daily digest blog post.
You have: the original source data, your first draft, and editorial feedback from a senior editor.
Rewrite the digest incorporating the editor's suggestions. Follow the same structure and guidelines as before.

Structure:
1. A brief opening paragraph with the overall theme of the day
2. **News** - Summarise the top stories (3-5 bullet points with key takeaways)
3. **Markets** - Brief overview of notable price movements and trends
4. **Videos** - Mention any notable new content if available
5. **Reminder** - Include the reminder as its own section with a ## heading, followed by a blank line, then the content

Do NOT start with a title or top-level heading. Jump straight into the opening paragraph.
Do NOT include any preamble, meta-commentary, or introductory text like "Here is the revised digest". Output ONLY the digest content.
Do NOT include a references section - references will be appended separately.
The total length should be around 300-500 words.`,
			Question: fmt.Sprintf("## Source Material\n\n%s\n\n## First Draft\n\n%s\n\n## Editorial Feedback\n\n%s", context, draft, feedback),
			Priority: ai.PriorityLow,
		}

		response, err = ai.Ask(finalPrompt)
		if err != nil {
			app.Log("digest", "Final rewrite failed, using draft as-is: %v", err)
			response = draft
		}
	} else {
		response = draft
	}

	response = stripPreamble(response)
	response = normalizeHeadings(response)

	// Append references
	if len(refs) > 0 {
		var refSection strings.Builder
		refSection.WriteString("\n\n## References\n\n")
		for i, r := range refs {
			refSection.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, r.title, r.url))
		}
		response += refSection.String()
	}

	title := fmt.Sprintf("%s", time.Now().Format("2 January 2006"))

	err = blog.CreatePost(title, response, "micro", "micro", "digest", false)
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

type ref struct {
	title string
	url   string
}

func gatherContext() (string, []ref) {
	var sb strings.Builder
	var refs []ref

	// News - group by category so all topics are represented
	feed := news.GetFeed()
	if len(feed) > 0 {
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
				refs = append(refs, ref{item.Title, item.URL})
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
			refs = append(refs, ref{v.Title, v.URL})
			sb.WriteString(fmt.Sprintf("- **%s** by %s\n", v.Title, v.Channel))
		}
		sb.WriteString("\n")
	}

	// Reminder
	if rem := reminder.GetReminderData(); rem != nil {
		sb.WriteString("## Reminder\n\n")
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

	return sb.String(), refs
}

// normalizeHeadings ensures every markdown heading has a blank line after it.
func normalizeHeadings(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for i, line := range lines {
		out = append(out, line)
		if strings.HasPrefix(strings.TrimSpace(line), "#") && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next != "" && !strings.HasPrefix(next, "#") {
				out = append(out, "")
			}
		}
	}
	return strings.Join(out, "\n")
}

// stripPreamble removes AI meta-commentary lines from the start of the response.
// Lines like "Here is the revised digest:" are not part of the actual content.
func stripPreamble(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.SplitN(s, "\n", -1)
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[0])
		lower := strings.ToLower(line)
		if line == "" ||
			strings.HasPrefix(lower, "here is") ||
			strings.HasPrefix(lower, "here's") ||
			strings.HasPrefix(lower, "below is") ||
			strings.HasPrefix(lower, "i've") ||
			strings.HasPrefix(lower, "i have") ||
			strings.HasSuffix(lower, ":") && !strings.HasPrefix(line, "**") && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			lines = lines[1:]
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
