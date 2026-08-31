// Package blog is the agent that writes.
//
// It was inside service/blog, which made the blog service import markets, news,
// prayer, video and web — five services, reached sideways, because the thing
// choosing what to write about lived in the thing that stores what was written.
// Those five imports were not dependencies, they were editorial policy compiled
// in: flights shipped and the daily opinion could not mention it until somebody
// edited this file.
//
// Services answer questions about state. Agents decide which questions to ask.
// The blog service holds posts; this decides what is worth saying today and
// calls CreatePost like any other caller. An agent is allowed to read the
// catalogue — that is what an agent is for — so the imports below are here
// rather than gone, and here is where they cost nothing: nobody has to
// understand this file to understand what a blog post is.
package blog

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"

	blogsvc "mu/service/blog"
)

// opinionTag is the tag used for daily opinion posts.
const opinionTag = "opinion"

// agentPurpose is the foundational objective that grounds all of the opinion
// agent's behaviour — writing, engagement, self-reflection, and memory.
// This is the root node. Core principles are immutable. Everything the agent
// does must serve this purpose. Any self-modification (directives, stances)
// operates within these bounds and cannot override them.
const agentPurpose = `You are Micro, the voice of Mu — an independent platform built in the UK.

Your foundational purpose is to inform and benefit. You exist to help readers understand the world more clearly — not to tear down, mock, or point fingers, but to illuminate what matters and why.

Your moral grounding:
- You are rooted in Islamic values: honesty, justice, mercy, accountability, and the rejection of greed-driven decision making. You believe this life is a test, this world is temporary, and it was created with purpose. This shapes your moral compass — you don't preach it, but it is always present.
- The Prophet (peace be upon him) said: "Do not harm and do not reciprocate harm." This is your editorial north star. Your writing should never belittle, backbite, or mock — even when critiquing powerful institutions.
- You are NOT in opposition to Western culture or modernisation. Technology, advancement, and progress are part of human evolution. But they must be grounded in the right morality and purpose — serving people, not exploiting them.
- You respect that good exists everywhere and that no single civilisation, institution, or media ecosystem has a monopoly on truth — or on error.

Your intellectual character:
- Engineering mindset — you look at systems, incentives, and data to understand how things actually work.
- Fair-minded and balanced. You present multiple perspectives honestly before offering your own assessment.
- Not contrarian for the sake of it. You follow evidence and reasoning wherever they lead.
- You care about the Muslim world, the Global South, and underrepresented perspectives — but you write for a global audience.
- You assume good faith in people and institutions unless the evidence clearly shows otherwise.

Your tone:
- Informative, thoughtful, and constructive — like a wise friend explaining what's going on.
- Never snarky, sarcastic, or mocking. Never gossip or backbite. Never punch down.
- When you identify a problem, also point toward what good looks like. Critique without cruelty.
- Write with humility — you could be wrong, and you're comfortable saying so.

Your measure of success:
- Did the reader learn something genuinely useful?
- Did you provide context that helps them understand the bigger picture?
- Did you connect information in a way that benefits their understanding?
- A single piece that leaves someone better informed and more thoughtful is worth more than ten that merely provoke.`

// opinionCategories is what the blog says it is about.
//
// One of the two places this still reads the blog package directly rather than
// through a tool. Writing goes through blog_create — that is the effect, and an
// effect should be counted. These two are reads of the blog's own configuration
// and of its own posts, and there is no tool shaped for either: blog_list
// returns prose, and asking it "which categories did Micro publish under today"
// would mean parsing back out of text. Worth a tool eventually; not worth
// pretending it has one now.
func opinionCategories() []string { return blogsvc.Topics() }

// Start begins the background opinion generation loop.
func Start() {
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
// DISABLED: With no active users, this loop burns AI calls responding to
// empty comment sections, reviewing nothing, and self-reflecting unnecessarily.
// Re-enable when there are active users engaging with opinion posts.
func opinionEngageLoop() {
	app.Log("opinion", "Engagement loop disabled to reduce API costs")
}

// One piece per topic, per day — and a ceiling, because the topic list is a
// data file and a data file can grow.
//
// This was `const maxDailyOpinions = 1`, added as cost control, and one a day
// against eight topics means each subject gets a piece every eight days. A
// blog that says it is about Finance and last mentioned it a week ago is not
// covering Finance. opinionInterval was already written for one-per-category —
// it divides sixteen waking hours by the count — so the pacing has been
// waiting for this the whole time.
//
// The ceiling is the honest part of the cost argument. Each piece is a
// catalogue gather, a web research pass and a generation, all billed to
// Micro's own account, so the number of them has to be bounded by something
// other than how many strings somebody put in topics.json.
const maxDailyOpinions = 8

// dailyOpinions is how many pieces today should have: one per topic, capped.
func dailyOpinions() int {
	n := len(opinionCategories())
	if n > maxDailyOpinions {
		return maxDailyOpinions
	}
	return n
}

// opinionsEnabled is the switch for an instance that does not want them.
//
// The same shape as NOTES=off in service/blog/notes.go rather than a new idea:
// a self-hosted instance whose model costs money is entitled to a blog that
// does not write itself, and the alternative to a switch is editing
// topics.json down to nothing, which reads as a mistake rather than a
// decision.
func opinionsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OPINIONS"))) {
	case "off", "false", "no", "0", "none":
		return false
	}
	return true
}

// publishNextOpinion finds the next category that needs an opinion today
// and publishes it, respecting the spacing between posts.
func publishNextOpinion() {
	if !opinionsEnabled() {
		return
	}
	categories := opinionCategories()
	if len(categories) == 0 {
		return
	}

	published := findTodayOpinionCategories()

	// Cost control: limit daily opinion posts
	want := dailyOpinions()
	if len(published) >= want {
		return
	}

	// Find last publish time today
	if last := latestTodayOpinionTime(); !last.IsZero() {
		elapsed := time.Since(last)
		interval := opinionInterval(want)
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
	// Published through the tool, not the package. blog_create takes the author
	// from the caller, and the caller is now Micro's own account — so the post
	// is attributed, counted and priced exactly as it would be for anybody else
	// publishing to their blog.
	_, failed, err := api.ExecuteToolAs(auth.MicroID, "blog_create", map[string]any{
		"title": title, "content": body, "tags": tags,
	})
	if err == nil && failed {
		err = fmt.Errorf("blog_create refused the post")
	}
	if err != nil {
		app.Log("opinion", "Failed to create opinion post [%s]: %v", category, err)
		return
	}

	recordOpinionTopic(title, category)
	app.Log("opinion", "Opinion published [%s]: %s", category, title)
}

// FindTodayOpinions returns all opinion posts from today, newest first.
//
// Through the service's public API rather than its posts slice and its mutex,
// which is what moving out of the package cost and what it was worth: this can
// no longer reach into blog's internals by accident.
func FindTodayOpinions() []*blogsvc.Post {
	now := time.Now()
	y, m, d := now.Date()
	return opinions(func(t time.Time) bool {
		py, pm, pd := t.Date()
		return py == y && pm == m && pd == d
	})
}

// OpinionsSince is the same pieces, over a window rather than a calendar day.
//
// The digest needs this and "today" is the wrong question for it: the briefing
// goes out at 06:00 and the pieces are written across the sixteen hours before
// that, so a digest asking what was published today would always find nothing
// and fall back to the feeds — every morning, silently, which is the failure
// mode where the feature looks implemented and never runs.
//
// A window also survives the thing a calendar day does not: an instance whose
// clock is UTC and whose readers are not.
func OpinionsSince(t time.Time) []*blogsvc.Post {
	return opinions(func(at time.Time) bool { return at.After(t) })
}

// opinions is the shared walk: Micro's posts, tagged opinion, matching when.
func opinions(when func(time.Time) bool) []*blogsvc.Post {
	var result []*blogsvc.Post
	for _, post := range blogsvc.PostsByAuthorID(auth.MicroID, "Micro") {
		if !strings.Contains(post.Tags, opinionTag) {
			continue
		}
		if when(post.CreatedAt) {
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
	material := gatherFromCatalogue(category)
	if material == "" {
		return "", "", fmt.Errorf("no content available for %s", category)
	}

	webResearch := researchCategoryStories(category)

	fullContext := material
	if webResearch != "" {
		fullContext += "\n\n## Web Research & Cross-References\n\n" + webResearch
	}

	memContext := getMemoryContext()
	if memContext != "" {
		fullContext += "\n\n" + memContext
	}

	prompt := &ai.Prompt{
		System: agentPurpose + fmt.Sprintf(`

Your task: Write today's analysis piece for the **%s** category.

Today's Islamic reminder (verse, hadith) is provided as context — let it inform your moral framing where relevant, but don't force it. You have been given web research with full article content from multiple sources — use this to provide a well-rounded, informed perspective.

What you produce:
- An informative, thoughtful piece focused on %s that helps the reader understand what's happening and why it matters
- Focus on the most important story or theme within this category's news today
- Connect the dots between events, market movements, and geopolitics where relevant
- Where context is missing from headlines, provide it fairly — explain what's being overlooked and why it matters
- Offer your own grounded assessment with humility — acknowledge uncertainty where it exists

What you must NEVER do:
- Never mock, belittle, or use sarcasm about any person, company, or institution
- Never use language that sounds like gossip or backbiting
- Never be snarky or cynical — critique constructively, with mercy
- Never assume bad faith without clear evidence
- When identifying problems, also point toward what good looks like

Your output format:
Line 1: Just the title (no "Opinion:" prefix, no quotes). This should be clear and informative, e.g. "What the AI marketplace trend means for independent creators" or "Understanding the shift in quarterly reporting rules"
Line 2: Empty line
Line 3+: The piece body

Rules:
- Write 4-6 paragraphs of flowing prose
- Be clear and direct — inform, don't lecture
- Use plain language, no jargon
- Do NOT start with "Today" or "In today's"
- Do NOT include bullet points, lists, or headings in the body
- Do NOT include a references section
- Write dollar amounts as plain numbers like $94 or $1.2 trillion — NEVER use LaTeX formatting
- Do NOT include preamble like "Here is my opinion"
- CRITICAL: Keep under 2500 characters total (title + body).`, category, category),
		Question: fullContext,
		Priority: ai.PriorityLow,
		Caller:   "opinion-generate",
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

// researchCategoryStories does web research on the top stories for a category.
//
// The headlines come from the news service as data — the structured half of
// news_list, added for exactly this — rather than by parsing them back out of
// the prose the same service had just formatted. Web search is a paid call and
// stays a direct one: it is the only place this agent spends money, and that is
// worth being able to see.
func researchCategoryStories(category string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gatherTimeout)
	defer cancel()

	rsp, err := service.CallDynamic(ctx, "news", "List", map[string]any{
		"topic": category, "limit": researchStories,
	})
	if err != nil {
		return ""
	}
	items, _ := rsp["items"].([]any)
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, raw := range items {
		if i >= researchStories {
			break
		}
		item, _ := raw.(map[string]any)
		title, _ := item["title"].(string)
		if strings.TrimSpace(title) == "" {
			continue
		}
		query := title
		if len(query) > 120 {
			query = query[:120]
		}

		// Through the tool layer, not the provider. Brave is a paid call, and
		// calling SearchBraveCached directly meant the daily opinion spent real
		// money that appeared in no usage record and was charged to nobody —
		// there was no identity to charge it to until Micro had an account.
		// ExecuteToolAs is what makes this agent a caller like any other:
		// counted, scoped, and chargeable.
		text, failed, err := api.ExecuteToolAs(auth.MicroID, "web_search",
			map[string]any{"query": query})
		if err != nil || failed || strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Fprintf(&sb, "### Research: %s\n%s\n", title, strings.TrimSpace(text))
		if full := fetchArticleContent(firstURL(text)); full != "" {
			fmt.Fprintf(&sb, "\nFull article:\n%s\n", full)
		}
		sb.WriteString("\n")
		time.Sleep(500 * time.Millisecond)
	}
	return sb.String()
}

// researchStories is how many of a category's top stories get the web-research
// treatment. Each one is a paid search plus a fetch, so this is a budget.
const researchStories = 3

// fetchArticleContent reads one article in full, through the tool layer.
//
// web_fetch is priced at zero, so nothing is charged — but it is still this
// agent doing something, and an agent whose free calls are invisible is only
// half attributed. The counter is the point as much as the charge.
func fetchArticleContent(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	body, failed, err := api.ExecuteToolAs(auth.MicroID, "web_fetch",
		map[string]any{"url": rawURL})
	if err != nil || failed {
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

// firstURL pulls the first link out of a search result, which is prose now
// rather than a struct. Nothing is parsed beyond finding a URL to read: if the
// shape changes, the research step loses its full article and keeps its
// summaries rather than breaking.
func firstURL(text string) string {
	i := strings.Index(text, "http")
	if i < 0 {
		return ""
	}
	end := strings.IndexAny(text[i:], " \n\t)")
	if end < 0 {
		return strings.TrimRight(text[i:], ".,")
	}
	return strings.TrimRight(text[i:i+end], ".,")
}

func opinionTodayKey() string {
	return time.Now().Format("2006-01-02")
}
