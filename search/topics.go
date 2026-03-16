package search

import (
	"strings"
	"sync"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/data"
)

// topicCache holds LLM-generated topics from recent content.
var topicCache struct {
	sync.RWMutex
	topics  []string
	updated time.Time
}

const topicCacheTTL = 1 * time.Hour
const maxTopics = 10

// GetTopics returns current topic suggestions. Returns cached results immediately;
// regeneration happens in the background.
func GetTopics() []string {
	topicCache.RLock()
	topics := topicCache.topics
	stale := time.Since(topicCache.updated) > topicCacheTTL
	topicCache.RUnlock()

	if stale {
		go regenerateTopics()
	}

	return topics
}

// StartTopics loads cached topics from disk and kicks off background generation.
func StartTopics() {
	var cached []string
	if err := data.LoadJSON("web_topics.json", &cached); err == nil && len(cached) > 0 {
		topicCache.Lock()
		topicCache.topics = cached
		topicCache.updated = time.Now()
		topicCache.Unlock()
		app.Log("search", "Loaded %d cached topics from disk", len(cached))
	}
	go regenerateTopics()
}

var topicGenerating sync.Mutex

func regenerateTopics() {
	// Prevent concurrent generation
	if !topicGenerating.TryLock() {
		return
	}
	defer topicGenerating.Unlock()

	// Gather recent content titles
	var headlines []string

	newsItems := data.GetByType("news", 30)
	for _, item := range newsItems {
		headlines = append(headlines, item.Title)
	}

	blogItems := data.GetByType("blog", 10)
	for _, item := range blogItems {
		headlines = append(headlines, item.Title)
	}

	videoItems := data.GetByType("video", 10)
	for _, item := range videoItems {
		headlines = append(headlines, item.Title)
	}

	if len(headlines) == 0 {
		return
	}

	// Truncate to avoid sending too much context
	if len(headlines) > 40 {
		headlines = headlines[:40]
	}

	prompt := &ai.Prompt{
		System: `You extract trending search topics from headlines. Return ONLY a newline-separated list of topics. Each topic should be:
- A specific person, place, event, technology, or concept (e.g. "Iran nuclear talks", "Ramadan 2026", "Bitcoin ETF", "OpenAI Sora")
- 1-4 words, suitable as a web search query
- Something a reader would want to learn more about
- No generic words, no duplicates, no numbering, no punctuation
Return exactly 10 topics, one per line. Nothing else.`,
		Question: "Extract 10 trending search topics from these recent headlines:\n\n" + strings.Join(headlines, "\n"),
		Priority: ai.PriorityLow,
	}

	resp, err := ai.Ask(prompt)
	if err != nil {
		app.Log("search", "Failed to generate topics: %v", err)
		return
	}

	// Parse response: one topic per line
	var topics []string
	for _, line := range strings.Split(resp, "\n") {
		topic := strings.TrimSpace(line)
		// Skip empty lines, numbered lines, or lines that are too long/short
		if topic == "" || len(topic) < 3 || len(topic) > 60 {
			continue
		}
		// Strip leading numbering like "1. " or "- "
		if len(topic) > 2 && (topic[1] == '.' || topic[1] == ')') {
			topic = strings.TrimSpace(topic[2:])
		}
		if len(topic) > 3 && (topic[2] == '.' || topic[2] == ')') {
			topic = strings.TrimSpace(topic[3:])
		}
		topic = strings.TrimLeft(topic, "- ")
		if topic != "" && len(topic) >= 3 {
			topics = append(topics, topic)
		}
		if len(topics) >= maxTopics {
			break
		}
	}

	if len(topics) == 0 {
		app.Log("search", "LLM returned no parseable topics")
		return
	}

	topicCache.Lock()
	topicCache.topics = topics
	topicCache.updated = time.Now()
	topicCache.Unlock()

	data.SaveJSON("web_topics.json", topics)
	app.Log("search", "Generated %d topics: %v", len(topics), topics)
}
