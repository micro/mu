package app

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	line = protectCurrencyDollars(line)
	line = repairMalformedLeadingBold(line)
	if heading, rest, ok := strings.Cut(line, " "); ok && isMarkdownHeading(heading) {
		return heading + " " + strings.TrimSpace(rest)
	}
	if isMarkdownHeading(line) {
		return line
	}
	if strings.HasPrefix(line, "#") {
		trimmed := strings.TrimLeft(line, "#")
		level := len(line) - len(trimmed)
		if level > 0 && level <= 6 {
			return strings.Repeat("#", level) + " " + strings.TrimSpace(trimmed)
		}
	}

	if rest, ok := strings.CutPrefix(line, ">"); ok {
		return "> " + strings.TrimSpace(rest)
	}
	for _, prefix := range []string{"-", "*", "+"} {
		if rest, ok := strings.CutPrefix(line, prefix); ok && startsMarkdownListRest(rest) {
			return prefix + " " + strings.TrimSpace(rest)
		}
	}
	if rest, ok := strings.CutPrefix(line, "•"); ok && startsMarkdownListRest(rest) {
		return "- " + strings.TrimSpace(rest)
	}
	if numbered := normalizeNumberedListLine(line); numbered != "" {
		return numbered
	}
	if strings.Contains(line, "|") {
		return normalizeTableLine(line)
	}
	return line
}

var (
	malformedLeadingBoldRe = regexp.MustCompile(`^\*([^*\n]+?:)\*\*(\s|$)`)
	currencyDollarRe       = regexp.MustCompile(`\$(\d)`)
)

func protectCurrencyDollars(line string) string {
	return currencyDollarRe.ReplaceAllString(line, "$\u2060$1")
}

func repairMalformedLeadingBold(line string) string {
	return malformedLeadingBoldRe.ReplaceAllString(line, `**$1**$2`)
}

func startsMarkdownListRest(rest string) bool {
	if rest == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return unicode.IsSpace(r)
}

func isMarkdownHeading(s string) bool {
	if s == "" || len(s) > 6 {
		return false
	}
	for _, r := range s {
		if r != '#' {
			return false
		}
	}
	return true
}

func normalizeNumberedListLine(line string) string {
	dot := strings.IndexByte(line, '.')
	if dot <= 0 {
		return ""
	}
	for _, r := range line[:dot] {
		if !unicode.IsDigit(r) {
			return ""
		}
	}
	return line[:dot+1] + " " + strings.TrimSpace(line[dot+1:])
}

func normalizeTableLine(line string) string {
	leadingPipe := strings.HasPrefix(line, "|")
	trailingPipe := strings.HasSuffix(line, "|")
	body := strings.Trim(line, "|")
	cells := strings.Split(body, "|")
	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}
	out := strings.Join(cells, " | ")
	if leadingPipe {
		out = "| " + out
	}
	if trailingPipe {
		out += " |"
	}
	return out
}
