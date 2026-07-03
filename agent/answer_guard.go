package agent

import (
	"regexp"
	"strings"
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
	if len(ragParts) == 0 || (!isProgressOnlyAnswer(trimmed) && !isRawToolPayloadAnswer(trimmed)) {
		return answer
	}

	return synthesizeToolFallback(ragParts)
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
	if canonical == "weather" {
		lines = prioritizeCurrentWeatherLines(lines)
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
	lines := meaningfulLines(body, 4)
	if len(lines) == 0 {
		return ""
	}

	limited := hasLowConfidenceWebEvidence(body)
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
		if len([]rune(line)) > 280 {
			r := []rune(line)
			line = string(r[:280]) + "…"
		}
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
	return strings.TrimSpace(fallbackOrdinalPrefix.ReplaceAllString(line, ""))
}

func prioritizeCurrentWeatherLines(lines []string) []string {
	if len(lines) < 2 {
		return lines
	}
	out := make([]string, 0, len(lines))
	used := make([]bool, len(lines))
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "now:") {
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
	return strings.HasPrefix(lower, "search results for ") || lower == "search results:" || strings.HasPrefix(lower, "web results for ")
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
	switch strings.TrimSpace(title) {
	case "weather_forecast":
		return "weather"
	case "news_search", "news_headlines", "news_read":
		return "news"
	case "web_search":
		return "web_search"
	default:
		return strings.TrimSpace(title)
	}
}
