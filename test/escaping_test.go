package test

// Every HTML escaper in the repo escapes the same characters.
//
// There were three, written separately, and they disagreed: the one in agent
// escaped & < > and the double quote but not the single quote, while the ones
// in home and wallet escaped all five. Nothing put agent's output inside a
// single-quoted attribute, so it was a hazard rather than a hole — and the
// hazard is that nothing said so. The next single-quoted attribute would have
// been an attribute breakout, written by somebody reasonably assuming the
// function called htmlEsc escapes HTML.
//
// They all delegate to html.EscapeString now. This checks that no package
// grows its own again, because a second implementation is where they start to
// drift.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHTMLEscapersDelegateToTheStandardLibrary(t *testing.T) {
	// A hand-rolled escaper looks like a run of ReplaceAll calls on the
	// entities, or a NewReplacer over them.
	handRolled := regexp.MustCompile(`(?s)func \w*[eE]sc\w*\(s string\) string \{[^}]*?&amp;`)

	var offenders []string
	err := filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if handRolled.Match(src) {
			rel, _ := filepath.Rel(repo, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range offenders {
		t.Errorf("%s builds its own HTML escaper instead of using html.EscapeString — "+
			"three of these existed and one escaped a character fewer than the others", f)
	}
}
