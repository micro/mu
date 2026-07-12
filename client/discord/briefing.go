package discord

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/mail"
	"mu/markets"
	"mu/news"
)

// StartBriefingLoop runs a daily morning briefing for all linked users.
func StartBriefingLoop() {
	go func() {
		for {
			now := time.Now()
			// Next 7am UTC
			next := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, time.UTC)
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			time.Sleep(time.Until(next))
			sendMorningBriefings()
		}
	}()
	app.Log("discord", "Morning briefing loop started (daily at 7am UTC)")
}

func sendMorningBriefings() {
	briefing := buildBriefing()
	if briefing == "" {
		return
	}

	// Post to every server's configured briefing channel
	channels := getBriefingChannels()
	for _, channelID := range channels {
		embed := Embed{
			Title:       "☀️ Morning Briefing",
			Description: briefing,
			Color:       ColorGold,
			Footer:      &EmbedFooter{Text: time.Now().Format("Monday, 2 January 2006")},
		}
		sendEmbed(channelID, embed)
	}
	if len(channels) > 0 {
		app.Log("discord", "Posted morning briefing to %d channels", len(channels))
	}

	// Send personal context (unread mail, etc.) as DMs
	linkMu.RLock()
	userMap := make(map[string]string, len(links))
	for discordID, muAccount := range links {
		userMap[discordID] = muAccount
	}
	linkMu.RUnlock()

	for discordID, muAccount := range userMap {
		personal := personalContext(muAccount)
		if personal == "" {
			continue
		}
		dmChannel := getDMChannel(discordID)
		if dmChannel == "" {
			continue
		}
		sendMessage(dmChannel, personal)
	}
}

// briefVoice is the editorial voice for the morning brief: sharp, opinionated,
// and short enough for a chat message — closer to the blog's plain-and-honest
// tone than a headline dump.
const briefVoice = `You are Micro, writing a short morning brief for one sharp, busy person — not a newswire.
From the stories below (already spread across topics), choose only the few that genuinely matter today and say, in one plain line each, what happened and why it's worth knowing. Have a point of view. Be concrete and honest — no hype, no filler, and do not lead with crypto unless it is genuinely the biggest story of the day.
Format for a chat message:
- One short **bold** lead line: the single most important thing.
- Then 2 to 4 one-line bullets, each under 20 words.
- Keep the whole brief under 120 words.
If nothing is genuinely important, say that in one line rather than padding.`

func buildBriefing() string {
	stories := diverseHeadlines(news.GetFeed(), 2, 8)
	if len(stories) == 0 {
		return ""
	}

	var src strings.Builder
	src.WriteString("Stories across topics:\n")
	for _, p := range stories {
		line := p.Title
		if d := clipRunes(firstLine(p.Description), 160); d != "" {
			line += " — " + d
		}
		cat := strings.TrimSpace(p.Category)
		if cat != "" {
			fmt.Fprintf(&src, "- [%s] %s\n", cat, line)
		} else {
			fmt.Fprintf(&src, "- %s\n", line)
		}
	}
	if mv := bigMoversLine(); mv != "" {
		src.WriteString("\nNotable market moves: " + mv + "\n")
	}

	result, err := ai.Ask(&ai.Prompt{
		System:    briefVoice,
		Question:  src.String(),
		Model:     ai.BackgroundModel(),
		Priority:  ai.PriorityLow,
		Caller:    "morning-briefing",
		MaxTokens: 400,
	})
	if err != nil || strings.TrimSpace(result) == "" {
		// Fallback: a plain, still-diverse headline list.
		var b strings.Builder
		for _, p := range stories {
			fmt.Fprintf(&b, "- %s\n", p.Title)
		}
		return strings.TrimSpace(b.String())
	}
	return strings.TrimSpace(result)
}

// diverseHeadlines spreads the pick across news categories so a single
// high-volume feed (crypto, finance) can't dominate. It round-robins across
// categories in feed-recency order, deferring crypto to last, taking up to
// perCat per category and total overall.
func diverseHeadlines(feed []*news.Post, perCat, total int) []*news.Post {
	byCat := map[string][]*news.Post{}
	var order []string
	for _, p := range feed {
		c := strings.ToLower(strings.TrimSpace(p.Category))
		if c == "" {
			c = "other"
		}
		if _, seen := byCat[c]; !seen {
			order = append(order, c)
		}
		if len(byCat[c]) < perCat {
			byCat[c] = append(byCat[c], p)
		}
	}
	// Pick order: every category except crypto (in feed order), then crypto.
	var cats []string
	for _, c := range order {
		if c != "crypto" {
			cats = append(cats, c)
		}
	}
	for _, c := range order {
		if c == "crypto" {
			cats = append(cats, c)
		}
	}
	var out []*news.Post
	for round := 0; round < perCat && len(out) < total; round++ {
		for _, c := range cats {
			if round < len(byCat[c]) {
				out = append(out, byCat[c][round])
				if len(out) >= total {
					return out
				}
			}
		}
	}
	return out
}

// bigMoversLine returns the top few genuinely large 24h moves (>=8%), or "".
func bigMoversLine() string {
	type mv struct {
		sym string
		ch  float64
	}
	var movers []mv
	for symbol, pd := range markets.GetAllPriceData() {
		if pd.Change24h >= 8 || pd.Change24h <= -8 {
			movers = append(movers, mv{symbol, pd.Change24h})
		}
	}
	if len(movers) == 0 {
		return ""
	}
	sort.Slice(movers, func(i, j int) bool { return math.Abs(movers[i].ch) > math.Abs(movers[j].ch) })
	if len(movers) > 3 {
		movers = movers[:3]
	}
	var s []string
	for _, m := range movers {
		dir := "up"
		if m.ch < 0 {
			dir = "down"
		}
		s = append(s, fmt.Sprintf("%s %s %.0f%%", m.sym, dir, math.Abs(m.ch)))
	}
	return strings.Join(s, ", ")
}

// firstLine returns the first line of s, trimmed.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// clipRunes truncates s to at most n runes, adding an ellipsis when cut.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func personalContext(accountID string) string {
	var extras []string

	if unread := mail.GetUnreadCount(accountID); unread > 0 {
		extras = append(extras, fmt.Sprintf("📬 You have %d unread email%s.", unread, func() string {
			if unread == 1 {
				return ""
			}
			return "s"
		}()))
	}

	if len(extras) > 0 {
		return strings.Join(extras, "\n")
	}
	return ""
}

// SummariseEmail generates a short summary of an email using DeepSeek.
func SummariseEmail(from, subject, body string) string {
	if len(body) > 2000 {
		body = body[:2000]
	}
	prompt := fmt.Sprintf("From: %s\nSubject: %s\n\n%s", from, subject, body)
	result, err := ai.Ask(&ai.Prompt{
		System:   "Summarise this email in one sentence. Be specific — include the key information or action required.",
		Question: prompt,
		Model:    ai.BackgroundModel(),
		Priority: ai.PriorityLow,
		Caller:   "email-summary",
	})
	if err != nil {
		return subject
	}
	return result
}
