package social

import (
	"encoding/json"
	"strings"
	"sync"

	"mu/app"
	"mu/data"
)

// blocklist stores title fragments that have been dismissed.
// When a seeded thread is marked as spam/dismissed, its normalised title
// is added here so similar stories are never seeded again.
var (
	blockMu    sync.RWMutex
	blocklist  []string
	blockFile  = "social_blocklist.json"
)

func loadBlocklist() {
	b, err := data.LoadFile(blockFile)
	if err != nil {
		return
	}
	blockMu.Lock()
	defer blockMu.Unlock()
	json.Unmarshal(b, &blocklist)
}

func saveBlocklist() {
	blockMu.RLock()
	defer blockMu.RUnlock()
	data.SaveJSON(blockFile, blocklist)
}

// DismissThread adds a thread's title to the blocklist so similar stories
// aren't seeded in the future, then hides it. Called when admin marks
// a seeded thread as irrelevant.
func DismissThread(threadID string) {
	mutex.RLock()
	t := getThread(threadID)
	if t == nil {
		mutex.RUnlock()
		return
	}
	title := t.Title
	mutex.RUnlock()

	// Add normalised title words to blocklist
	key := normTitle(title)
	if key != "" && !isBlocked(key) {
		blockMu.Lock()
		blocklist = append(blocklist, key)
		blockMu.Unlock()
		saveBlocklist()
		app.Log("social", "Dismissed and blocked: %q", key)
	}
}

// isBlocked checks if a title matches any entry in the blocklist
func isBlocked(titleKey string) bool {
	blockMu.RLock()
	defer blockMu.RUnlock()
	for _, blocked := range blocklist {
		// Match if any blocked phrase appears in the title or vice versa
		if strings.Contains(titleKey, blocked) || strings.Contains(blocked, titleKey) {
			return true
		}
	}
	return false
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
