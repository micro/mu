package agent

import "strings"

// completeToolAnswer prevents the agent from returning only progress narration
// after tools have already produced usable live context. LLMs occasionally stop
// at phrases like "Let me pull that data" even though the tool calls are done;
// in that case, synthesize a compact answer directly from the collected results
// so the user still gets useful output and unavailable slices are explicit.
func completeToolAnswer(answer string, ragParts []string) string {
	trimmed := strings.TrimSpace(answer)
	if len(ragParts) == 0 || !isProgressOnlyAnswer(trimmed) {
		return answer
	}

	return synthesizeToolFallback(ragParts)
}

func synthesizeToolFallback(ragParts []string) string {
	var sections []string
	var unavailable []string
	for _, part := range ragParts {
		title, body := splitRAGPart(part)
		if body == "" {
			continue
		}
		if isUnavailableToolResult(body) {
			unavailable = append(unavailable, title)
			continue
		}
		if section := formatFallbackSection(title, body); section != "" {
			sections = append(sections, section)
		}
	}

	var b strings.Builder
	if len(sections) > 0 {
		b.WriteString("Here's what I found from the live results:\n\n")
		b.WriteString(strings.Join(sections, "\n\n"))
	} else {
		b.WriteString("I checked the live sources, but the requested data is unavailable right now.")
	}
	if len(unavailable) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Unavailable: ")
		b.WriteString(strings.Join(unavailable, ", "))
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
	lines := meaningfulLines(body, 6)
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("**")
	b.WriteString(title)
	b.WriteString("**\n")
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func meaningfulLines(body string, limit int) []string {
	var lines []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "-• ")
		if line == "" {
			continue
		}
		if len([]rune(line)) > 280 {
			r := []rune(line)
			line = string(r[:280]) + "…"
		}
		lines = append(lines, line)
		if len(lines) >= limit {
			break
		}
	}
	return lines
}

func isUnavailableToolResult(body string) bool {
	lower := strings.ToLower(strings.TrimSpace(body))
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
		"i'll pull",
		"i’ll pull",
		"i will pull",
		"i'll fetch",
		"i’ll fetch",
		"i will fetch",
		"i'll check",
		"i’ll check",
		"i will check",
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
