package social

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"mu/ai"
	"mu/app"
	"mu/blog"
	"mu/data"
	"mu/news/reminder"
	"mu/search"
)

// maxNewsThreads is the number of news stories to seed as discussion threads per day
const maxNewsThreads = 5

// StartSeeding begins the background seeding of social discussions
func StartSeeding() {
	go seedLoop()
}

func seedLoop() {
	// Wait for other services to load first
	time.Sleep(30 * time.Second)

	// Seed immediately on startup
	seedAll()

	// Then check every hour
	for {
		time.Sleep(time.Hour)
		seedAll()
	}
}

func seedAll() {
	seedReminder()
	seedDigest()
	seedTopNews()
}

// seedReminder creates a daily discussion thread from the Islamic reminder
func seedReminder() {
	today := todayKey()
	seedID := "reminder-" + today

	// Check if already seeded today
	if threadExists(seedID) {
		return
	}

	rd := reminder.GetReminderData()
	if rd == nil {
		return
	}

	// Build the thread content with just the message summary and a link
	// to the full reminder page. Embedding the full content (verse, hadith, name)
	// causes markdown formatting issues (backticks become pre blocks, etc.)
	var sb strings.Builder

	if rd.Message != "" {
		sb.WriteString(rd.Message)
		sb.WriteString("\n\n")
	}

	sb.WriteString("[Read the full reminder](/reminder)")
	sb.WriteString("\n\n")
	sb.WriteString("*Share your reflections and thoughts on today's reminder.*")

	content := sb.String()
	if content == "" {
		return
	}

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Reminder — " + time.Now().Format("2 Jan 2006"),
		Link:      "/reminder",
		Content:   content,
		Topic:     "Islam",
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded daily reminder thread")
}

// seedDigest creates a discussion thread for the daily blog digest
func seedDigest() {
	today := todayKey()
	seedID := "digest-" + today

	if threadExists(seedID) {
		return
	}

	digest := blog.FindTodayDigest()
	if digest == nil {
		return
	}

	// Create a summary for discussion — first few lines of digest
	content := digest.Content
	if len(content) > 500 {
		// Truncate at a sentence boundary
		cut := strings.LastIndex(content[:500], ". ")
		if cut > 200 {
			content = content[:cut+1]
		} else {
			content = content[:500]
		}
		content += "\n\n[Read the full digest](/post/" + digest.ID + ")"
	}

	content += "\n\n*What are your thoughts on today's top stories?*"

	thread := &Thread{
		ID:        seedID,
		Title:     "Daily Digest — " + time.Now().Format("2 Jan 2006"),
		Link:      "/post/" + digest.ID,
		Content:   content,
		Topic:     "World",
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	addSeededThread(thread)
	app.Log("social", "Seeded daily digest thread")
}

// seedTopNews creates discussion threads from the most notable news stories.
// Each story gets its own thread with web-sourced context — background on
// the key players, history, and facts — to ground the discussion in truth
// rather than opinion.
func seedTopNews() {
	today := todayKey()

	// Get recent news items
	entries := data.GetByType("news", 30)
	if len(entries) == 0 {
		return
	}

	seeded := 0
	seen := map[string]bool{} // deduplicate by title similarity

	for _, entry := range entries {
		if seeded >= maxNewsThreads {
			break
		}

		// Create a stable ID from entry ID + date
		seedID := fmt.Sprintf("news-%s-%s", today, storyKey(entry.ID))

		if threadExists(seedID) {
			seeded++ // count existing threads toward the limit
			continue
		}

		// Skip if we've seen a very similar title already today
		titleKey := normTitle(entry.Title)
		if seen[titleKey] {
			continue
		}
		seen[titleKey] = true

		// Extract metadata
		link := ""
		if entry.Metadata != nil {
			if u, ok := entry.Metadata["url"].(string); ok {
				link = u
			}
		}

		topic := "World"
		if entry.Metadata != nil {
			if cat, ok := entry.Metadata["category"].(string); ok && isValidTopic(cat) {
				topic = cat
			}
		}

		// Build context-rich discussion content
		content := buildDiscussionContent(entry.Title, entry.Content, link)

		thread := &Thread{
			ID:        seedID,
			Title:     entry.Title,
			Link:      link,
			Content:   content,
			Topic:     topic,
			Author:    app.SystemUserName,
			AuthorID:  app.SystemUserID,
			CreatedAt: time.Now(),
		}

		addSeededThread(thread)
		seeded++
		app.Log("social", "Seeded news thread: %s", entry.Title)
	}
}

// buildDiscussionContent creates a fact-grounded discussion post for a news story.
// It searches the web (Brave) for background context (key players, history, Wikipedia)
// and uses AI to write a truth-seeking blurb that frames the discussion.
func buildDiscussionContent(title, articleContent, link string) string {
	// Search the web for background context on this story
	query := title
	if len(query) > 120 {
		query = query[:120]
	}

	var allResults []search.BraveResult

	// Primary search: the story itself
	results, err := search.SearchBraveCached(query, 5)
	if err == nil && len(results) > 0 {
		allResults = append(allResults, results...)
		app.Log("social", "Brave search for %q: %d results", title, len(results))
	}

	// Background search: historical context, key players
	bgResults, err := search.SearchBraveCached(query+" background history", 5)
	if err == nil && len(bgResults) > 0 {
		allResults = append(allResults, bgResults...)
		app.Log("social", "Brave background for %q: %d results", title, len(bgResults))
	}

	// Format results as context strings for the AI
	contextParts := formatBraveResults(allResults)

	// If we have web context, use AI to synthesise a discussion blurb
	if len(contextParts) > 0 || articleContent != "" {
		blurb := generateDiscussionBlurb(title, articleContent, contextParts)
		if blurb != "" {
			var sb strings.Builder
			sb.WriteString(blurb)

			// Add source links (deduplicated)
			sources := uniqueSources(allResults, 4)
			if len(sources) > 0 {
				sb.WriteString("\n\n**Sources:**\n")
				for _, src := range sources {
					sb.WriteString(fmt.Sprintf("- [%s](%s)\n", src.Title, src.URL))
				}
			}

			if link != "" {
				sb.WriteString(fmt.Sprintf("\n[Read the original article](%s)", link))
			}

			sb.WriteString("\n\n*What are your thoughts? Share what you know.*")
			return sb.String()
		}
	}

	// Fallback: use article content directly
	content := articleContent
	if len(content) > 400 {
		cut := strings.LastIndex(content[:400], ". ")
		if cut > 150 {
			content = content[:cut+1]
		} else {
			content = content[:400] + "..."
		}
	}
	if link != "" {
		content += fmt.Sprintf("\n\n[Read more](%s)", link)
	}
	content += "\n\n*What are your thoughts? Share what you know.*"
	return content
}

// generateDiscussionBlurb uses AI to write a truth-seeking context blurb
// that grounds the discussion in facts rather than opinion.
func generateDiscussionBlurb(title, articleContent string, webContext []string) string {
	var question strings.Builder
	question.WriteString("## Article\n\n")
	question.WriteString("**" + title + "**\n\n")

	if articleContent != "" {
		summary := articleContent
		if len(summary) > 800 {
			cut := strings.LastIndex(summary[:800], ". ")
			if cut > 300 {
				summary = summary[:cut+1]
			} else {
				summary = summary[:800] + "..."
			}
		}
		question.WriteString(summary)
		question.WriteString("\n\n")
	}

	if len(webContext) > 0 {
		question.WriteString("## Web Context\n\n")
		for _, ctx := range webContext {
			question.WriteString("- " + ctx + "\n")
		}
	}

	prompt := &ai.Prompt{
		System: `You are a fact-checker and context writer for Mu, an independent truth-seeking platform. Your job is to write a short discussion primer that helps readers understand a news story with clarity and honesty.

You will receive an article headline, its content, and web search results with background information.

Write a 2-3 paragraph blurb that:
1. Summarises what happened in plain, direct prose
2. Adds essential context: who are the key players, what is the history, what led to this
3. Highlights what is being left out or misrepresented in mainstream coverage, if applicable
4. Notes any impact on the Muslim world, the Middle East, or marginalised communities where relevant

Rules:
- Be factual and specific — cite countries, names, dates, numbers
- Do NOT editorialize or tell readers what to think
- Do NOT use inflammatory language or take sides in geopolitics
- DO illuminate injustice, double standards, or selective reporting where the facts support it
- Write from a globally neutral standpoint — no US-centric or Western-centric framing
- Do NOT start with "This article" or "According to" — just state the facts
- Write dollar amounts as plain numbers like $100 — never use LaTeX
- CRITICAL: Keep under 1000 characters total`,
		Question: question.String(),
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("social", "AI discussion blurb failed: %v", err)
		return ""
	}

	return strings.TrimSpace(app.StripLatexDollars(response))
}

// formatBraveResults converts Brave search results into context strings for AI
func formatBraveResults(results []search.BraveResult) []string {
	var parts []string
	for _, r := range results {
		text := fmt.Sprintf("%s: %s", r.Title, r.Description)
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		if r.URL != "" {
			text += fmt.Sprintf(" (Source: %s)", r.URL)
		}
		parts = append(parts, text)
	}
	return parts
}

// uniqueSources deduplicates Brave results by URL and returns up to limit sources
func uniqueSources(results []search.BraveResult, limit int) []search.BraveResult {
	var out []search.BraveResult
	seen := map[string]bool{}
	for _, r := range results {
		if r.URL == "" || seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// addSeededThread adds a thread without requiring auth or quota
func addSeededThread(thread *Thread) {
	mutex.Lock()
	threads = append([]*Thread{thread}, threads...)
	mutex.Unlock()

	save()
	indexThread(thread)
	updateCache()
}

// threadExists checks if a thread with the given ID already exists
func threadExists(id string) bool {
	mutex.RLock()
	defer mutex.RUnlock()
	return getThread(id) != nil
}

// todayKey returns today's date as a string key
func todayKey() string {
	return time.Now().Format("2006-01-02")
}

// storyKey creates a short hash from a story ID for use in thread IDs
func storyKey(id string) string {
	h := fmt.Sprintf("%x", md5.Sum([]byte(id)))
	return h[:8]
}

// normTitle normalises a title for deduplication — lowercase, strip punctuation
func normTitle(title string) string {
	t := strings.ToLower(title)
	// Take first 5 significant words
	words := strings.Fields(t)
	if len(words) > 5 {
		words = words[:5]
	}
	return strings.Join(words, " ")
}
