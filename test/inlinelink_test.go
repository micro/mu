package test

// A row of links separated by middots has to be a row.
//
// mu.css makes `.link` display:block. That is right for what it was written
// for — a card with one call to action at the bottom — and it is wrong every
// time somebody writes a sentence of them:
//
//	<p><a class="link" href="/a">Open it</a> · <a class="link" href="/b">Edit</a></p>
//
// renders as three stacked lines with the separators orphaned between them, a
// hundred pixels of white space where one line of text was meant to be. There
// is no error and nothing in the console; it simply looks wrong, and only if
// you look.
//
// The stylesheet already says this, in a comment directly above the rule, and
// names the fix — app.TextLink, which uses .link-text. It says it because the
// mistake had been made twice before and patched twice with a descendant
// override: `.notice .link` and `.rooms .row .link` both exist to set
// display:inline back. A third page made it anyway.
//
// A comment nobody reads at the moment they need it is not a rule. This is.
//
// What it looks for is the shape that always breaks: two or more class="link"
// anchors inside one element, side by side, with text between them. One on its
// own is a card's call to action and correct.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// blockLink matches an anchor carrying the class that is display:block.
var blockLink = regexp.MustCompile(`<a[^>]*class=["'][^"']*\blink\b[^"']*["']`)

// midDotRow finds a run of markup with two class="link" anchors and a
// separator between them — the shape that stacks.
var midDotRow = regexp.MustCompile(
	`<a[^>]*class=["'][^"']*\blink\b[^"']*["'][^>]*>[^<]*</a>\s*(?:·|&middot;|\||•)`)

func TestNoRowOfBlockLinks(t *testing.T) {
	root := repoRoot(t)

	// Where the stylesheet already sets display:inline back. A page inside one
	// of these can use .link in a row and it renders correctly.
	overridden := []string{"notice", "rooms"}

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
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		for _, lit := range markupPerFunc(string(b)) {
			hit := midDotRow.FindString(lit)
			if hit == "" {
				continue
			}
			// Inside a container the stylesheet already fixes?
			skip := false
			for _, c := range overridden {
				if strings.Contains(lit, `class="`+c) || strings.Contains(lit, `class="`+c+` `) {
					skip = true
				}
			}
			if skip {
				continue
			}
			bad = append(bad, rel+": "+strings.TrimSpace(hit))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(bad) > 0 {
		t.Errorf(".link is display:block, so a row of them separated by middots "+
			"stacks into one line each with the separators stranded. Use "+
			"app.TextLink (.link-text) for links inside a sentence. %d place(s):\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// markupPerFunc returns, for each function in a Go file, its raw string
// literals joined as the markup they build.
//
// Joined, because reading them one at a time is how the first version of this
// test passed a mutation that put the bug straight back. Markup with a value in
// it is not one literal:
//
//	`<a class="link" href="/apps/` + slug + `">Open it</a> · `
//
// is two, and neither half contains both the anchor and the separator. A scan
// over individual literals sees nothing. The value between them becomes NUL,
// which the character classes cross freely and a human never types.
//
// Split per function so that two unrelated literals at opposite ends of a file
// cannot be joined into a row that nothing renders.
func markupPerFunc(src string) []string {
	var out []string
	for _, fn := range strings.Split(src, "\nfunc ") {
		var b strings.Builder
		for _, lit := range rawStrings(fn) {
			b.WriteString(lit)
			b.WriteByte(0)
		}
		if b.Len() > 0 {
			out = append(out, b.String())
		}
	}
	return out
}

// And the rule this depends on is still the rule.
//
// If .link ever stops being display:block the test above is checking nothing,
// and would keep passing forever — the worst kind of green.
func TestLinkIsStillDisplayBlock(t *testing.T) {
	b, err := os.ReadFile(repoRoot(t) + "/internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	i := strings.Index(css, "\n.link {")
	if i < 0 {
		t.Fatal("no .link rule in mu.css — this scan is broken, not the code")
	}
	block := css[i:]
	if end := strings.Index(block, "}"); end > 0 {
		block = block[:end]
	}
	if !strings.Contains(block, "display: block") {
		t.Skip(".link is no longer display:block, so the row rule above is moot — " +
			"delete both, and the two descendant overrides in mu.css with them")
	}
}

// The link out of a card is drawn one way.
//
// A card's More link is app.Link, and the three peeks on the home page each
// hand-rolled their own anchor with a peek-more class: normal weight where
// Link is semibold, and secondary grey where Link is the primary text colour.
// Side by side on one page, in the same position on the same kind of card,
// they were visibly two different things pretending to be one.
//
// It is the ordinary way this drifts — a second way of doing something is
// easier to write than to find the first — so the test is on there being one
// way rather than on the styling of either.
func TestACardsWayOutIsTheSameLinkEverywhere(t *testing.T) {
	var found []string
	err := filepath.Walk(at(""), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".css") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(b), "peek-more") {
			rel, _ := filepath.Rel(at(""), path)
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Errorf("peek-more is back in %v — a card's way out is app.Link, which is "+
			"semibold and the primary text colour. A second class for the same "+
			"job is how two links in the same place on the same page end up "+
			"looking different.", found)
	}
}
