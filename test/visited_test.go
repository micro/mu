package test

// A filled link has to name its visited state.
//
// mu.css carries `a:visited { color: #000 }`. It is 0-1-1, so it beats any
// single-class selector — and a class drawing white text on a dark fill is
// usually exactly that. The control goes black on black the moment somebody
// clicks it, which for a *selected* filter chip is always.
//
// It used to read `a:visited:not(.btn)`, which is 0-2-1 and beat two-class
// selectors as well. That is why this was found and patched five separate
// times on five different classes — .ib-reply-go, .mail-tag.on, .pill.on,
// .recent-search-item.active, .ar-chip.on — each time by fixing the class
// rather than the rule. The :not() is gone (a.btn carries color:#fff !important
// and never needed it), so two-class controls are safe by construction and this
// test covers the rest.
//
// It scans every stylesheet the product ships, not only mu.css. That was the
// hole: this test existed when /archive turned black-on-black, and missed it
// because .ar-chip.on lives in the page's own <style> block. A guard that only
// looks where the last bug was is a guard that catches the last bug.
//
// Only classes this repo actually puts on an <a> are checked. A badge on a
// span cannot lose to a:visited, and demanding a :visited variant of every
// white-on-dark rule in the stylesheet is noise that would get the test
// deleted.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// The class attribute of an <a> in rendered Go markup, however it is built:
	// `<a class="pill on"`, `<a href="…" class="mail-tag on"`.
	anchorClass = regexp.MustCompile(`(?is)<a\b[^>]{0,200}?class="([^"<>]{1,200})"`)
	// A declaration setting a near-white colour.
	whiteText = regexp.MustCompile(`(?i)color:\s*(#fff(f{3})?|white)\s*(!important)?\s*(;|$)`)
)

func TestAFilledLinkNamesItsVisitedState(t *testing.T) {
	// Every class this repo puts on an anchor.
	onLinks := map[string]bool{}
	walkGo(t, func(path, src string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, m := range anchorClass.FindAllStringSubmatch(stripComments(src), -1) {
			for _, c := range strings.Fields(m[1]) {
				// Skip the template holes: `class="` + cls + `"` yields junk.
				if strings.ContainsAny(c, "+`\"") {
					continue
				}
				onLinks[c] = true
			}
		}
	})
	if len(onLinks) < 20 {
		t.Fatalf("only %d anchor classes found — this scan is broken, not the CSS", len(onLinks))
	}

	// Every stylesheet the product ships: mu.css, and the <style> block each
	// page still carries.
	var sheets []string
	shared, err := os.ReadFile(filepath.Join("..", "internal", "app", "html", "mu.css"))
	if err != nil {
		t.Fatal(err)
	}
	sheets = append(sheets, string(shared))
	walkGo(t, func(path, src string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, b := range styleBlocksIn(src) {
			sheets = append(sheets, b)
		}
	})
	if len(sheets) < 20 {
		t.Fatalf("only %d stylesheets found — this scan is broken, not the CSS", len(sheets))
	}

	for _, block := range strings.Split(stripCSSComments(strings.Join(sheets, "\n")), "}") {
		i := strings.Index(block, "{")
		if i < 0 {
			continue
		}
		selector, body := strings.TrimSpace(block[:i]), block[i+1:]
		m := whiteText.FindString(body)
		if m == "" || strings.Contains(m, "!important") {
			continue
		}
		if strings.Contains(selector, ":visited") {
			continue
		}
		for _, class := range classesIn(selector) {
			if !onLinks[class] {
				continue
			}
			// The visited rule is a:visited:not(.btn), so a.btn is already
			// excluded from it — and carries color:#fff !important besides.
			// It is the answer to this bug, not an instance of it.
			if class == "btn" {
				continue
			}
			t.Errorf("%q sets white text and never names :visited, and .%s is put on "+
				"an <a> in this repo.\na:visited is 0-1-1 and outranks a single-class "+
				"selector, so this turns black-on-black once it has been clicked. Add "+
				"a :visited variant to the selector list, or use a.btn "+
				"(app.ActionLink), which is immune.",
				selector, class)
			break
		}
	}
}

// styleBlocksIn is the CSS inside every <style> a page ships.
func styleBlocksIn(src string) []string {
	var out []string
	rest := src
	for {
		i := strings.Index(rest, "<style>")
		if i < 0 {
			return out
		}
		rest = rest[i+len("<style>"):]
		j := strings.Index(rest, "</style>")
		if j < 0 {
			return append(out, rest)
		}
		out = append(out, rest[:j])
		rest = rest[j:]
	}
}

// classesIn is every class name mentioned in a selector list.
func classesIn(selector string) []string {
	var out []string
	for i := 0; i < len(selector); i++ {
		if selector[i] != '.' {
			continue
		}
		j := i + 1
		for j < len(selector) && (selector[j] == '-' || selector[j] == '_' ||
			(selector[j] >= 'a' && selector[j] <= 'z') ||
			(selector[j] >= 'A' && selector[j] <= 'Z') ||
			(selector[j] >= '0' && selector[j] <= '9')) {
			j++
		}
		if j > i+1 {
			out = append(out, selector[i+1:j])
		}
		i = j - 1
	}
	return out
}

// stripCSSComments removes /* ... */ so prose describing a rule is not read as
// one — this file's own neighbours in mu.css discuss color:#fff at length.
func stripCSSComments(css string) string {
	var b strings.Builder
	for {
		i := strings.Index(css, "/*")
		if i < 0 {
			b.WriteString(css)
			return b.String()
		}
		b.WriteString(css[:i])
		j := strings.Index(css[i:], "*/")
		if j < 0 {
			return b.String()
		}
		css = css[i+j+2:]
	}
}
