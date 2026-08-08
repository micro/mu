package main

// Every function that builds markup must close what it opens.
//
// The agents page opened a 720px column and never closed it. The browser nested
// everything after it — including the footer — inside that column, so the footer
// rendered halfway up the page instead of at the bottom. #content is a flex
// child sized to push the footer down, and a single missing tag defeats that
// silently: it looks like a CSS problem rather than five missing characters.
//
// The invariant is per function, not per page: these pages are string
// concatenation across a dozen helpers, and if every helper returns a balanced
// fragment then every page composed from them is balanced too. A function that
// deliberately opens a wrapper for its caller to close is the pattern that let
// this happen, so it is the pattern this refuses.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	funcStart = regexp.MustCompile(`(?m)^func\b`)
	divOpen   = regexp.MustCompile(`(?i)<div\b`)
	divClose  = regexp.MustCompile(`(?i)</div\s*>`)
	scriptish = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)>`)
	// A function that turns its own markup into a served page.
	rendersPage = regexp.MustCompile(`RenderHTMLForRequest\(|RenderHTMLWithLangAndAuth\(|app\.Respond\(`)
)

// Known-good exceptions: a wrapper opened for a caller to close, where the pair
// is obvious and local. Keep this list empty if you can.
var unbalancedByDesign = map[string]bool{}

func TestMarkupBuildersCloseTheirTags(t *testing.T) {
	var checked int
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, fn := range splitFuncs(string(src)) {
			// Only functions that render a whole page. That is where the bug
			// lives — a page handler opens a column and hands the rest of the
			// document to the chrome — and it is the only place the count is
			// meaningful: half the codebase mentions <div> inside a regexp
			// because it parses or sanitises HTML rather than emitting it.
			if !rendersPage.MatchString(fn.body) {
				continue
			}
			if !divOpen.MatchString(fn.body) && !divClose.MatchString(fn.body) {
				continue
			}
			checked++
			if unbalancedByDesign[fn.name] {
				continue
			}
			body := scriptish.ReplaceAllString(fn.body, "")
			d := len(divOpen.FindAllString(body, -1)) - len(divClose.FindAllString(body, -1))
			if d > 0 {
				t.Errorf("%s: %s leaves %d <div> unclosed — anything rendered after it "+
					"gets nested inside, and the footer stops sitting at the bottom", path, fn.name, d)
			}
			if d < 0 {
				t.Errorf("%s: %s closes %d more </div> than it opens — it ends its caller's "+
					"layout early", path, fn.name, -d)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no markup-building functions were found, so this proved nothing")
	}
	t.Logf("checked %d markup-building functions", checked)
}

type goFunc struct {
	name string
	body string
}

// splitFuncs cuts a file at top-level func declarations. Crude on purpose: it
// only needs to attribute a run of markup to the function it sits in.
func splitFuncs(src string) []goFunc {
	idx := funcStart.FindAllStringIndex(src, -1)
	out := make([]goFunc, 0, len(idx))
	for i, at := range idx {
		end := len(src)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		body := src[at[0]:end]
		name := "func"
		if nl := strings.IndexByte(body, '\n'); nl > 0 {
			name = strings.TrimSpace(body[:nl])
			if p := strings.IndexByte(name, '('); p > 5 {
				name = strings.TrimSpace(name[:p])
			}
			name = strings.TrimPrefix(name, "func ")
		}
		out = append(out, goFunc{name: name, body: body})
	}
	return out
}
