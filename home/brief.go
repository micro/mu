package home

import (
	"strings"

	"mu/internal/app"
	"mu/markets"
	"mu/news"
	"mu/reminder"
)

// Summary returns the cached at-a-glance summary text (may be empty).
func Summary() string {
	summaryMu.RLock()
	defer summaryMu.RUnlock()
	return summaryCache
}

// MorningBriefHTML assembles a card-rich daily brief from the same live data
// the home dashboard uses: a short AI summary plus the reminder, markets and
// news cards. Returns "" if there is nothing worth showing yet.
//
// It deliberately reuses the self-contained service card renderers so the brief
// stays in sync with the dashboard and the inline chat cards — one source,
// presented as a daily digest.
func MorningBriefHTML() string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h4>Good morning ☀️</h4>`)
	if s := strings.TrimSpace(Summary()); s != "" {
		b.WriteString(app.RenderString(s))
	} else {
		b.WriteString(`<p style="color:#666;margin:0;">Here's your brief for today.</p>`)
	}
	b.WriteString(`</div>`)

	cards := []struct {
		title  string
		render func() string
	}{
		{"☪️ Reminder", reminder.ReminderHTML},
		{"📈 Markets", markets.MarketsHTML},
		{"📰 News", news.Headlines},
	}
	for _, c := range cards {
		if body := strings.TrimSpace(c.render()); body != "" {
			b.WriteString(`<div class="card"><h4>` + c.title + `</h4>` + body + `</div>`)
		}
	}
	return b.String()
}
