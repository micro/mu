package search

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// WebSearchText runs a cached web search and returns model-ready results.
// It is the AI-first accessor behind the web_search agent tool.
func WebSearchText(query string, limit int) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "No query provided."
	}
	if limit <= 0 || limit > 10 {
		limit = 6
	}
	results, err := SearchBraveCached(query, limit)
	if err != nil {
		return "Web search is unavailable right now."
	}
	if len(results) == 0 {
		return fmt.Sprintf("No web results for %q.", query)
	}

	return formatWebSearchResults(query, results)
}

func formatWebSearchResults(query string, results []BraveResult) string {
	results = groundNewsWebResults(query, results)
	confidence := webSearchConfidence(query, results)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Web results for %q:\n", query)
	fmt.Fprintf(&sb, "Query intent: answer the user's original query %q; do not replace it with a broader or different meaning.\n", query)
	if confidence == "low" {
		fmt.Fprintf(&sb, "Confidence: low — the returned snippets only partially match the query intent. Say that the search results do not clearly support an answer and ask the user to refine the query before making unsupported claims.\n")
	} else {
		fmt.Fprintf(&sb, "Confidence: %s — synthesize only what the listed sources support.\n", confidence)
	}
	if isNewsLikeWebQuery(query) {
		fmt.Fprintf(&sb, "Grounding rule: treat each source as evidence only for facts named in its title or snippet; do not turn generic topic/category pages or weak snippets into specific news headlines. If the snippets are thin, say the evidence is limited.\n")
	}
	fmt.Fprintf(&sb, "Sources:\n")
	for i, r := range results {
		desc := strings.Join(strings.Fields(r.Description), " ")
		if len(desc) > 220 {
			desc = desc[:220] + "…"
		}
		title := strings.TrimSpace(r.Title)
		if title == "" {
			title = sourceHost(r.URL)
		}
		fmt.Fprintf(&sb, "%d. %s — %s (%s)\n", i+1, title, desc, r.URL)
	}
	return sb.String()
}

func groundNewsWebResults(query string, results []BraveResult) []BraveResult {
	if !isNewsLikeWebQuery(query) {
		return results
	}
	terms := meaningfulQueryTerms(query)
	if len(terms) == 0 {
		return results
	}
	storyLike := make([]BraveResult, 0, len(results))
	filtered := make([]BraveResult, 0, len(results))
	for _, r := range results {
		desc := strings.TrimSpace(r.Description)
		if desc == "" {
			continue
		}
		haystack := strings.ToLower(r.Title + " " + desc + " " + r.URL)
		matchedTopic := false
		for term := range terms {
			if term == "news" || term == "latest" || term == "today" || term == "current" {
				continue
			}
			if strings.Contains(haystack, term) {
				matchedTopic = true
				break
			}
		}
		if matchedTopic {
			filtered = append(filtered, r)
			if isArticleLevelNewsResult(r) {
				storyLike = append(storyLike, r)
			}
		}
	}
	if len(storyLike) > 0 {
		return storyLike
	}
	if len(filtered) == 0 {
		return results
	}
	return filtered
}

func isArticleLevelNewsResult(r BraveResult) bool {
	if strings.TrimSpace(r.URL) == "" {
		return false
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		return false
	}
	path := strings.Trim(strings.ToLower(u.EscapedPath()), "/")
	if path == "" {
		return false
	}
	segments := strings.Split(path, "/")
	last := strings.TrimSuffix(segments[len(segments)-1], ".html")
	last = strings.TrimSuffix(last, ".amp")
	genericLast := map[string]struct{}{
		"ai": {}, "artificial-intelligence": {}, "artificial_intelligence": {},
		"news": {}, "tech": {}, "technology": {}, "updates": {},
		"category": {}, "topics": {}, "topic": {}, "reviews": {},
	}
	if _, ok := genericLast[last]; ok && len(segments) <= 2 {
		return false
	}
	if strings.Contains(strings.ToLower(r.Title), "|") {
		left := strings.TrimSpace(strings.Split(strings.ToLower(r.Title), "|")[0])
		if _, ok := genericLast[strings.ReplaceAll(left, " ", "-")]; ok {
			return false
		}
	}
	for _, segment := range segments {
		if len(segment) >= 4 && containsDigit(segment) {
			return true
		}
	}
	storyWords := 0
	for _, field := range strings.FieldsFunc(last, func(r rune) bool { return r == '-' || r == '_' || r == '+' }) {
		if len(field) > 2 {
			storyWords++
		}
	}
	return storyWords >= 3 || len(segments) >= 3
}

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func isNewsLikeWebQuery(query string) bool {
	lower := strings.ToLower(query)
	return strings.Contains(lower, "news") || strings.Contains(lower, "latest") || strings.Contains(lower, "today") || strings.Contains(lower, "current")
}

func webSearchConfidence(query string, results []BraveResult) string {
	terms := meaningfulQueryTerms(query)
	if len(terms) == 0 || len(results) == 0 {
		return "low"
	}
	matches := 0
	for term := range terms {
		for _, r := range results {
			haystack := strings.ToLower(r.Title + " " + r.Description + " " + r.URL)
			if strings.Contains(haystack, term) {
				matches++
				break
			}
		}
	}
	ratio := float64(matches) / float64(len(terms))
	switch {
	case ratio >= 0.75:
		return "high"
	case ratio >= 0.67:
		return "medium"
	default:
		return "low"
	}
}

func meaningfulQueryTerms(query string) map[string]struct{} {
	stop := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "are": {}, "for": {}, "from": {}, "how": {}, "is": {}, "me": {}, "of": {}, "on": {}, "or": {}, "search": {}, "show": {}, "the": {}, "to": {}, "web": {}, "what": {}, "whats": {}, "with": {},
	}
	terms := make(map[string]struct{})
	for _, field := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		field = strings.TrimSpace(field)
		if len(field) < 2 {
			continue
		}
		if _, ok := stop[field]; ok {
			continue
		}
		terms[field] = struct{}{}
	}
	return terms
}

func sourceHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	return u.Host
}
