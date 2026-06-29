package agent

import "strings"

// completeToolAnswer prevents the agent from returning only progress narration
// after tools have already produced usable live context. LLMs occasionally stop
// at phrases like "Let me pull that data" even though the tool calls are done;
// in that case, return the collected results directly so the user still gets a
// complete answer or a clear unavailable state.
func completeToolAnswer(answer string, ragParts []string) string {
	trimmed := strings.TrimSpace(answer)
	if len(ragParts) == 0 || !isProgressOnlyAnswer(trimmed) {
		return answer
	}

	var b strings.Builder
	b.WriteString("I found live results, but couldn't synthesize a full narrative. Here are the results:\n\n")
	for i, part := range ragParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(part)
	}
	return b.String()
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
