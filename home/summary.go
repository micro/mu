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
	summaryMu       sync.RWMutex
	summaryCache    string
	summaryCachedAt time.Time
	summaryTTL      = 10 * time.Minute
	summaryRunning  bool
)

// StartSummaryLoop generates the home summary in the background on a timer.
func StartSummaryLoop() {
	go func() {
		generateSummary()
		for {
			time.Sleep(summaryTTL)
			generateSummary()
		}
	}()
}

func SummaryHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RespondJSON(w, map[string]string{"summary": ""})
		return
	}

	summaryMu.RLock()
	s := summaryCache
	summaryMu.RUnlock()

	// Personalise: lead with the viewer's own signals (cheap, per-request, no
	// per-user LLM call), then the shared news/markets briefing.
	var facts []string
	if unread := mail.GetUnreadCount(sess.Account); unread > 0 {
		suffix := "s"
		if unread == 1 {
			suffix = ""
		}
		facts = append(facts, fmt.Sprintf("%d unread email%s", unread, suffix))
	}

	brief := s
	if len(facts) > 0 {
		prefix := strings.Join(facts, " · ")
		if brief != "" {
			brief = prefix + ". " + brief
		} else {
			brief = prefix + "."
		}
	}

	app.RespondJSON(w, map[string]string{"summary": brief})
}

func generateSummary() {
	summaryMu.Lock()
	if summaryRunning {
		summaryMu.Unlock()
		return
	}
	summaryRunning = true
	summaryMu.Unlock()

	defer func() {
		summaryMu.Lock()
		summaryRunning = false
		summaryMu.Unlock()
	}()

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

	if len(parts) == 0 {
		return
	}

	context := strings.Join(parts, ". ")
	result, err := ai.Ask(&ai.Prompt{
		System:    "Write a 1-2 sentence briefing based on this context. Be conversational and concise. No bullet points.",
		Question:  context,
		Model:     ai.BackgroundModel(),
		Priority:  ai.PriorityLow,
		Caller:    "home-summary",
		MaxTokens: 256,
	})
	if err != nil {
		return
	}

	summaryMu.Lock()
	summaryCache = strings.TrimSpace(result)
	summaryCachedAt = time.Now()
	summaryMu.Unlock()
}
