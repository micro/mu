package test

// A state that is drawn with weight has to say what the other state weighs.
//
// mu.css carries `a { font-weight: bold }`, so every link in the product is
// bold and anything inside a link inherits it. A rule like
//
//     .ib-row.unseen .ib-subject { font-weight: 700 }
//
// then says nothing at all: the subject was already 700 on every row, read or
// not, and the one distinction a mailbox exists to draw was invisible. It was
// reported as "I've read the mail and the subject is still in bold".
//
// The neighbouring .ib-who escaped it by declaring 500 of its own, and
// .peek-title on Home escaped the same way — which is what makes this worth a
// test rather than a fix. Nothing about the failing rule looks wrong; it is
// correct and inert, and the missing half is somewhere else in the file.
//
// So: for every rule that turns something bold in a state, the same element
// must declare a base weight somewhere.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// weightDecl is a font-weight declaration, however it is written — a keyword, a
// number, or a var() with either as its fallback.
var weightDecl = regexp.MustCompile(`(?i)font-weight:\s*[^;}]+`)

// stateful is a selector that draws a state: it qualifies something with a
// second class or a pseudo-class rather than naming it plainly.
var stateful = regexp.MustCompile(`\.(unseen|unread|active|selected|current|on)\b`)

func TestAStatefulWeightHasABaseWeight(t *testing.T) {
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
		sheets = append(sheets, styleBlocksIn(src)...)
	})

	// Which classes declare a weight of their own, anywhere, under a selector
	// that is not itself about a state.
	declares := map[string]bool{}
	// class -> the stateful selector that bolds it.
	needs := map[string]string{}

	for _, block := range strings.Split(stripCSSComments(strings.Join(sheets, "\n")), "}") {
		i := strings.Index(block, "{")
		if i < 0 {
			continue
		}
		selector, body := strings.TrimSpace(block[:i]), block[i+1:]
		if !weightDecl.MatchString(body) {
			continue
		}
		for _, sel := range strings.Split(selector, ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			classes := classesIn(sel)
			if len(classes) == 0 {
				continue
			}
			// The element the rule is about is the last class in the selector.
			last := classes[len(classes)-1]
			if stateful.MatchString(sel) || strings.Contains(sel, ":hover") ||
				strings.Contains(sel, ":focus") {
				// A state rule about the element it qualifies (.pill.on) is its
				// own base; only a descendant of a state (.row.unseen .subject)
				// needs one elsewhere.
				if stateful.MatchString("." + last) {
					continue
				}
				if _, seen := needs[last]; !seen {
					needs[last] = sel
				}
				continue
			}
			declares[last] = true
		}
	}

	if len(declares) < 20 {
		t.Fatalf("only %d classes declare a weight — this scan is broken, not the CSS", len(declares))
	}

	for class, sel := range needs {
		if declares[class] {
			continue
		}
		t.Errorf("%q makes .%s bold in a state, and .%s never declares a weight of "+
			"its own.\nmu.css sets `a { font-weight: bold }`, so if .%s sits inside a "+
			"link it is already bold and this rule draws no distinction at all. Give "+
			".%s an explicit base weight.", sel, class, class, class, class)
	}
}
