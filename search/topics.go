package search

import (
	"strings"
	"sync"
	"time"

	"mu/data"
)

// topicCache holds extracted topics from recent content, refreshed periodically.
var topicCache struct {
	sync.RWMutex
	topics  []string
	updated time.Time
}

const topicCacheTTL = 30 * time.Minute
const maxTopics = 12

// GetTopics returns current topic suggestions extracted from recent indexed content.
func GetTopics() []string {
	topicCache.RLock()
	if time.Since(topicCache.updated) < topicCacheTTL && len(topicCache.topics) > 0 {
		topics := topicCache.topics
		topicCache.RUnlock()
		return topics
	}
	topicCache.RUnlock()

	topics := extractTopics()

	topicCache.Lock()
	topicCache.topics = topics
	topicCache.updated = time.Now()
	topicCache.Unlock()

	return topics
}

// Common words to skip when extracting topics
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
	"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "can": true,
	"not": true, "no": true, "it": true, "its": true, "this": true, "that": true,
	"than": true, "then": true, "so": true, "if": true, "as": true, "what": true,
	"how": true, "why": true, "who": true, "which": true, "when": true, "where": true,
	"up": true, "out": true, "about": true, "into": true, "over": true, "after": true,
	"new": true, "says": true, "said": true, "just": true, "more": true, "most": true,
	"also": true, "now": true, "get": true, "gets": true, "got": true, "go": true,
	"here": true, "there": true, "all": true, "some": true, "any": true, "many": true,
	"much": true, "very": true, "like": true, "us": true, "we": true, "our": true,
	"they": true, "them": true, "their": true, "he": true, "she": true, "his": true,
	"her": true, "you": true, "your": true, "my": true, "me": true, "i": true,
	"am": true, "these": true, "those": true,
	"each": true, "every": true, "both": true, "few": true, "other": true,
	"such": true, "only": true, "own": true, "same": true, "too": true,
	"first": true, "last": true, "while": true, "still": true, "back": true,
	"make": true, "makes": true, "made": true, "take": true, "takes": true,
	"set": true, "via": true, "per": true, "even": true, "well": true,
	"top": true, "big": true, "best": true, "one": true, "two": true, "three": true,
}

// extractTopics pulls recent news and blog content and extracts distinctive topic phrases.
func extractTopics() []string {
	// Gather recent content titles
	var titles []string

	// News items (most current events)
	newsItems := data.GetByType("news", 50)
	for _, item := range newsItems {
		titles = append(titles, item.Title)
	}

	// Blog posts
	blogItems := data.GetByType("blog", 20)
	for _, item := range blogItems {
		titles = append(titles, item.Title)
	}

	// Video titles
	videoItems := data.GetByType("video", 20)
	for _, item := range videoItems {
		titles = append(titles, item.Title)
	}

	// Extract meaningful phrases from titles
	// Strategy: look for capitalized multi-word phrases (proper nouns / named entities)
	// and high-frequency distinctive single words
	phraseCount := make(map[string]int)
	wordCount := make(map[string]int)

	for _, title := range titles {
		// Extract 2-3 word capitalized phrases (named entities like "Iran War", "Federal Reserve")
		words := strings.Fields(title)
		for i := 0; i < len(words); i++ {
			w := cleanWord(words[i])
			wLower := strings.ToLower(w)
			if len(w) < 3 || stopWords[wLower] {
				continue
			}
			// Count individual distinctive words
			if isCapitalized(w) || len(w) > 5 {
				wordCount[w]++
			}

			// Look for 2-word capitalized phrases
			if i+1 < len(words) && isCapitalized(w) {
				next := cleanWord(words[i+1])
				if isCapitalized(next) && len(next) >= 2 && !stopWords[strings.ToLower(next)] {
					phrase := w + " " + next
					phraseCount[phrase]++
				}
			}
		}
	}

	// Score and rank: phrases score higher than individual words
	type scored struct {
		term  string
		score int
	}
	var candidates []scored

	for phrase, count := range phraseCount {
		if count >= 1 {
			candidates = append(candidates, scored{phrase, count * 3})
		}
	}
	for word, count := range wordCount {
		if count >= 2 {
			// Skip if already covered by a phrase
			coveredByPhrase := false
			for phrase := range phraseCount {
				if strings.Contains(strings.ToLower(phrase), strings.ToLower(word)) {
					coveredByPhrase = true
					break
				}
			}
			if !coveredByPhrase {
				candidates = append(candidates, scored{word, count})
			}
		}
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Deduplicate: skip terms that are substrings of already-selected terms
	var topics []string
	for _, c := range candidates {
		if len(topics) >= maxTopics {
			break
		}
		cLower := strings.ToLower(c.term)
		duplicate := false
		for _, existing := range topics {
			eLower := strings.ToLower(existing)
			if strings.Contains(eLower, cLower) || strings.Contains(cLower, eLower) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			topics = append(topics, c.term)
		}
	}

	return topics
}

// cleanWord strips leading/trailing punctuation from a word.
func cleanWord(w string) string {
	return strings.Trim(w, ".,;:!?\"'()[]{}—–-/\\|")
}

// isCapitalized returns true if the word starts with an uppercase letter.
func isCapitalized(w string) bool {
	if len(w) == 0 {
		return false
	}
	return w[0] >= 'A' && w[0] <= 'Z'
}
