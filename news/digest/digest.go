// Package digest generates daily news digests by synthesizing headlines,
// market data, and video content into a coherent briefing. It lives under
// news/ because it summarizes the day's news — it is not a blog post.
package digest

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// DigestEntry represents a single daily digest.
type DigestEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	mu         sync.Mutex
	running    bool
	lastDigest time.Time
	lastError  string
	lastStatus string // "ok", "error", "running", "pending"

	digestMu sync.RWMutex
	digests  []*DigestEntry
)

// Load starts the daily digest scheduler and loads existing digests.
func Load() {
	if b, err := data.LoadFile("digests.json"); err == nil {
		json.Unmarshal(b, &digests)
	}

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

// GetTodayDigest returns today's digest entry, or nil if none exists.
func GetTodayDigest() *DigestEntry {
	digestMu.RLock()
	defer digestMu.RUnlock()

	now := time.Now()
	y, m, d := now.Date()
	for _, entry := range digests {
		ey, em, ed := entry.CreatedAt.Date()
		if ey == y && em == m && ed == d {
			return entry
		}
	}
	return nil
}

// GetLatestDigest returns the most recent digest entry.
func GetLatestDigest() *DigestEntry {
	digestMu.RLock()
	defer digestMu.RUnlock()
	if len(digests) == 0 {
		return nil
	}
	return digests[0]
}

// GetDigestByDate returns the digest for a specific date (YYYY-MM-DD), or nil.
func GetDigestByDate(date string) *DigestEntry {
	digestMu.RLock()
	defer digestMu.RUnlock()
	for _, entry := range digests {
		if entry.ID == date {
			return entry
		}
	}
	return nil
}

// Handler serves the daily digest page at /news/digest.
// Supports ?date=YYYY-MM-DD to fetch a specific day's digest.
func Handler(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")

	var d *DigestEntry
	if date != "" {
		d = GetDigestByDate(date)
	} else {
		d = GetLatestDigest()
	}

	if app.WantsJSON(r) {
		if d == nil {
			app.RespondJSON(w, map[string]any{"digest": nil})
			return
		}
		app.RespondJSON(w, map[string]any{"digest": d})
		return
	}

	if d == nil {
		msg := "No digest available yet. Check back soon."
		if date != "" {
			msg = fmt.Sprintf("No digest found for %s.", date)
		}
		app.Respond(w, r, app.Response{
			Title:       "Daily Digest",
			Description: "No digest available",
			HTML:        fmt.Sprintf(`<p>%s</p>`, msg),
		})
		return
	}

	rendered := string(app.Render([]byte(d.Content)))
	html := fmt.Sprintf(`<div class="digest">%s</div>`, rendered)

	app.Respond(w, r, app.Response{
		Title:       "Daily Digest — " + d.CreatedAt.Format("2 Jan 2006"),
		Description: "Daily briefing summarising news, markets, and videos",
		HTML:        html,
	})
}

func scheduler() {
	generate()
	for {
		time.Sleep(time.Hour)
		generate()
	}
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

	entry := &DigestEntry{
		ID:        time.Now().Format("2006-01-02"),
		Title:     time.Now().Format("2 January 2006"),
		Content:   response,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	digestMu.Lock()
	digests = append([]*DigestEntry{entry}, digests...)
	if len(digests) > 30 {
		digests = digests[:30]
	}
	digestMu.Unlock()

	saveDigests()
	setSuccess()
	app.Log("digest", "Daily digest published: %s", entry.Title)
}

func updateDigest(entry *DigestEntry) {
	app.Log("digest", "Updating digest %s with latest data", entry.ID)

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

	digestMu.Lock()
	entry.Content = response
	entry.UpdatedAt = time.Now()
	digestMu.Unlock()

	saveDigests()
	setSuccess()
	app.Log("digest", "Digest %s updated with latest data", entry.ID)
}

func saveDigests() {
	digestMu.RLock()
	defer digestMu.RUnlock()
	data.SaveJSON("digests.json", digests)
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
