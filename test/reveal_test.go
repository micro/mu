package test

// Nothing hidden by !important can be shown by an inline style.
//
// `.d-none { display: none !important }` is in mu.css, and !important beats an
// element's own style attribute. So this, which reads like it works:
//
//	<div id="thing" class="d-none">
//	document.getElementById('thing').style.display = 'block';
//
// leaves the element hidden forever. There is no error, nothing in the console
// and nothing on the screen: the script runs, the property is set, and the
// cascade discards it. It is the purest form of built-but-invisible — the
// feature is finished, shipped, and unreachable.
//
// This was not one mistake. The same pair was written five separate times, on
// five different pages, by whoever was nearest:
//
//   - the Login with Passkey button, so an account with a passkey registered
//     had no way to use it and the whole feature was decorative;
//   - the panel on /token that shows a token you just created, so creating one
//     looked like it had failed;
//   - and three on /mcp — the tool description, the run output and the raw JSON
//     — which is every moving part of the try-it panel.
//
// Five sites, one cause, and none of them would ever have been found by reading
// the code, because the code is correct in isolation. Only the combination is
// wrong, which is exactly what a test can see and a reviewer cannot.
//
// The fix at each site is classList.remove('d-none'), which removes the rule
// rather than losing an argument with it. This test is the rule: reveal by
// class, or do not use the class.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hiddenByClass finds `id="thing" class="... d-none ..."` and names thing.
var hiddenByClass = regexp.MustCompile(`id=["']([a-zA-Z0-9_-]+)["'][^>]*class=["'][^"']*\bd-none\b`)

// revealedInline finds getElementById('thing') ... .style.display = 'something'
// on one line, which is how every one of these was written.
var revealedInline = regexp.MustCompile(`getElementById\(["']([a-zA-Z0-9_-]+)["']\)[^;\n]*\.style\.display\s*=\s*["'][^"']*["']`)

// varThenReveal covers the two-step form: a variable is bound to the element on
// one line and revealed on another. Matched by variable name across the file.
var boundToElement = regexp.MustCompile(`(?:var|let|const)\s+([a-zA-Z0-9_$]+)\s*=\s*document\.getElementById\(["']([a-zA-Z0-9_-]+)["']\)`)

func TestNothingIsRevealedByAnInlineStyleOverDNone(t *testing.T) {
	root := repoRoot(t)

	var bad []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		switch filepath.Ext(path) {
		case ".go", ".js", ".html":
		default:
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		src := string(b)

		// Which ids this file hides with the class.
		hidden := map[string]bool{}
		for _, m := range hiddenByClass.FindAllStringSubmatch(src, -1) {
			hidden[m[1]] = true
		}
		if len(hidden) == 0 {
			return nil
		}

		rel, _ := filepath.Rel(root, path)

		// Direct: getElementById('x').style.display = ...
		for _, m := range revealedInline.FindAllStringSubmatch(src, -1) {
			if hidden[m[1]] {
				bad = append(bad, rel+": #"+m[1]+" is hidden with d-none and revealed with an inline style")
			}
		}

		// Two-step: var x = getElementById('y'); ... x.style.display = ...
		for _, m := range boundToElement.FindAllStringSubmatch(src, -1) {
			name, id := m[1], m[2]
			if !hidden[id] {
				continue
			}
			byVar := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\.style\.display\s*=\s*["'][^"']*["']`)
			if byVar.MatchString(src) {
				bad = append(bad, rel+": #"+id+" is hidden with d-none and revealed via "+name+".style.display")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(bad) > 0 {
		t.Errorf(".d-none is display:none !important, so an inline style cannot undo it.\n"+
			"Use classList.remove('d-none') instead. %d place(s):\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// repoRoot walks up from the test's directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above " + dir)
		}
		dir = parent
	}
}
