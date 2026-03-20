package social

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/search"
)

// maxDailyNotes is the maximum number of community notes seeded per day.
const maxDailyNotes = 5

// NewsArticle is a lightweight news item for fact-checking.
type NewsArticle struct {
	Title       string
	Description string
	URL         string
	Category    string // e.g. "Tech", "Finance", "World"
}

// GetRecentNews returns recent news articles (one per topic) for fact-checking.
// Wired in main.go: social.GetRecentNews = func() []social.NewsArticle { ... }
var GetRecentNews func() []NewsArticle

// seedNewsNotes picks recent news articles and analyses them.
// Only posts when there's something genuinely worth enlightening people about.
func seedNewsNotes() {
	if GetRecentNews == nil {
		return
	}

	if countTodayNotes() >= maxDailyNotes {
		return
	}

	articles := GetRecentNews()
	if len(articles) == 0 {
		return
	}

	for _, article := range articles {
		if countTodayNotes() >= maxDailyNotes {
			break
		}
		seedArticleNote(article)
	}
}

func seedArticleNote(article NewsArticle) {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(article.URL)))
	seedID := "note-" + hash[:12]

	if threadExists(seedID) {
		return
	}

	// 1. Read the actual article
	var articleBody string
	if article.URL != "" {
		_, body, err := search.FetchAndExtract(article.URL)
		if err == nil && len(body) > 0 {
			if len(body) > 4000 {
				body = body[:4000]
			}
			articleBody = body
		}
	}

	// 2. Search for background context (Wikipedia, UN reports, etc.)
	bgQuery := article.Title + " wikipedia"
	if len(bgQuery) > 160 {
		bgQuery = bgQuery[:160]
	}
	var background string
	var bgSources []Source
	bgResults, err := search.SearchBraveCached(bgQuery, 3)
	if err == nil {
		for _, r := range bgResults {
			bgSources = append(bgSources, Source{Title: r.Title, URL: r.URL})
			if background == "" && isReferenceSource(r.URL) {
				_, content, fetchErr := search.FetchAndExtract(r.URL)
				if fetchErr == nil && len(content) > 0 {
					if len(content) > 3000 {
						content = content[:3000]
					}
					background = content
				}
			}
		}
	}

	// 3. Search for additional reporting / alternative perspectives
	var altContext []string
	altResults, err := search.SearchBraveCached(article.Title, 5)
	if err == nil {
		for _, r := range altResults {
			ctx := fmt.Sprintf("%s: %s (Source: %s)", r.Title, r.Description, r.URL)
			if len(ctx) > 500 {
				ctx = ctx[:500] + "..."
			}
			altContext = append(altContext, ctx)
			bgSources = append(bgSources, Source{Title: r.Title, URL: r.URL})
		}
	}

	// 4. Ask AI: is this worth a community note? If so, write one.
	var question strings.Builder
	question.WriteString("## Article\n\n")
	question.WriteString("**" + article.Title + "**\n\n")
	if article.Description != "" {
		question.WriteString(article.Description + "\n\n")
	}
	if articleBody != "" {
		question.WriteString(articleBody + "\n\n")
	}
	if len(altContext) > 0 {
		question.WriteString("## Other Reporting on This Story\n\n")
		for _, ctx := range altContext {
			question.WriteString("- " + ctx + "\n")
		}
		question.WriteString("\n")
	}
	if background != "" {
		question.WriteString("## Background / Reference Material\n\n")
		question.WriteString(background)
		question.WriteString("\n")
	}

	prompt := &ai.Prompt{
		System: noteWriterPrompt,
		Question: question.String(),
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("social", "Community note AI failed: %v", err)
		return
	}

	response = strings.TrimSpace(app.StripLatexDollars(response))
	if response == "" {
		return
	}

	// 5. Parse the response — first line is VERDICT, rest is the thread
	verdict, body := parseNoteResponse(response)
	if verdict == "skip" || body == "" {
		return
	}

	// 6. Append sources
	var content strings.Builder
	content.WriteString(body)

	// Deduplicate and limit sources
	seen := map[string]bool{}
	var sources []Source
	for _, s := range bgSources {
		if s.URL == "" || seen[s.URL] {
			continue
		}
		seen[s.URL] = true
		sources = append(sources, s)
		if len(sources) >= 5 {
			break
		}
	}
	if len(sources) > 0 {
		content.WriteString("\n\n**Sources:**\n")
		for _, src := range sources {
			content.WriteString(fmt.Sprintf("- [%s](%s)\n", src.Title, src.URL))
		}
	}

	topic := mapCategoryToTopic(article.Category)

	thread := &Thread{
		ID:        seedID,
		Title:     article.Title,
		Link:      article.URL,
		Content:   content.String(),
		Topic:     topic,
		Author:    app.SystemUserName,
		AuthorID:  app.SystemUserID,
		CreatedAt: time.Now(),
	}

	AddSeededThread(thread)
	app.Log("social", "Seeded community note: %s — %s", article.Title, article.Category)
}

const noteWriterPrompt = `You are a community note writer. You read news articles in full — the actual content, not just headlines — along with background research from Wikipedia and other reference sources.

Your job: decide if this article needs a community note, and if so, write one that genuinely enlightens the reader. Think of the best threads on X/Twitter — people who break down a complex story, provide the history, humanise the people involved, and help others see what the mainstream framing misses.

STEP 1: Should you write a note?

Most articles DON'T need one. Only write when:
- The article contains specific factual errors you can correct with evidence
- The article omits critical facts that fundamentally change understanding (e.g. reporting strikes without mentioning confirmed civilian casualties, or sanctions without mentioning humanitarian impact)
- The framing dehumanises or erases affected people in a way that would leave the reader with a materially wrong picture
- Market/crypto coverage is designed to induce panic or FOMO when the data says otherwise

Do NOT write a note for:
- Articles that are factually accurate and reasonably framed
- Simplified headlines — that's normal journalism
- Opinion pieces or analysis
- Stories where you'd just be adding "nice to know" context

If the article doesn't need a note, respond with just: VERDICT: skip

STEP 2: Write the note.

If the article DOES need a note, write it as an enlightening thread:

VERDICT: note

[Your community note here in markdown]

The note should:
- Lead with the key insight — what does the reader need to know that the article doesn't tell them?
- Provide historical context from the background research — dates, events, decisions that led to this moment
- Humanise the affected people — who are they, what is their daily reality, how many are affected?
- Use specific facts, numbers, and named sources — not vague assertions
- Be 2-4 paragraphs. Substantive enough to educate, concise enough to read in 60 seconds
- Use a direct, clear tone. No lecturing, no moralising, no "it's important to note that..."
- Let the facts speak. If the facts are damning, they don't need your commentary.`

// parseNoteResponse extracts the verdict and body from the AI response.
func parseNoteResponse(response string) (string, string) {
	lines := strings.SplitN(response, "\n", 2)
	if len(lines) == 0 {
		return "skip", ""
	}

	firstLine := strings.TrimSpace(lines[0])
	if strings.HasPrefix(strings.ToUpper(firstLine), "VERDICT:") {
		verdict := strings.TrimSpace(strings.TrimPrefix(
			strings.TrimPrefix(firstLine, "VERDICT:"),
			"verdict:"))
		verdict = strings.ToLower(strings.TrimSpace(verdict))

		if verdict == "skip" || verdict == "none" || verdict == "no" {
			return "skip", ""
		}

		if len(lines) > 1 {
			return "note", strings.TrimSpace(lines[1])
		}
		return "skip", ""
	}

	// No verdict line — treat entire response as content if it's substantial
	if len(response) > 100 {
		return "note", response
	}
	return "skip", ""
}

// countTodayNotes counts how many community note threads were seeded today.
func countTodayNotes() int {
	today := todayKey()
	mutex.RLock()
	defer mutex.RUnlock()

	count := 0
	for _, t := range threads {
		if !strings.HasPrefix(t.ID, "note-") {
			continue
		}
		if t.CreatedAt.Format("2006-01-02") != today {
			continue
		}
		count++
	}
	return count
}

// mapCategoryToTopic maps a news feed category to a valid social topic.
func mapCategoryToTopic(category string) string {
	switch category {
	case "Crypto":
		return "Crypto"
	case "Dev":
		return "Dev"
	case "Finance":
		return "Finance"
	case "Islam":
		return "Islam"
	case "Politics":
		return "Politics"
	case "Tech":
		return "Tech"
	case "UK":
		return "UK"
	case "World":
		return "World"
	default:
		return "World"
	}
}
