package home

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/mail"
	"mu/markets"
	"mu/news"
)

var (
	summaryMu    sync.RWMutex
	summaryCache string
	summaryCachedAt time.Time
	summaryTTL   = 10 * time.Minute
)

func SummaryHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RespondJSON(w, map[string]string{"summary": ""})
		return
	}

	summaryMu.RLock()
	if time.Since(summaryCachedAt) < summaryTTL && summaryCache != "" {
		s := summaryCache
		summaryMu.RUnlock()
		app.RespondJSON(w, map[string]string{"summary": s})
		return
	}
	summaryMu.RUnlock()

	// Build context
	var parts []string

	feed := news.GetFeed()
	if len(feed) > 5 {
		feed = feed[:5]
	}
	if len(feed) > 0 {
		var headlines []string
		for _, p := range feed {
			headlines = append(headlines, p.Title)
		}
		parts = append(parts, "Top news: "+strings.Join(headlines, "; "))
	}

	priceData := markets.GetAllPriceData()
	if len(priceData) > 0 {
		var movers []string
		for symbol, pd := range priceData {
			if pd.Change24h > 3 || pd.Change24h < -3 {
				movers = append(movers, fmt.Sprintf("%s %.1f%%", symbol, pd.Change24h))
			}
		}
		if len(movers) > 0 {
			parts = append(parts, "Market movers: "+strings.Join(movers, ", "))
		}
	}

	if unread := mail.GetUnreadCount(sess.Account); unread > 0 {
		parts = append(parts, fmt.Sprintf("%d unread email(s)", unread))
	}

	if len(parts) == 0 {
		app.RespondJSON(w, map[string]string{"summary": ""})
		return
	}

	context := strings.Join(parts, ". ")
	result, err := ai.Ask(&ai.Prompt{
		System:    "Write a 1-2 sentence personal briefing based on this context. Be conversational and concise. No bullet points. Just a natural sentence or two about what's happening right now.",
		Question:  context,
		Model:     ai.BackgroundModel(),
		Priority:  ai.PriorityLow,
		Caller:    "home-summary",
		MaxTokens: 256,
	})
	if err != nil {
		app.RespondJSON(w, map[string]string{"summary": ""})
		return
	}

	summaryMu.Lock()
	summaryCache = strings.TrimSpace(result)
	summaryCachedAt = time.Now()
	summaryMu.Unlock()

	app.RespondJSON(w, map[string]string{"summary": strings.TrimSpace(result)})
}
