// Package digest generates daily news digests by synthesizing headlines,
// market data, and video content into a coherent briefing. The generated
// digest is published as a blog post tagged "digest".
package digest

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/data"
	"mu/markets"
	"mu/news"
	"mu/video"
)

// DigestPost holds minimal info about a digest blog post.
// Populated via callbacks wired in main.go to avoid import cycles with blog.
type DigestPost struct {
	ID      string
	Title   string
	Content string
}

// PublishBlogPost creates a new blog post. Wired in main.go.
var PublishBlogPost func(title, content, author, authorID, tags string) (string, error)

// UpdateBlogPost updates an existing blog post. Wired in main.go.
var UpdateBlogPost func(id, title, content, tags string) error

// FindTodayBlogDigest returns today's digest blog post, if any. Wired in main.go.
var FindTodayBlogDigest func() *DigestPost

var (
	mu         sync.Mutex
	running    bool
	lastDigest time.Time
	lastError  string
	lastStatus string // "ok", "error", "running", "pending"
)

// Load starts the daily digest scheduler.
func Load() {
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

// Status returns the current digest state for the status page.
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

// GetTodayDigest returns today's digest from the blog, or nil.
func GetTodayDigest() *DigestPost {
	if FindTodayBlogDigest == nil {
		return nil
	}
	return FindTodayBlogDigest()
}

// GetLatestDigest returns whether a digest exists (for status checks).
func GetLatestDigest() bool {
	return !lastDigest.IsZero()
}

func scheduler() {
	// Wait for blog callbacks to be wired in main.go
	time.Sleep(5 * time.Second)
	// Only create a digest on startup if one doesn't exist for today
	if GetTodayDigest() == nil {
		generate()
	}
	for {
		// Run once per day — sleep until next 6am UTC
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		time.Sleep(time.Until(next))
		generate()
	}
}

func generate() {
	if PublishBlogPost == nil || UpdateBlogPost == nil {
		app.Log("digest", "Blog callbacks not wired, skipping")
		return
	}

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

	existing := GetTodayDigest()
	if existing == nil {
		createDigest()
	} else {
		updateDigest(existing)
	}
}

func createDigest() {
	app.Log("digest", "Creating new daily digest")

	context, refs := gatherContext()
	if context == "" {
		setError("no content available")
		app.Log("digest", "No content available for digest")
		return
	}

	response, err := generateDigestContent(context)
	if err != nil {
		setError(err.Error())
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	response += buildReferences(refs)

	title := "Daily Digest — " + time.Now().Format("2 Jan 2006")
	_, err = PublishBlogPost(title, response, app.SystemUserName, app.SystemUserID, "digest")
	if err != nil {
		setError(err.Error())
		app.Log("digest", "Failed to publish digest blog post: %v", err)
		return
	}

	setSuccess()
	app.Log("digest", "Daily digest published as blog post: %s", title)
}

func updateDigest(existing *DigestPost) {
	app.Log("digest", "Updating digest %s with latest data", existing.ID)

	context, refs := gatherContext()
	if context == "" {
		app.Log("digest", "No content available for update")
		setSuccess()
		return
	}

	response, err := generateDigestContent(context)
	if err != nil {
		setError(err.Error())
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	response += buildReferences(refs)

	err = UpdateBlogPost(existing.ID, existing.Title, response, "digest")
	if err != nil {
		setError(err.Error())
		app.Log("digest", "Failed to update digest blog post: %v", err)
		return
	}

	setSuccess()
	app.Log("digest", "Digest %s updated with latest data", existing.ID)
}

func generateDigestContent(context string) (string, error) {
	if context == "" {
		return "", nil
	}

	prompt := &ai.Prompt{
		System: `You are a senior analyst writing a daily briefing for Mu, an independent platform built in the UK. Your audience is global and diverse, with particular relevance to Muslim readers — but the content is for everyone.

You will be given news headlines, market data, and video content from today.

Write a coherent, integrated summary that connects the dots between events and market movements. The reader wants to understand what happened today and WHY markets moved — not just see raw prices.

Perspective:
- Write from a globally neutral standpoint — no US-centric framing or bias
- Never use relative phrases like "back home", "here", or "domestically" to refer to any single country
- Name countries explicitly: "in the US", "in the UK", "in Saudi Arabia"
- Give appropriate weight to events in the Muslim world, the Middle East, Africa, and Asia — not just Western markets
- Where relevant, note impacts on halal markets, Islamic finance, or Muslim-majority economies
- Treat all regions with equal editorial weight

Structure your briefing as 3-5 short paragraphs of flowing prose:
- Open with the dominant theme or story of the day
- Weave in market movements where relevant to the narrative (e.g. "Oil surged 8% as tensions in the Gulf escalated" not "Oil: $94.63")
- Cover geopolitics, finance, tech, and other notable stories
- Close with anything else worth knowing

Rules:
- Write in plain, direct prose — no bullet points, no lists, no headings
- Do NOT start with a title or heading
- Do NOT include preamble like "Here is today's briefing"
- Do NOT include a references section
- Write dollar amounts as plain numbers like $94 or $1.2 trillion — NEVER use LaTeX formatting, backslashes, or math notation
- Keep it human and readable — like a morning briefing email
- CRITICAL: Keep under 1500 characters total.`,
		Question: context,
		Priority: ai.PriorityLow,
		Caller:   "daily-digest",
	}

	draft, err := ai.Ask(prompt)
	if err != nil {
		return "", err
	}

	return cleanResponse(draft), nil
}

type ref struct {
	title string
	url   string
}

func gatherContext() (string, []ref) {
	var sb strings.Builder
	var refs []ref

	feed := news.GetFeed()
	if len(feed) > 0 {
		sb.WriteString("## News Headlines\n\n")
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
				sb.WriteString(fmt.Sprintf("- %s: %s\n", item.Title, item.Description))
			}
			sb.WriteString("\n")
		}
	}

	priceData := markets.GetAllPriceData()
	if len(priceData) > 0 {
		sb.WriteString("## Market Data\n\n")
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
			for _, symbol := range cat.assets {
				if pd, ok := priceData[symbol]; ok && pd.Price > 0 {
					change := ""
					if pd.Change24h != 0 {
						change = fmt.Sprintf(" %+.1f%%", pd.Change24h)
					}
					sb.WriteString(fmt.Sprintf("- %s: %.2f USD%s\n", symbol, pd.Price, change))
				}
			}
		}
		sb.WriteString("\n")
	}

	videos := video.GetLatestVideos(5)
	if len(videos) > 0 {
		sb.WriteString("## Videos\n\n")
		for _, v := range videos {
			refs = append(refs, ref{v.Title, v.URL})
			sb.WriteString(fmt.Sprintf("- %s by %s\n", v.Title, v.Channel))
		}
		sb.WriteString("\n")
	}

	return sb.String(), refs
}

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
