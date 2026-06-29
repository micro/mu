package app

import "strings"

// NormalizeAnswerMarkdown prepares assistant answers for every Mu surface before
// they are rendered as HTML or sent to chat clients. It keeps markdown intact,
// but removes accidental spacing drift that makes the same answer look different
// across web, Discord, Telegram, and WhatsApp.
func NormalizeAnswerMarkdown(answer string) string {
	answer = strings.ReplaceAll(answer, "\r\n", "\n")
	answer = strings.ReplaceAll(answer, "\r", "\n")

	lines := strings.Split(answer, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	inFence := false

	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.HasPrefix(strings.TrimSpace(trimmedRight), "```") {
			inFence = !inFence
			out = append(out, trimmedRight)
			blank = 0
			continue
		}
		if inFence {
			out = append(out, trimmedRight)
			continue
		}

		trimmed := strings.TrimSpace(trimmedRight)
		if trimmed == "" {
			blank++
			if blank <= 1 {
				out = append(out, "")
			}
			continue
		}

		blank = 0
		out = append(out, normalizeMarkdownLine(trimmed))
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}

func normalizeMarkdownLine(line string) string {
	for _, prefix := range []string{"-", "*", "+"} {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			return prefix + " " + strings.TrimSpace(rest)
		}
	}
	return line
}
