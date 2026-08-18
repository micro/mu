package test

// A link in the middle of a sentence must not break the line.
//
// `.link` is display:block — that is what makes app.Link a call to action
// standing on its own. Used inside a paragraph it puts a line break where it
// sits and hangs an arrow before whatever comes next, so
//
//	Answers your own verified address and people in
//	your contacts →
//	. Other mail is filed, not answered.
//
// went out looking like that, and so did three others. There is an app.TextLink
// for prose. The rule is mechanical — if anything follows the link in the same
// sentence, it is the wrong one — so it is checked rather than remembered.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// after captures the string literal immediately following an app.Link call, if
// the call is followed by one on the same line.
var after = regexp.MustCompile("app\\.Link\\([^)]*\\)\\s*\\+\\s*`([^`]*)`")

// tags is markup rather than words.
var tags = regexp.MustCompile(`<[^>]*>`)

func TestALinkInProseUsesTheProseForm(t *testing.T) {
	root := at("")
	var found []string

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range after.FindAllStringSubmatch(line, -1) {
				// What is left once the markup is taken out. A closing </p> is
				// the link ending its block, which is what app.Link is for.
				// Anything else — a word, or even the full stop that would land
				// on its own line — is a sentence carrying on around it.
				rest := strings.TrimSpace(tags.ReplaceAllString(m[1], ""))
				if rest == "" {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				found = append(found, rel+":"+itoa(i+1)+" — "+strings.TrimSpace(line))
			}
		}
		return nil
	})

	if len(found) > 0 {
		t.Errorf("app.Link is display:block, so these break the sentence they are in — "+
			"use app.TextLink:\n  %s", strings.Join(found, "\n  "))
	}
}
