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
	// Run immediately on startup
	generate()

	// Then run every hour
	for {
		time.Sleep(time.Hour)
		generate()
	}
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

	// Check if today's digest post already exists
	existing := blog.FindTodayDigest()

	if existing == nil {
		createDigest()
	} else {
		updateDigest(existing)
	}
}

// createDigest generates and publishes a new digest post for today.
func createDigest() {
	app.Log("digest", "Creating new daily digest")

	context, refs := gatherContext()
	if context == "" {
		setError("no content available")
		app.Log("digest", "No content available for digest")
		return
	}

	prompt := &ai.Prompt{
		System: digestSystemPrompt,
		Question: context,
		Priority: ai.PriorityLow,
	}

	draft, err := ai.Ask(prompt)
	if err != nil {
		setError(err.Error())
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	response := cleanResponse(draft)
	response += buildReferences(refs)

	title := time.Now().Format("2 January 2006")

	err = blog.CreatePost(title, response, "micro", "micro", "digest", false)
	if err != nil {
		setError(err.Error())
		app.Log("digest", "Failed to create blog post: %v", err)
		return
	}

	setSuccess()
	app.Log("digest", "Daily digest published: %s", title)
}

// updateDigest refreshes the main digest post with full 24-hour coverage,
// then optionally adds a comment highlighting only significant changes
// from the last hour (if any).
func updateDigest(post *blog.Post) {
	app.Log("digest", "Updating digest %s with full 24-hour coverage", post.ID)

	context, refs := gatherContext()
	if context == "" {
		app.Log("digest", "No content available for update")
		setSuccess()
		return
	}

	// Step 1: Regenerate the full digest post with current data
	prompt := &ai.Prompt{
		System:   digestSystemPrompt,
		Question: context,
		Priority: ai.PriorityLow,
	}

	draft, err := ai.Ask(prompt)
	if err != nil {
		setError(err.Error())
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	response := cleanResponse(draft)
	response += buildReferences(refs)

	// Update the existing post in place
	err = blog.UpdatePost(post.ID, post.Title, response, post.Tags, post.Private)
	if err != nil {
		setError(err.Error())
		app.Log("digest", "Failed to update digest post: %v", err)
		return
	}

	app.Log("digest", "Digest post %s updated with latest data", post.ID)

	// Step 2: Add a comment only if there are significant changes
	// compared to what was already covered in previous comments
	addHourlyComment(post, context)

	setSuccess()
}

// addHourlyComment adds a comment highlighting only significant new developments
// from the last hour. It skips if changes are minimal or already covered.
func addHourlyComment(post *blog.Post, currentContext string) {
	// Build context from previous comments to avoid repetition
	comments := blog.GetComments(post.ID)
	var priorUpdates strings.Builder
	for _, c := range comments {
		priorUpdates.WriteString(c.Content)
		priorUpdates.WriteString("\n\n")
	}

	prompt := &ai.Prompt{
		System: `You are a live blog updater. You will be given:
1. Previous hourly update comments (if any)
2. The latest data from news, markets, videos

Your job: identify ONLY significant developments from the LAST HOUR that are NOT already covered in previous comments. Significant means major price swings (>3%), breaking news, or notable new events.

If there are no significant new developments, or the changes are minor/incremental, respond with exactly: NO_UPDATE

Rules:
- Start with a bold timestamp like **14:00** (use the current hour)
- 1-3 bullet points max, only truly significant changes
- Use plain dollar signs, no LaTeX
- Do NOT repeat anything from previous comments
- Do NOT report minor price fluctuations or routine market movements
- Do NOT include preamble or meta-commentary
- CRITICAL: Keep under 512 characters. Be extremely concise.`,
		Question: fmt.Sprintf("## Previous hourly comments\n\n%s\n\n## Latest data\n\n%s", priorUpdates.String(), currentContext),
		Priority: ai.PriorityLow,
	}

	update, err := ai.Ask(prompt)
	if err != nil {
		app.Log("digest", "AI hourly comment generation failed: %v", err)
		return
	}

	update = cleanResponse(update)

	// Check if the AI determined nothing significant has changed
	if strings.TrimSpace(update) == "NO_UPDATE" || strings.TrimSpace(update) == "" {
		app.Log("digest", "No significant changes, skipping hourly comment")
		return
	}

	err = blog.CreateComment(post.ID, update, "micro", "micro")
	if err != nil {
		app.Log("digest", "Failed to add hourly comment: %v", err)
		return
	}

	app.Log("digest", "Hourly comment added to digest %s", post.ID)
}

const digestSystemPrompt = `You are a writer producing a short daily digest blog post.
You will be given today's data from various sources: news headlines, market prices, videos, and a reminder.
Write a very concise digest. Use markdown formatting.

Structure:
1. One sentence setting the theme of the day
2. **News** - 3-5 bullet points, one line each
3. **Markets** - Key movers in one line each
4. **Reminder** - Always use exactly "## Reminder" as the heading, then include each item (Name of Allah, Verse, Hadith) on its own line in bold label format

Rules:
- Do NOT start with a title or heading — jump straight in
- Do NOT include preamble like "Here is the digest"
- Do NOT include a references section
- Do NOT rename the Reminder section — always use "## Reminder"
- Use plain dollar signs (e.g. $69,811), no LaTeX
- CRITICAL: The entire output must be under 1024 characters. Be extremely concise.`

// buildReferences wraps source references in a collapsible details block.
func buildReferences(refs []ref) string {
	if len(refs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n<details>\n<summary>References</summary>\n\n")
	for i, r := range refs {
		sb.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, r.title, r.url))
	}
	sb.WriteString("\n</details>")
	return sb.String()
}

func cleanResponse(s string) string {
	s = stripPreamble(s)
	s = normalizeHeadings(s)
	s = app.StripLatexDollars(s)
	return s
}

func setError(msg string) {
	mu.Lock()
	lastStatus = "error"
	lastError = msg
	mu.Unlock()
}

func setSuccess() {
	mu.Lock()
	lastDigest = time.Now()
	lastStatus = "ok"
	lastError = ""
	mu.Unlock()
	data.SaveFile("digest_last.txt", lastDigest.Format(time.RFC3339))
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
