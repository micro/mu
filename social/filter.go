package social

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/data"
)

// DismissedRule is a learned filter rule from a dismissed thread.
// Captures WHY the thread was irrelevant so the reasoning can be
// applied to future content.
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
// to the filter. Uses AI to extract the reason so the learning is
// based on sentiment, not just keywords.
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

// GetDismissedRules returns the learned rules for display/debugging
func GetDismissedRules() []DismissedRule {
	blockMu.RLock()
	defer blockMu.RUnlock()
	out := make([]DismissedRule, len(dismissed))
	copy(out, dismissed)
	return out
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
		return "dismissed by moderator"
	}
	return strings.TrimSpace(resp)
}
