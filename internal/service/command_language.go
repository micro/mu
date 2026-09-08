package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MatchCommandsFor composes up to four independent reads. It uses the service
// declarations for every clause; it never guesses a tool or discards a clause.
// A complete match wins before splitting, so search terms retain their meaning.
func MatchCommandsFor(input string, allowed []string, private bool) ([]CommandCall, bool) {
	if len(input) > 8000 || !utf8.ValidString(input) {
		return nil, false
	}
	for _, r := range input {
		if unicode.IsControl(r) && r != '\t' {
			return nil, false
		}
	}
	input = strings.TrimSpace(input)
	if call, ok := MatchCommandFor(input, allowed, private); ok {
		return []CommandCall{call}, true
	}
	// Only a leading request wrapper is removable. Negation, dates, quoted
	// queries and trailing qualifications are never treated as stop words.
	for _, prefix := range []string{"please ", "can you ", "could you "} {
		if len(input) >= len(prefix) && strings.EqualFold(input[:len(prefix)], prefix) {
			input = strings.TrimSpace(input[len(prefix):])
			break
		}
	}
	if call, ok := MatchCommandFor(input, allowed, private); ok {
		return []CommandCall{call}, true
	}
	// Quoting protects literal conjunctions; unfamiliar composition belongs to
	// the assistant, rather than partly executing a sentence we did not parse.
	if strings.ContainsAny(input, "\"“”‘’;") {
		return nil, false
	}
	words := strings.Fields(input)
	var parts []string
	start := 0
	for i, word := range words {
		if strings.EqualFold(word, "and") || word == "&" {
			if i == start {
				return nil, false
			}
			parts = append(parts, strings.Join(words[start:i], " "))
			start = i + 1
		}
	}
	if start == 0 || start == len(words) || len(parts) >= 4 {
		return nil, false
	}
	parts = append(parts, strings.Join(words[start:], " "))
	calls := make([]CommandCall, 0, len(parts))
	for _, part := range parts {
		call, ok := MatchCommandFor(part, allowed, private)
		if !ok {
			return nil, false
		}
		calls = append(calls, call)
	}
	return calls, true
}
