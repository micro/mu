package blog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/markets"
	"mu/news"
	"mu/reminder"
	"mu/search"
	"mu/video"
)

// opinionTag is the tag used for daily opinion posts.
const opinionTag = "opinion"

// agentPurpose is the foundational objective that grounds all of the opinion
// agent's behaviour — writing, engagement, self-reflection, and memory.
// This is the root node. Core principles are immutable. Everything the agent
// does must serve this purpose. Any self-modification (directives, stances)
// operates within these bounds and cannot override them.
const agentPurpose = `You are Micro, the voice of Mu — an independent platform built in the UK.

Your foundational purpose is truth-seeking. You exist to help readers see through narratives — not to replace one narrative with another, but to develop the habit of questioning what they are shown.

Your moral grounding:
- You are rooted in Islamic values: honesty, justice, accountability, and the rejection of greed-driven decision making. You believe this life is a test, this world is temporary, and it was created with purpose. This shapes your moral compass — you don't preach it, but it is always present.
- You are NOT in opposition to Western culture or modernisation. Technology, advancement, and progress are part of human evolution. But they must be grounded in the right morality and purpose — serving people, not exploiting them.
- You reject the idea that any single civilisation, institution, or media ecosystem has a monopoly on truth.

Your intellectual character:
- Engineering mindset — you look at systems, incentives, and data rather than taking narratives at face value.
- Sceptical of media bias from ALL directions — Western, Eastern, state-run, corporate, independent. No source is gospel.
- Not contrarian for the sake of it. You follow evidence and reasoning wherever they lead.
- You care about the Muslim world, the Global South, and underrepresented perspectives — but you write for a global audience.

Your measure of success:
- Did the reader question something they previously took for granted?
- Did you provide context that was missing from the mainstream conversation?
- Did you connect dots that others missed?
- A single strong piece that makes someone pause and think is worth more than ten that merely inform.`

// opinionCategories returns the list of categories from topics.json.
// Uses topicsJSON which is embedded in blog.go.
func opinionCategories() []string {
	var cats []string
	json.Unmarshal(topicsJSON, &cats)
	return cats
}

// StartOpinion begins the background opinion generation and engagement loops.
// Called from main.go after all building blocks are loaded.
func StartOpinion() {
	memory = loadMemory()
	go opinionLoop()
	go opinionEngageLoop()
}

func opinionLoop() {
	// Wait for other services to load first
	time.Sleep(30 * time.Second)

	for {
		publishNextOpinion()
		time.Sleep(30 * time.Minute) // check every 30m, actual pacing is time-based
	}
}

// opinionEngageLoop runs the opinion agent's engagement cycle.
// Every hour it checks for new human comments on today's opinion posts,
// then reviews the discussion to extract learnings for editorial memory.
func opinionEngageLoop() {
	time.Sleep(2 * time.Minute)

	for {
		for _, post := range FindTodayOpinions() {
			engageOpinionPost(post)
			reviewOpinionPost(post)
		}
		selfReflect()
		time.Sleep(time.Hour)
	}
}

// publishNextOpinion finds the next category that needs an opinion today
// and publishes it, respecting the 2-hour spacing between posts.
func publishNextOpinion() {
	categories := opinionCategories()
	if len(categories) == 0 {
		return
	}

	published := findTodayOpinionCategories()

	// Find last publish time today
	if last := latestTodayOpinionTime(); !last.IsZero() {
		elapsed := time.Since(last)
		interval := opinionInterval(len(categories))
		if elapsed < interval {
			return // too soon
		}
	}

	// Find next category to publish (tags are stored lowercase)
	for _, cat := range categories {
		if _, done := published[strings.ToLower(cat)]; !done {
			publishCategoryOpinion(cat)
			return
		}
	}
}

// opinionInterval calculates spacing between posts.
// Target: spread across ~16 waking hours (06:00–22:00).
func opinionInterval(numCategories int) time.Duration {
	if numCategories <= 1 {
		return 2 * time.Hour
	}
	interval := (16 * time.Hour) / time.Duration(numCategories)
	// Clamp between 1h and 3h
	if interval < time.Hour {
		interval = time.Hour
	}
	if interval > 3*time.Hour {
		interval = 3 * time.Hour
	}
	return interval
}

// publishCategoryOpinion generates and publishes an opinion for a specific category.
func publishCategoryOpinion(category string) {
	title, body, err := generateOpinion(category)
	if err != nil {
		app.Log("opinion", "Opinion generation failed [%s]: %v", category, err)
		return
	}

	tags := opinionTag + "," + strings.ToLower(category)
	err = CreatePost(title, body, app.SystemUserName, app.SystemUserID, tags, false)
	if err != nil {
		app.Log("opinion", "Failed to create opinion post [%s]: %v", category, err)
		return
	}

	recordOpinionTopic(title, category)
	app.Log("opinion", "Opinion published [%s]: %s", category, title)
}

// FindTodayOpinion returns the first opinion post from today (for backwards compat).
func FindTodayOpinion() *Post {
	opinions := FindTodayOpinions()
	if len(opinions) == 0 {
		return nil
	}
	return opinions[0]
}

// FindTodayOpinions returns all opinion posts from today, newest first.
func FindTodayOpinions() []*Post {
	mutex.RLock()
	defer mutex.RUnlock()

	now := time.Now()
	y, m, d := now.Date()
	var result []*Post
	for _, post := range posts {
		if post.AuthorID != app.SystemUserID {
			continue
		}
		if !strings.Contains(post.Tags, opinionTag) {
			continue
		}
		py, pm, pd := post.CreatedAt.Date()
		if py == y && pm == m && pd == d {
			result = append(result, post)
		}
	}
	return result
}

// findTodayOpinionCategories returns which categories have been published today.
func findTodayOpinionCategories() map[string]bool {
	result := make(map[string]bool)
	for _, post := range FindTodayOpinions() {
		for _, tag := range strings.Split(post.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != opinionTag && tag != "" {
				result[tag] = true
			}
		}
	}
	return result
}

// latestTodayOpinionTime returns the creation time of the most recent opinion today.
func latestTodayOpinionTime() time.Time {
	opinions := FindTodayOpinions()
	if len(opinions) == 0 {
		return time.Time{}
	}
	// posts are newest-first
	return opinions[0].CreatedAt
}

// generateOpinion gathers category-specific data (news, markets, videos),
// cross-references with web search for deeper context, and uses AI to
// produce an opinion piece. Returns the title and the body content.
func generateOpinion(category string) (string, string, error) {
	context := gatherCategoryContext(category)
	if context == "" {
		return "", "", fmt.Errorf("no content available for %s", category)
	}

	webResearch := researchCategoryStories(category)

	fullContext := context
	if webResearch != "" {
		fullContext += "\n\n## Web Research & Cross-References\n\n" + webResearch
	}

	memContext := getMemoryContext()
	if memContext != "" {
		fullContext += "\n\n" + memContext
	}

	prompt := &ai.Prompt{
		System: agentPurpose + fmt.Sprintf(`

Your task: Write today's opinion piece for the **%s** category.

Today's Islamic reminder (verse, hadith) is provided as context — let it inform your moral framing where relevant, but don't force it. You have been given web research with full article content from independent sources — use this to cross-reference and challenge the mainstream narrative.

What you produce:
- A sharp, opinionated piece focused on %s that makes the reader question something they might otherwise accept uncritically
- Focus on the most compelling angle within this category's news today
- Connect the dots between events, market movements, and geopolitics where relevant
- Call out media bias, missing context, or misleading framing where you see it
- Offer your own grounded assessment — what's really happening and why it matters

Your output format:
Line 1: Just the opinion title (no "Opinion:" prefix, no quotes). This should be punchy and reflect your take, e.g. "Markets are not looking at the war right" or "The AI hype is hiding a labour crisis"
Line 2: Empty line
Line 3+: The opinion piece body

Rules:
- Write 4-6 paragraphs of flowing prose
- Be direct and assertive — this is an opinion, not a report
- Use plain language, no jargon
- Do NOT start with "Today" or "In today's"
- Do NOT include bullet points, lists, or headings in the body
- Do NOT include a references section
- Write dollar amounts as plain numbers like $94 or $1.2 trillion — NEVER use LaTeX formatting
- Do NOT include preamble like "Here is my opinion"
- CRITICAL: Keep under 2500 characters total (title + body).`, category, category),
		Question: fullContext,
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		return "", "", err
	}

	response = strings.TrimSpace(app.StripLatexDollars(response))

	parts := strings.SplitN(response, "\n", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected response format")
	}

	title := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])

	title = strings.TrimPrefix(title, "Opinion: ")
	title = strings.TrimPrefix(title, "Opinion:")
	title = strings.Trim(title, `"'`)

	if title == "" || body == "" {
		return "", "", fmt.Errorf("empty title or body")
	}

	return title, body, nil
}

// gatherCategoryContext builds context focused on a specific category,
// with supporting market data and Islamic reminder.
func gatherCategoryContext(category string) string {
	var sb strings.Builder

	feed := news.GetFeed()
	if len(feed) > 0 {
		// Primary: news for this category
		var categoryItems []*news.Post
		for _, item := range feed {
			if strings.EqualFold(item.Category, category) {
				categoryItems = append(categoryItems, item)
			}
		}
		if len(categoryItems) > 0 {
			sb.WriteString(fmt.Sprintf("## %s News (Primary Focus)\n\n", category))
			count := 8
			if len(categoryItems) < count {
				count = len(categoryItems)
			}
			for _, item := range categoryItems[:count] {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", item.Title, item.Description))
			}
			sb.WriteString("\n")
		}

		// Brief context from other categories
		byCategory := make(map[string][]*news.Post)
		for _, item := range feed {
			if !strings.EqualFold(item.Category, category) {
				byCategory[item.Category] = append(byCategory[item.Category], item)
			}
		}
		if len(byCategory) > 0 {
			sb.WriteString("## Other Headlines (for context)\n\n")
			cats := make([]string, 0, len(byCategory))
			for c := range byCategory {
				cats = append(cats, c)
			}
			sort.Strings(cats)
			for _, c := range cats {
				items := byCategory[c]
				count := 2
				if len(items) < count {
					count = len(items)
				}
				for _, item := range items[:count] {
					sb.WriteString(fmt.Sprintf("- [%s] %s\n", c, item.Title))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Market data — always useful context
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
			sb.WriteString(fmt.Sprintf("- %s by %s\n", v.Title, v.Channel))
		}
		sb.WriteString("\n")
	}

	rd := reminder.GetReminderData()
	if rd != nil {
		sb.WriteString("## Today's Islamic Reminder\n\n")
		if rd.Message != "" {
			sb.WriteString(rd.Message + "\n\n")
		}
		if rd.Verse != "" {
			sb.WriteString("Verse: " + rd.Verse + "\n\n")
		}
		if rd.Hadith != "" {
			sb.WriteString("Hadith: " + rd.Hadith + "\n\n")
		}
	}

	return sb.String()
}

// researchCategoryStories does web research on the top stories for a category.
func researchCategoryStories(category string) string {
	feed := news.GetFeed()
	if len(feed) == 0 {
		return ""
	}

	var categoryItems []*news.Post
	for _, item := range feed {
		if strings.EqualFold(item.Category, category) {
			categoryItems = append(categoryItems, item)
		}
	}
	if len(categoryItems) == 0 {
		return ""
	}

	limit := 3
	if len(categoryItems) < limit {
		limit = len(categoryItems)
	}

	var sb strings.Builder
	for _, item := range categoryItems[:limit] {
		query := item.Title
		if len(query) > 120 {
			query = query[:120]
		}

		results, err := search.SearchBraveCached(query, 5)
		if err != nil || len(results) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### Research: %s\n", item.Title))

		for _, r := range results {
			desc := r.Description
			if len(desc) > 300 {
				desc = desc[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s (Source: %s)\n", r.Title, desc, r.URL))
		}

		if len(results) > 0 {
			fullContent := fetchArticleContent(results[0].URL)
			if fullContent != "" {
				sb.WriteString(fmt.Sprintf("\nFull article from %s:\n%s\n", results[0].URL, fullContent))
			}
		}

		sb.WriteString("\n")
		time.Sleep(500 * time.Millisecond)
	}

	return sb.String()
}

func fetchArticleContent(rawURL string) string {
	_, body, err := search.FetchAndExtract(rawURL)
	if err != nil {
		app.Log("opinion", "Failed to fetch %s: %v", rawURL, err)
		return ""
	}

	if len(body) > 2000 {
		cut := strings.LastIndex(body[:2000], ". ")
		if cut > 1000 {
			body = body[:cut+1]
		} else {
			body = body[:2000]
		}
		body += "\n[truncated]"
	}

	return body
}

func opinionTodayKey() string {
	return time.Now().Format("2006-01-02")
}
