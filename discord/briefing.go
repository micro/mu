package discord

import (
	"fmt"
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

func buildBriefing() string {
	var parts []string

	// Top news (lead with this)
	feed := news.GetFeed()
	if len(feed) > 10 {
		feed = feed[:10]
	}
	if len(feed) > 0 {
		var headlines []string
		for _, p := range feed {
			headlines = append(headlines, "- "+p.Title)
		}
		parts = append(parts, "**Headlines:**\n"+strings.Join(headlines, "\n"))
	}

	// Market movers (only significant moves, not the full ticker)
	priceData := markets.GetAllPriceData()
	if len(priceData) > 0 {
		var movers []string
		for symbol, pd := range priceData {
			if pd.Change24h > 5 || pd.Change24h < -5 {
				dir := "up"
				if pd.Change24h < 0 {
					dir = "down"
				}
				movers = append(movers, fmt.Sprintf("%s %s %.1f%%", symbol, dir, pd.Change24h))
			}
		}
		if len(movers) > 0 {
			parts = append(parts, "**Market movers:** "+strings.Join(movers, ", "))
		}
	}
	if len(feed) > 0 {
		var headlines []string
		for _, p := range feed {
			headlines = append(headlines, "- "+p.Title)
		}
		parts = append(parts, "**Headlines:**\n"+strings.Join(headlines, "\n"))
	}

	if len(parts) == 0 {
		return ""
	}

	// Use DeepSeek to synthesise into a concise briefing
	raw := strings.Join(parts, "\n\n")
	result, err := ai.Ask(&ai.Prompt{
		System:   "You are a personal assistant writing a morning briefing. Lead with the most important news stories across all topics — tech, world events, business, science. Only mention markets if there are significant moves (5%+). Keep it conversational, 3-5 sentences. Don't lead with crypto unless it's genuinely the biggest story.",
		Question: raw,
		Model:    ai.BackgroundModel(),
		Priority: ai.PriorityLow,
		Caller:   "morning-briefing",
	})
	if err != nil {
		return raw
	}
	return result
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
