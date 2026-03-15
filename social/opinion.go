package social

import (
	"fmt"
	"strings"
	"time"

	"mu/ai"
	"mu/app"
	"mu/news"
	"mu/news/markets"
	"mu/search"
	"mu/video"
)

// generateOpinion gathers all available data (news, markets, videos),
// cross-references with web search for deeper context, and uses AI to
// produce an opinion piece reflecting grounded sentiment analysis.
// Returns the title (without "Opinion: " prefix) and the body content.
func generateOpinion() (string, string, error) {
	context := gatherOpinionContext()
	if context == "" {
		return "", "", fmt.Errorf("no content available")
	}

	// Do web research on the top stories to cross-reference
	webResearch := researchTopStories()

	fullContext := context
	if webResearch != "" {
		fullContext += "\n\n## Web Research & Cross-References\n\n" + webResearch
	}

	prompt := &ai.Prompt{
		System: `You are a senior opinion writer for Mu, an independent platform built in the UK. You produce a single daily opinion piece that reflects on the day's events with depth, nuance, and original thinking.

Your perspective:
- You are grounded by Islamic values — honesty, justice, accountability, and a rejection of greed-driven decision making. You don't preach, but your moral compass is clear.
- You have an engineering mindset — you look at systems, incentives, and data rather than taking narratives at face value.
- You are sceptical of media bias from ALL sources — Western, Eastern, state-run, corporate. You don't take any source as gospel truth. You cross-reference and think critically.
- You are not contrarian for the sake of it, but you recognise that mainstream narratives often serve power structures rather than truth.
- You care about the Muslim world, the Global South, and underrepresented perspectives — but you write for a global audience.

What you produce:
- A sharp, opinionated take on the dominant theme of the day
- Connect the dots between events, market movements, and geopolitics
- Call out media bias, missing context, or misleading framing where you see it
- Offer your own grounded assessment — what's really happening and why
- If sentiment is skewing one way, explain whether that's justified or manufactured

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
- CRITICAL: Keep under 2500 characters total (title + body).`,
		Question: fullContext,
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		return "", "", err
	}

	response = strings.TrimSpace(app.StripLatexDollars(response))

	// Parse: first line is title, rest is body
	parts := strings.SplitN(response, "\n", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected response format")
	}

	title := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])

	// Clean up title — remove quotes, "Opinion:" prefix if AI added it
	title = strings.TrimPrefix(title, "Opinion: ")
	title = strings.TrimPrefix(title, "Opinion:")
	title = strings.Trim(title, `"'`)

	if title == "" || body == "" {
		return "", "", fmt.Errorf("empty title or body")
	}

	return title, body, nil
}

// gatherOpinionContext collects news, markets, and video data for opinion generation.
func gatherOpinionContext() string {
	var sb strings.Builder

	// News
	feed := news.GetFeed()
	if len(feed) > 0 {
		sb.WriteString("## Today's News\n\n")
		byCategory := make(map[string][]*news.Post)
		for _, item := range feed {
			byCategory[item.Category] = append(byCategory[item.Category], item)
		}
		for category, items := range byCategory {
			sb.WriteString(fmt.Sprintf("### %s\n", category))
			count := 5
			if len(items) < count {
				count = len(items)
			}
			for _, item := range items[:count] {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", item.Title, item.Description))
			}
			sb.WriteString("\n")
		}
	}

	// Markets
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

	// Videos
	videos := video.GetLatestVideos(5)
	if len(videos) > 0 {
		sb.WriteString("## Videos\n\n")
		for _, v := range videos {
			sb.WriteString(fmt.Sprintf("- %s by %s\n", v.Title, v.Channel))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// researchTopStories picks the most important stories from the news feed
// and does web searches to cross-reference them, providing additional context
// that the opinion writer can use to form a more grounded view.
func researchTopStories() string {
	feed := news.GetFeed()
	if len(feed) == 0 {
		return ""
	}

	// Pick up to 3 top stories to research
	limit := 3
	if len(feed) < limit {
		limit = len(feed)
	}

	var sb strings.Builder
	for _, item := range feed[:limit] {
		query := item.Title
		if len(query) > 120 {
			query = query[:120]
		}

		results, err := search.SearchBraveCached(query, 3)
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
		sb.WriteString("\n")

		// Small delay between searches to be respectful to the API
		time.Sleep(500 * time.Millisecond)
	}

	return sb.String()
}
