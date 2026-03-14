package social

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"mu/ai"
	"mu/app"
	"mu/data"
)

// DismissedRule is a learned filter rule from a dismissed thread.
// Instead of matching keywords, it captures WHY the thread was irrelevant
// so the AI can apply the same reasoning to future stories.
type DismissedRule struct {
	Title  string `json:"title"`  // original dismissed title
	Reason string `json:"reason"` // AI-extracted reason it was irrelevant
}

var (
	blockMu   sync.RWMutex
	dismissed []DismissedRule
	blockFile = "social_blocklist.json"
)

func loadBlocklist() {
	b, err := data.LoadFile(blockFile)
	if err != nil {
		return
	}
	blockMu.Lock()
	defer blockMu.Unlock()
	json.Unmarshal(b, &dismissed)
}

func saveBlocklist() {
	blockMu.RLock()
	defer blockMu.RUnlock()
	data.SaveJSON(blockFile, dismissed)
}

// DismissThread learns why a thread is irrelevant and adds the rule
// to the filter. Uses AI to extract the reason so future stories with
// the same sentiment/category are blocked, not just keyword matches.
func DismissThread(threadID string) {
	mutex.RLock()
	t := getThread(threadID)
	if t == nil {
		mutex.RUnlock()
		return
	}
	title := t.Title
	content := t.Content
	mutex.RUnlock()

	// Use AI to understand WHY this was dismissed
	reason := learnDismissReason(title, content)

	blockMu.Lock()
	dismissed = append(dismissed, DismissedRule{
		Title:  title,
		Reason: reason,
	})
	blockMu.Unlock()
	saveBlocklist()

	app.Log("social", "Dismissed %q — learned: %s", title, reason)
}

// learnDismissReason asks AI to extract why a story is irrelevant
func learnDismissReason(title, content string) string {
	prompt := &ai.Prompt{
		System: `You are a content filter for a news discussion platform focused on technology, politics, finance, religion, and global affairs.

A moderator has dismissed a story as irrelevant. Explain in ONE short sentence WHY this story is not suitable for discussion. Focus on the category/type of content, not the specific details.

Examples:
- "Celebrity entertainment — TV presenter chat show review"
- "Sports results — football match coverage"
- "Tabloid gossip — personal life of public figure"
- "Local crime — individual crime story with no broader significance"
- "Entertainment industry — movie/album/show review"

Respond with ONLY the one-line reason.`,
		Question: fmt.Sprintf("Title: %s\n\nContent: %s", title, content),
		Priority: ai.PriorityLow,
	}

	resp, err := ai.Ask(prompt)
	if err != nil {
		// Fallback to a generic reason
		return "dismissed by moderator"
	}
	return strings.TrimSpace(resp)
}

// shouldSeed checks a candidate story against the learned dismiss rules.
// Uses AI to determine if the story matches the sentiment/category of
// previously dismissed content. Returns true if the story should be seeded.
func shouldSeed(title string) bool {
	blockMu.RLock()
	rules := dismissed
	blockMu.RUnlock()

	if len(rules) == 0 {
		return true
	}

	// First do a cheap keyword check against noise list
	if isNoise(title) {
		return false
	}

	// Build the learned rules context for AI screening
	var ruleLines []string
	for _, r := range rules {
		ruleLines = append(ruleLines, fmt.Sprintf("- %q → %s", r.Title, r.Reason))
	}

	prompt := &ai.Prompt{
		System: `You are a content filter for a news discussion platform. The platform covers technology, development, politics, religion (Islam), finance, and crypto. It does NOT cover entertainment, sports, celebrity gossip, or tabloid news.

You have learned from previously dismissed stories. Based on the patterns below, decide if a new story should be POSTED or BLOCKED.

Previously dismissed (with reasons):
` + strings.Join(ruleLines, "\n") + `

Respond with ONLY one word: POST or BLOCK`,
		Question: fmt.Sprintf("Should this story be posted?\n\nTitle: %s", title),
		Priority: ai.PriorityLow,
	}

	resp, err := ai.Ask(prompt)
	if err != nil {
		// On error, allow it through — better to over-include than miss important stories
		return true
	}

	resp = strings.TrimSpace(strings.ToUpper(resp))
	if resp == "BLOCK" {
		app.Log("social", "AI filter blocked: %s", title)
		return false
	}
	return true
}

// relevantTopics are the only news categories we seed discussions for.
// Entertainment, celebrity, and generic local news are filtered out.
var relevantTopics = map[string]bool{
	"Tech":     true,
	"Dev":      true,
	"Politics": true,
	"Islam":    true,
	"Finance":  true,
	"Crypto":   true,
}

// noiseKeywords are title fragments that indicate entertainment, sports,
// celebrity, or other noise that doesn't belong in discussions.
var noiseKeywords = []string{
	"football", "premier league", "champions league", "fifa",
	"cricket", "rugby", "tennis", "f1", "formula 1",
	"olympic", "world cup", "transfer", "goalkeeper",
	"celebrity", "reality tv", "chat show", "soap opera",
	"love island", "strictly", "x factor", "big brother",
	"kardashian", "entertainment", "box office", "movie review",
	"album review", "red carpet", "gossip",
}

// isRelevantForDiscussion checks if a news story belongs to a topic
// worth seeding as a social discussion thread.
func isRelevantForDiscussion(category string) bool {
	return relevantTopics[category]
}

// isNoise checks if a title contains entertainment/sports keywords
func isNoise(title string) bool {
	lower := strings.ToLower(title)
	for _, kw := range noiseKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
