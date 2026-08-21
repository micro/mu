package test

// A service page does not decide how wide the product is.
//
// Twelve of them had picked their own content width — 560, 600, 640, 680, 720,
// 760, 768 and 794 — and two centred it as well, so walking from Transit to
// Text to Recall moved the text sideways and changed its measure each time.
// None of that was decided; each page picked a number the day it was written,
// which is the same sentence app.Column's own comment already contains about
// the text pages it was built to fix. It was built and then not adopted.
//
// The rule is narrow on purpose. A page may set a width on something inside it
// — a radar that has to stay square, a QR code, an editor shaped like a sheet
// of paper — and those are all elements. What it may not do is invent the
// column the page itself lives in, because that is the one measurement that
// has to agree across the product.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// centred finds a CSS rule that sets a width and centres itself, which is what
// a page container looks like and what a component does not.
var centred = regexp.MustCompile(`([^\n{};]+)\{[^}]*max-width:\s*(\d+)px[^}]*margin:\s*0 auto`)

// alsoCentred is the same pair written the other way round.
var alsoCentred = regexp.MustCompile(`([^\n{};]+)\{[^}]*margin:\s*0 auto[^}]*max-width:\s*(\d+)px`)

// element is a selector whose last word names an element rather than a region:
// `.fl-scope svg` centres a graphic, `.xwrap` centred the page.
var element = regexp.MustCompile(`(?:^|[\s>+~])(svg|img|canvas|video|table|iframe|pre)\s*$`)

func TestNoServicePageInventsItsOwnColumn(t *testing.T) {
	root := at("service")
	var found []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// The app sandbox writes whole documents for somebody else's page. What
		// a person builds there is theirs, and none of it is our chrome.
		if strings.Contains(path, filepath.Join("service", "apps")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, re := range []*regexp.Regexp{centred, alsoCentred} {
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				selector := strings.TrimSpace(m[1])
				if element.MatchString(selector) {
					continue
				}
				found = append(found, rel+": "+selector+" — "+m[2]+"px, centred")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(found) > 0 {
		t.Errorf("%d service page(s) centre a column of their own instead of using "+
			"app.Column():\n\t%s\n\nThe width of the product is one decision. See "+
			"app.Column, which is that decision, and internal/app/ui.go for why.",
			len(found), strings.Join(found, "\n\t"))
	}
}
