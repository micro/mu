package news

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
)

type Sentiment struct {
	Title   string   `json:"title"`
	Score   float64  `json:"score"` // -1.0 to 1.0
	Topics  []string `json:"topics"`
	Summary string   `json:"summary"` // one-line
}

var (
	sentimentMu    sync.RWMutex
	sentimentCache = map[string]*Sentiment{} // article URL → sentiment
	sentimentTTL   = 30 * time.Minute
)

// GetSentiment returns cached sentiment for an article URL.
func GetSentiment(url string) *Sentiment {
	sentimentMu.RLock()
	defer sentimentMu.RUnlock()
	return sentimentCache[url]
}

// GetAllSentiments returns all cached sentiments.
func GetAllSentiments() map[string]*Sentiment {
	sentimentMu.RLock()
	defer sentimentMu.RUnlock()
	result := make(map[string]*Sentiment, len(sentimentCache))
	for k, v := range sentimentCache {
		result[k] = v
	}
	return result
}

// StartSentimentLoop runs sentiment tagging every 15 minutes.
func StartSentimentLoop() {
	go func() {
		time.Sleep(30 * time.Second) // let feeds load first
		tagSentiments()
		for {
			time.Sleep(sentimentTTL)
			tagSentiments()
		}
	}()
	app.Log("news", "Sentiment tagging loop started (every %v)", sentimentTTL)
}

func tagSentiments() {
	feed := GetFeed()
	if len(feed) == 0 {
		return
	}

	// Only tag articles we haven't tagged yet
	sentimentMu.RLock()
	var untagged []*Post
	for _, p := range feed {
		if _, ok := sentimentCache[p.URL]; !ok {
			untagged = append(untagged, p)
		}
	}
	sentimentMu.RUnlock()

	if len(untagged) == 0 {
		return
	}

	// Cap at 30 articles per batch to keep context manageable
	if len(untagged) > 30 {
		untagged = untagged[:30]
	}

	// Build one prompt with all articles
	var sb strings.Builder
	for i, p := range untagged {
		sb.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, p.Title, truncate(p.Description, 150)))
	}

	result, err := ai.Ask(&ai.Prompt{
		System: `Tag each article with sentiment and topics. Return ONLY a JSON array with one object per article in order:
[{"score":0.5,"topics":["crypto","regulation"],"summary":"one line summary"},...]

Score: -1.0 (very negative) to 1.0 (very positive), 0 = neutral.
Topics: 1-3 lowercase tags.
Summary: one sentence, max 20 words.`,
		Question:  sb.String(),
		Model:     ai.BackgroundModel(),
		Priority:  ai.PriorityLow,
		Caller:    "news-sentiment",
		MaxTokens: 4096,
	})
	if err != nil {
		app.Log("news", "Sentiment tagging failed: %v", err)
		return
	}

	// Parse response
	start := strings.Index(result, "[")
	end := strings.LastIndex(result, "]")
	if start < 0 || end <= start {
		app.Log("news", "Sentiment response not valid JSON array")
		return
	}

	var tags []Sentiment
	if err := json.Unmarshal([]byte(result[start:end+1]), &tags); err != nil {
		app.Log("news", "Sentiment parse error: %v", err)
		return
	}

	// Store results
	sentimentMu.Lock()
	for i, tag := range tags {
		if i >= len(untagged) {
			break
		}
		tag.Title = untagged[i].Title
		sentimentCache[untagged[i].URL] = &tag
	}
	// Evict old entries
	if len(sentimentCache) > 500 {
		for k := range sentimentCache {
			delete(sentimentCache, k)
			if len(sentimentCache) <= 250 {
				break
			}
		}
	}
	sentimentMu.Unlock()

	app.Log("news", "Tagged %d articles with sentiment", len(tags))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
