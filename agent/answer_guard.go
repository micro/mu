package agent

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

func unavailableToolMessage(tool string) string {
	name := strings.TrimSpace(tool)
	if name == "" {
		name = "source"
	}
	return name + " is unavailable right now. Use any other available live results to answer, and mention this unavailable source briefly without exposing internal payloads."
}

// completeToolAnswer prevents the agent from returning only progress narration
// after tools have already produced usable live context. LLMs occasionally stop
// at phrases like "Let me pull that data" even though the tool calls are done;
// in that case, synthesize a compact answer directly from the collected results
// so the user still gets useful output and unavailable slices are explicit.
func completeToolAnswer(answer string, ragParts []string) string {
	trimmed := strings.TrimSpace(answer)
	if len(ragParts) == 0 {
		return answer
	}
	if caveat := staleNewsFreshnessCaveat(ragParts); caveat != "" {
		guarded := labelStaleNewsAnswerStories(trimmed)
		if answerLeadsWithFreshnessCaveat(trimmed) {
			if strings.HasPrefix(strings.ToLower(caveat), "mostly stale") && answerContainsNewsStoryLine(guarded) {
				if fallback := synthesizeToolFallback(ragParts); strings.TrimSpace(fallback) != "" {
					return fallback
				}
			}
			return guarded
		}
		if fallback := synthesizeToolFallback(ragParts); strings.TrimSpace(fallback) != "" {
			return fallback
		}
		if guarded == "" {
			return caveat
		}
		return caveat + "\n\n" + guarded
	}

	if !isProgressOnlyAnswer(trimmed) && !isRawToolPayloadAnswer(trimmed) && !hasOperationalFallbackLead(trimmed) {
		return answer
	}

	return synthesizeToolFallback(ragParts)
}

func staleNewsFreshnessCaveat(ragParts []string) string {
	for _, part := range ragParts {
		title, body := splitRAGPart(part)
		if canonicalToolTitle(title) != "news" {
			continue
		}
		for _, raw := range strings.Split(body, "\n") {
			line := strings.TrimSpace(strings.TrimLeft(raw, "-• "))
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "freshness caveat:") {
				notice := strings.TrimSpace(line[len("Freshness caveat:"):])
				if strings.Contains(lower, "only ") && strings.Contains(lower, " dated news_search results") {
					return "Mostly stale news_search results: " + notice
				}
				return "No current news_search results: " + notice
			}
		}
	}
	return ""
}

func labelStaleNewsAnswerStories(answer string) string {
	if strings.TrimSpace(answer) == "" {
		return ""
	}
	lines := strings.Split(answer, "\n")
	changed := false
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		marker := ""
		content := trimmed
		for _, prefix := range []string{"- ", "* ", "• "} {
			if strings.HasPrefix(content, prefix) {
				marker = prefix
				content = strings.TrimSpace(strings.TrimPrefix(content, prefix))
				break
			}
		}
		lower := strings.ToLower(content)
		if strings.HasPrefix(lower, "background:") || strings.HasPrefix(lower, "no current") || strings.Contains(lower, "no same-day") || strings.Contains(lower, "freshness caveat") {
			continue
		}
		if looksLikeNewsStoryLine(content) || regexp.MustCompile(`^\d+[.)]\s+`).MatchString(content) {
			indent := raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]
			lines[i] = indent + marker + "Background: " + content
			changed = true
		}
	}
	if !changed {
		return answer
	}
	return strings.Join(lines, "\n")
}

func answerContainsNewsStoryLine(answer string) bool {
	for _, raw := range strings.Split(answer, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(raw, "-•* "))
		if line == "" {
			continue
		}
		line = strings.TrimSpace(regexp.MustCompile(`^\d+[.)]\s+`).ReplaceAllString(line, ""))
		line = strings.TrimSpace(strings.TrimPrefix(line, "Background:"))
		if looksLikeNewsStoryLine(line) {
			return true
		}
	}
	return false
}

func answerLeadsWithFreshnessCaveat(answer string) bool {
	for _, raw := range strings.Split(answer, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(raw, "-•#* "))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		return strings.Contains(lower, "no current") || strings.Contains(lower, "no same-day") || strings.Contains(lower, "freshness caveat") || strings.Contains(lower, "stale")
	}
	return false
}

func synthesizeToolFallback(ragParts []string) string {
	var sections []string
	var unavailable []string
	available := map[string]bool{}
	for _, part := range ragParts {
		title, body := splitRAGPart(part)
		if body == "" || isUnavailableToolResult(body) {
			continue
		}
		if section := formatFallbackSection(title, body); section != "" {
			sections = append(sections, section)
			available[canonicalToolTitle(title)] = true
		}
	}
	for _, part := range ragParts {
		title, body := splitRAGPart(part)
		if body == "" || !isUnavailableToolResult(body) {
			continue
		}
		if available[canonicalToolTitle(title)] {
			continue
		}
		unavailable = append(unavailable, title)
	}

	var b strings.Builder
	if len(sections) > 0 {
		b.WriteString(strings.Join(sections, "\n\n"))
	} else {
		b.WriteString("I checked the live sources, but the requested data is unavailable right now.")
	}
	if len(unavailable) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Unavailable right now: ")
		b.WriteString(strings.Join(readableToolNames(unavailable), ", "))
		b.WriteString(".")
	}
	return b.String()
}

func hasOperationalFallbackLead(answer string) bool {
	for _, raw := range strings.Split(answer, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(raw, "-• "))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if isFallbackMetadataLine(line) || isSearchResultHeading(line) || isFallbackSectionLabel(line) || isFallbackSecondaryContextLine(line) {
			return true
		}
		operationalPrefixes := []string{
			"observation:",
			"observed at ",
			"observation time:",
			"current conditions observed",
			"provider timestamp:",
			"provider:",
			"as of provider",
			"as of the provider",
			"according to provider",
			"current request date:",
			"current date context:",
		}
		for _, prefix := range operationalPrefixes {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
		if isGenericSearchFallbackIntro(line) {
			return true
		}
		return false
	}
	return false
}

func splitRAGPart(part string) (string, string) {
	part = strings.TrimSpace(part)
	if part == "" {
		return "results", ""
	}
	if strings.HasPrefix(part, "### ") {
		line, rest, ok := strings.Cut(part, "\n")
		title := strings.TrimSpace(strings.TrimPrefix(line, "### "))
		if title == "" {
			title = "results"
		}
		if !ok {
			return title, ""
		}
		return title, strings.TrimSpace(rest)
	}
	return "results", part
}

func formatFallbackSection(title, body string) string {
	canonical := canonicalToolTitle(title)
	if canonical == "web_search" {
		return formatWebSearchFallbackSection(body)
	}

	lines := meaningfulLines(body, 6)
	if canonical == "news" {
		lines = prioritizeStaleNewsCaveatLines(lines)
	}
	if canonical == "weather" {
		lines = prioritizeCurrentWeatherLines(lines)
		if summary := weatherAnswerLead(lines); summary != "" {
			lines = prependDistinctLine(summary, lines)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	if shouldShowFallbackHeading(title) {
		b.WriteString("**")
		b.WriteString(readableToolName(title))
		b.WriteString("**\n")
	}
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatWebSearchFallbackSection(body string) string {
	lines := meaningfulLines(body, 6)
	lines = prioritizeSnippetBackedWebLines(lines)
	genericOnly := hasOnlyGenericWebEvidence(lines)
	adjacentOnly := hasOnlyAdjacentAIChipFinanceEvidence(lines)
	lines = filterAdjacentAIChipFinanceLines(lines, 4)
	lines = filterGenericWebResultLines(lines, 4)
	if len(lines) == 0 {
		return ""
	}

	limited := hasLowConfidenceWebEvidence(body) || genericOnly || adjacentOnly
	requestDate := fallbackRequestDate(body)
	if !limited {
		if summary := webSearchAnswerLead(lines, requestDate); summary != "" {
			lines = prependDistinctLine(summary, lines)
		}
	}
	var b strings.Builder
	if limited {
		b.WriteString("- The available web results do not clearly prove a complete answer; here is the limited source-backed evidence I found.\n")
	}
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func weatherAnswerLead(lines []string) string {
	var now, forecast string
	for _, line := range lines {
		label, value := splitFallbackLabel(line)
		switch strings.ToLower(label) {
		case "now", "current", "current conditions", "conditions", "temperature", "observation", "feels like":
			if now == "" {
				now = value
			}
		case "forecast", "today", "today's forecast", "high", "daily forecast":
			if forecast == "" {
				forecast = value
			}
		}
	}
	if now == "" {
		return ""
	}
	if forecast != "" {
		return "Right now: " + trimTrailingSentencePunctuation(now) + "; today: " + trimTrailingSentencePunctuation(forecast) + "."
	}
	return "Right now: " + trimTrailingSentencePunctuation(now) + "."
}

func splitFallbackLabel(line string) (string, string) {
	label, value, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(label), strings.TrimSpace(value)
}

func trimTrailingSentencePunctuation(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), ".")
}

func webSearchAnswerLead(lines []string, requestDate string) string {
	if len(lines) < 2 {
		return ""
	}
	first := webSearchStoryTitle(lines[0])
	second := webSearchStoryTitle(lines[1])
	if first == "" || second == "" {
		return ""
	}
	prefix := "Latest source-backed items"
	if requestDate != "" {
		prefix += " for " + requestDate
	}
	return prefix + ": " + first + "; " + second + "."
}

func prioritizeStaleNewsCaveatLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	caveatIndex := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "freshness caveat:") {
			caveatIndex = i
			break
		}
	}
	if caveatIndex < 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	caveat := strings.TrimSpace(lines[caveatIndex])
	caveat = strings.TrimSpace(caveat[len("Freshness caveat:"):])
	if lower := strings.ToLower(caveat); strings.Contains(lower, "only ") && strings.Contains(lower, " dated news_search results") {
		out = append(out, "Mostly stale news_search results: "+caveat)
	} else {
		out = append(out, "No current news_search results: "+caveat)
	}
	var storyLines []string
	var otherLines []string
	for i, line := range lines {
		if i == caveatIndex {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isSearchResultHeading(trimmed) {
			continue
		}
		if looksLikeNewsStoryLine(trimmed) {
			if !strings.HasPrefix(strings.ToLower(trimmed), "background:") {
				trimmed = "Background: " + trimmed
			}
			storyLines = append(storyLines, trimmed)
			continue
		}
		otherLines = append(otherLines, trimmed)
	}
	sort.SliceStable(storyLines, func(i, j int) bool {
		left := fallbackNewsPostedAt(storyLines[i])
		right := fallbackNewsPostedAt(storyLines[j])
		if left.IsZero() || right.IsZero() {
			return false
		}
		return left.After(right)
	})
	storyLines = filterAdjacentAIChipFinanceLines(storyLines, len(storyLines))
	out = append(out, storyLines...)
	out = append(out, otherLines...)
	return out
}

func fallbackNewsPostedAt(line string) time.Time {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "posted:")
	if idx < 0 {
		return time.Time{}
	}
	posted := strings.TrimSpace(line[idx+len("posted:"):])
	for _, sep := range []string{";", ")", " — "} {
		if cut := strings.Index(posted, sep); cut >= 0 {
			posted = strings.TrimSpace(posted[:cut])
		}
	}
	posted = strings.TrimSpace(strings.Trim(posted, ".,"))
	for _, layout := range []string{time.RFC3339, "2 Jan 2006 15:04 MST", "2 Jan 2006", "2006-01-02", "Jan 2, 2006"} {
		if t, err := time.Parse(layout, posted); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func looksLikeNewsStoryLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" || strings.HasPrefix(lower, "no current") || strings.HasPrefix(lower, "freshness caveat:") {
		return false
	}
	return strings.Contains(lower, "posted:") || strings.Contains(lower, "source: http") || strings.Contains(line, " — ")
}

func fallbackRequestDate(body string) string {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "current request date:"):
			return cleanRequestDateLabel(strings.TrimSpace(line[len("Current request date:"):]))
		case strings.HasPrefix(lower, "current date context: request date is "):
			return cleanRequestDateLabel(strings.TrimSpace(line[len("Current date context: request date is "):]))
		}
	}
	return ""
}

func cleanRequestDateLabel(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	for _, suffix := range []string{", UTC)", " UTC)"} {
		if strings.HasSuffix(value, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(value, suffix) + ")")
		}
	}
	value = strings.TrimSuffix(value, ", UTC")
	value = strings.TrimSuffix(value, " UTC")
	return strings.TrimSpace(value)
}

func webSearchStoryTitle(line string) string {
	line = cleanFallbackLine(line)
	if title, _, ok := strings.Cut(line, " — "); ok {
		return strings.TrimSpace(title)
	}
	if title, _, ok := strings.Cut(line, " - "); ok {
		return strings.TrimSpace(title)
	}
	return ""
}

func prependDistinctLine(line string, lines []string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return lines
	}
	for _, existing := range lines {
		if strings.EqualFold(strings.TrimSpace(existing), line) {
			return lines
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, line)
	out = append(out, lines...)
	return out
}

func shouldShowFallbackHeading(title string) bool {
	// The fallback answer is shown as the primary response when the model only
	// produced progress narration or raw tool payloads. Keep it answer-first:
	// implementation/service labels belong in the References card, not as the
	// opening line of the user-facing answer.
	return false
}

func readableToolNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, readableToolName(name))
	}
	return out
}

func readableToolName(name string) string {
	switch canonicalToolTitle(name) {
	case "weather":
		return "weather"
	case "news":
		return "news"
	case "web_search":
		return "web search"
	case "markets":
		return "markets"
	case "video":
		return "video"
	case "blog":
		return "blog"
	case "social":
		return "social"
	case "places_search", "places_nearby":
		return "places"
	case "recall":
		return "memory"
	default:
		return strings.ReplaceAll(strings.TrimSpace(name), "_", " ")
	}
}

func hasLowConfidenceWebEvidence(body string) bool {
	for _, raw := range strings.Split(body, "\n") {
		lower := strings.ToLower(strings.TrimSpace(raw))
		if strings.HasPrefix(lower, "confidence:") && (strings.Contains(lower, "low") || strings.Contains(lower, "limited")) {
			return true
		}
	}
	return false
}

func meaningfulLines(body string, limit int) []string {
	var primary []string
	var context []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "-• ")
		if line == "" || isFallbackMetadataLine(line) || isUnavailableLine(line) || isSearchResultHeading(line) || isFallbackSectionLabel(line) {
			continue
		}
		line = cleanFallbackLine(line)
		if line == "" {
			continue
		}
		line = truncateFallbackLinePreservingURL(line, 280)
		if isFallbackSecondaryContextLine(line) {
			context = append(context, line)
		} else {
			primary = append(primary, line)
		}
	}

	lines := append(primary, context...)
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}

var fallbackOrdinalPrefix = regexp.MustCompile(`^\d+[.)]\s+`)

func cleanFallbackLine(line string) string {
	line = strings.TrimSpace(fallbackOrdinalPrefix.ReplaceAllString(line, ""))
	line = fallbackHTMLTagPattern.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

var fallbackHTMLTagPattern = regexp.MustCompile(`</?(?:strong|b|em|i)\b[^>]*>`)

func prioritizeCurrentWeatherLines(lines []string) []string {
	if len(lines) < 2 {
		return lines
	}
	out := make([]string, 0, len(lines))
	used := make([]bool, len(lines))
	for i, line := range lines {
		label, _ := splitFallbackLabel(line)
		switch strings.ToLower(label) {
		case "now", "current", "current conditions", "conditions", "temperature", "observation", "feels like":
			out = append(out, line)
			used[i] = true
		}
	}
	for i, line := range lines {
		if !used[i] {
			out = append(out, line)
		}
	}
	return out
}

func isFallbackSecondaryContextLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	prefixes := []string{
		"freshness/source:",
		"last refresh:",
		"generated at ",
		"source:",
		"sources:",
		"disclosure:",
		"request date:",
		"provider:",
		"provider timestamp:",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isSearchResultHeading(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "search results for ") || lower == "search results:" || strings.HasPrefix(lower, "web results for ") || strings.HasPrefix(lower, "news results for ") || strings.HasPrefix(lower, "top results")
}

func isGenericSearchFallbackIntro(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "here are the search results") ||
		strings.HasPrefix(lower, "here are some search results") ||
		strings.HasPrefix(lower, "i found these search results") ||
		strings.HasPrefix(lower, "these search results")
}

func hasSnippetBackedWebStory(line string) bool {
	if isGenericWebResultLine(line) {
		return false
	}
	cleaned := cleanFallbackLine(line)
	_, snippet, hasSnippet := strings.Cut(cleaned, " — ")
	if !hasSnippet {
		_, snippet, hasSnippet = strings.Cut(cleaned, " - ")
	}
	return hasSnippet && !isGenericWebSnippet(snippet) && snippetStoryLead(snippet) != ""
}

func snippetStoryLead(snippet string) string {
	snippet = strings.TrimSpace(strings.TrimSuffix(snippet, "."))
	if snippet == "" {
		return ""
	}
	if idx := strings.Index(snippet, ". "); idx > 0 {
		snippet = strings.TrimSpace(snippet[:idx])
	}
	if len([]rune(snippet)) > 96 {
		r := []rune(snippet)
		snippet = strings.TrimSpace(string(r[:96]))
	}
	lower := strings.ToLower(snippet)
	storyVerbs := []string{"announced", "launch", "launched", "unveiled", "released", "ships", "said", "reports", "raises", "signs", "expands", "debuts", "introduces", "plans", "faces"}
	for _, verb := range storyVerbs {
		if strings.Contains(lower, verb) {
			return snippet
		}
	}
	return ""
}

func truncateFallbackLinePreservingURL(line string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len([]rune(line)) <= limit {
		return line
	}
	url := fallbackLineURL(line)
	if url == "" {
		r := []rune(line)
		return string(r[:limit]) + "…"
	}

	suffix := " (" + url + ")"
	prefix := strings.TrimSpace(strings.TrimSuffix(line, suffix))
	budget := limit - len([]rune(suffix)) - 1
	if budget < 24 {
		return url
	}
	prefixRunes := []rune(prefix)
	if len(prefixRunes) > budget {
		prefix = strings.TrimSpace(string(prefixRunes[:budget])) + "…"
	}
	return prefix + suffix
}

func fallbackLineURL(line string) string {
	start := strings.LastIndex(line, "(")
	end := strings.LastIndex(line, ")")
	if start < 0 || end <= start {
		return ""
	}
	url := strings.TrimSpace(line[start+1 : end])
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return ""
}

func prioritizeSnippetBackedWebLines(lines []string) []string {
	if len(lines) < 2 {
		return lines
	}
	buckets := make([][]string, 4)
	for _, line := range lines {
		priority := webResultFallbackPriority(line)
		buckets[priority] = append(buckets[priority], line)
	}
	if len(buckets[0])+len(buckets[1]) >= 2 {
		out := make([]string, 0, len(lines))
		for _, bucket := range buckets {
			out = append(out, bucket...)
		}
		return out
	}
	return lines
}

func filterGenericWebResultLines(lines []string, limit int) []string {
	if len(lines) == 0 {
		return lines
	}
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if !isGenericWebResultLine(line) || hasSnippetBackedWebStory(line) {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		kept = lines
	}
	if len(kept) > limit {
		kept = kept[:limit]
	}
	return kept
}

func filterAdjacentAIChipFinanceLines(lines []string, limit int) []string {
	if len(lines) == 0 {
		return lines
	}
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if !isAdjacentAIChipFinanceLine(line) {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		kept = lines
	}
	if len(kept) > limit {
		kept = kept[:limit]
	}
	return kept
}

func hasOnlyGenericWebEvidence(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !isGenericWebResultLine(line) || hasSnippetBackedWebStory(line) {
			return false
		}
	}
	return true
}

func hasOnlyAdjacentAIChipFinanceEvidence(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !isAdjacentAIChipFinanceLine(line) {
			return false
		}
	}
	return true
}

func webResultFallbackPriority(line string) int {
	if isAdjacentAIChipFinanceLine(line) {
		return 3
	}
	if isGenericWebResultLine(line) {
		return 3
	}
	if isDatedWebStoryLine(line) {
		return 0
	}
	if hasWebResultSnippet(line) {
		return 1
	}
	return 2
}

func hasWebResultSnippet(line string) bool {
	cleaned := cleanFallbackLine(line)
	if _, _, ok := strings.Cut(cleaned, " — "); ok {
		return true
	}
	_, _, ok := strings.Cut(cleaned, " - ")
	return ok
}

func isDatedWebStoryLine(line string) bool {
	lower := strings.ToLower(cleanFallbackLine(line))
	dateMarkers := []string{
		"today",
		"yesterday",
		"this morning",
		"this afternoon",
		"this evening",
		"this week",
	}
	for _, marker := range dateMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return webStoryNumericDatePattern.MatchString(lower) || webStoryMonthDatePattern.MatchString(lower)
}

func isAdjacentAIChipFinanceLine(line string) bool {
	lower := strings.ToLower(cleanFallbackLine(line))
	if !containsAny(lower, []string{"ai chip", "ai-chip", "artificial intelligence chip", "accelerator", "gpu", "nvidia", "semiconductor", "sk hynix", "hynix"}) {
		return false
	}
	if !containsAny(lower, []string{"ipo", "nasdaq", "stock", "stocks", "share", "shares", "valuation", "market", "investor", "revenue", "sales", "profit", "earnings"}) {
		return false
	}
	return !hasConcreteAIAction(lower)
}

func hasConcreteAIAction(lower string) bool {
	concreteTerms := []string{
		"ai model", "model update", "assistant", "agent", "training", "inference", "data center", "datacenter",
		"accelerator customers", "gpu cluster", "capacity", "supply deal", "pilot", "deploy", "deployed",
		"launch", "launched", "release", "released", "unveiled", "rollout", "rolls out", "tool",
	}
	return containsAny(lower, concreteTerms)
}

func containsAny(s string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

var (
	webStoryNumericDatePattern = regexp.MustCompile(`\b(?:20\d{2}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}[-/]\d{1,2}[-/]20\d{2})\b`)
	webStoryMonthDatePattern   = regexp.MustCompile(`\b(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\.?\s+\d{1,2}(?:,\s*20\d{2})?\b`)
)

func isGenericWebSnippet(snippet string) bool {
	snippet = strings.ToLower(strings.TrimSpace(snippet))
	genericSnippetTerms := []string{
		"articles about",
		"archive of",
		"category page",
		"latest news and headlines",
		"breaking news, analysis",
		"coverage of",
		"company news and announcements",
		"industry coverage and topic pages",
		"news and campus articles tagged",
	}
	for _, term := range genericSnippetTerms {
		if strings.Contains(snippet, term) {
			return true
		}
	}
	return false
}

func isGenericWebResultLine(line string) bool {
	cleaned := strings.ToLower(cleanFallbackLine(line))
	title, snippet, hasSnippet := strings.Cut(cleaned, " — ")
	if !hasSnippet {
		title, snippet, hasSnippet = strings.Cut(cleaned, " - ")
	}
	genericURLTerms := []string{
		"artificialintelligence-news.com",
		"bloomberg.com/ai",
		"bloomberg.com/technology/ai",
		"theguardian.com/technology/artificialintelligenceai",
		"reuters.com/technology/artificial-intelligence",
		"tech.yahoo.com/ai",
		"news.mit.edu/topic/artificial-intelligence",
		"aimagazine.com",
		"yahoo.com/news/tag/artificial-intelligence",
		"yahoo.com/tech/ai",
		"yahoo.com/tech/tag/artificial-intelligence",
		"techcrunch.com/category/artificial-intelligence",
	}
	for _, term := range genericURLTerms {
		if strings.Contains(cleaned, term) {
			return true
		}
	}
	genericTitleTerms := []string{
		"category",
		"archive",
		"tag",
		"search results",
		"latest news and headlines",
		"news &",
		"artificial intelligence |",
		"artificial intelligence news",
	}
	for _, term := range genericTitleTerms {
		if strings.Contains(title, term) {
			return true
		}
	}
	if !hasSnippet {
		return false
	}
	return isGenericWebSnippet(snippet)
}

func isFallbackSectionLabel(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	labels := []string{
		"live crypto prices:",
		"latest news:",
		"news results:",
	}
	for _, label := range labels {
		if lower == label {
			return true
		}
	}
	return false
}

func isFallbackMetadataLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	prefixes := []string{
		"query intent:",
		"confidence:",
		"sources:",
		"current date context:",
		"current request date:",
		"grounding rule:",
		"provider timestamp:",
	}
	if strings.HasPrefix(lower, "observation:") && !strings.Contains(lower, "°") {
		return true
	}
	if strings.HasPrefix(lower, "observed at ") || strings.HasPrefix(lower, "observation time:") || strings.HasPrefix(lower, "current conditions observed") {
		return true
	}
	if strings.HasPrefix(lower, "provider:") || strings.HasPrefix(lower, "provider timestamp:") {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isUnavailableToolResult(body string) bool {
	hasUnavailable := false
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(raw, "-• "))
		if line == "" || isFallbackMetadataLine(line) {
			continue
		}
		if isUnavailableLine(line) {
			hasUnavailable = true
			continue
		}
		return false
	}
	return hasUnavailable
}

func isUnavailableLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	unavailablePhrases := []string{
		"unavailable",
		"no news available",
		"no news headlines available",
		"no news headlines found",
		"no videos found",
		"no places found",
		"no topup methods available",
		"data unavailable",
	}
	for _, phrase := range unavailablePhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// completeNativeToolAnswer provides the same safety net for the native
// go-micro agent path. That path streams tool lifecycle events but not raw tool
// payloads, so when it stops at progress narration after using tools we can at
// least return a clear unavailable state instead of leaving the user with
// “let me check” as the final answer.
func completeNativeToolAnswer(answer string, toolLabels []string) string {
	trimmed := strings.TrimSpace(answer)
	if len(toolLabels) == 0 || !isProgressOnlyAnswer(trimmed) {
		return answer
	}

	return "I checked the live sources, but couldn't produce a complete final answer from the available tool results. Please try again in a moment; if one source is unavailable, ask for the specific slice (news, markets, weather, or search) and I'll show what is reachable."
}

func isProgressOnlyAnswer(answer string) bool {
	if answer == "" {
		return true
	}

	lower := strings.ToLower(answer)
	progressPhrases := []string{
		"let me pull",
		"let me fetch",
		"let me check",
		"let me search",
		"i'll pull",
		"i’ll pull",
		"i will pull",
		"i'll fetch",
		"i’ll fetch",
		"i will fetch",
		"i'll check",
		"i’ll check",
		"i will check",
		"i'll search",
		"i’ll search",
		"i will search",
		"i'll look up",
		"i’ll look up",
		"i will look up",
		"search the web",
		"search for more",
		"pull that data",
		"fetch that data",
		"gather that information",
		"look that up",
	}
	for _, phrase := range progressPhrases {
		if strings.Contains(lower, phrase) && len([]rune(answer)) < 240 {
			return true
		}
	}
	return false
}

func isRawToolPayloadAnswer(answer string) bool {
	if answer == "" {
		return false
	}

	trimmed := strings.TrimSpace(answer)
	if len([]rune(trimmed)) > 4000 {
		return false
	}

	lower := strings.ToLower(trimmed)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		jsonMarkers := []string{
			`"results"`,
			`"feed"`,
			`"items"`,
			`"title"`,
			`"url"`,
			`"error"`,
			`"text"`,
			`"summary"`,
			`"source"`,
			`"status"`,
		}
		for _, marker := range jsonMarkers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}

	payloadMarkers := []string{
		`{"results":`,
		`{"feed":`,
		`[{"title":`,
		`"error":`,
		`"url":`,
		`"source":`,
		`"status":`,
	}
	matches := 0
	for _, marker := range payloadMarkers {
		if strings.Contains(lower, marker) {
			matches++
		}
	}
	return matches >= 2
}

func canonicalToolTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	lower := strings.ToLower(trimmed)
	switch lower {
	case "weather_forecast":
		return "weather"
	case "news", "news_search", "news_headlines", "news_read":
		return "news"
	case "web_search":
		return "web_search"
	}

	// Native go-micro tool names can arrive with service/method wrappers rather
	// than the public MCP tool name (for example dotted service labels around
	// news.Search). Treat those as news too so freshness caveats from the model-
	// ready news_search payload are enforced consistently in streaming/final
	// answer guards.
	compact := strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_").Replace(lower)
	if strings.Contains(compact, "news_search") || strings.Contains(compact, "news_headlines") || strings.Contains(compact, "news_read") || strings.HasSuffix(compact, "_news") || strings.Contains(compact, "_news_") {
		return "news"
	}

	return trimmed
}
