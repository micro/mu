// Package editorial validates generated publication text without changing it.
package editorial

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var protocol = regexp.MustCompile("(?i)<[/?]?(?:think|analysis|reasoning|function_calls|tool_call|invoke|parameter|xml)(?:[\\s>/]|$)|\\[/?(?:think|tool_calls)\\]|<\\|(?:im_start|im_end|analysis|assistant)")
var link = regexp.MustCompile(`\[[^\]\n]+\]\((https?://[^\s)]+)\)`)

// Artifact detects model protocol leaked into prose, including unmatched closers.
func Artifact(text string) bool { return protocol.MatchString(text) }

// Validate rejects unfinished output; it never repairs it into publishable prose.
func Validate(title, body string) error {
	if Artifact(title + "\n" + body) {
		return fmt.Errorf("model protocol in article")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(title)); n < 8 || n > 180 || strings.ContainsAny(title, "\n\r<>") {
		return fmt.Errorf("invalid article title")
	}
	if len(strings.Fields(body)) < 40 {
		return fmt.Errorf("article body is too short")
	}
	if len(body) > 30000 {
		return fmt.Errorf("article body is too long")
	}
	if !link.MatchString(body) {
		return fmt.Errorf("article has no linked sources")
	}
	return nil
}
