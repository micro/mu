package test

// Whether the site can look like itself.
//
// This is not a taste test and it does not check that anything is pretty. It
// checks one structural fact: that a page which needs an ordinary shape has a
// way to ask for it, and is not typing `border:1px solid #eee` for the fifth
// time under a fifth class name.
//
// The count is where it is because sixty-six packages ship a <style> block of
// their own, using nine greys and nine corner radii between them, while
// internal/app/html/mu.css sits on every page carrying a full palette that most
// of them never touch. /inbox used zero tokens and sixty-two literal colours,
// which is why it did not look like the rest of the site — not because it was
// styled badly, because it was styled separately.
//
// So the number below is a ratchet, not a rule. It may go down. When it has to
// go up, the question to answer first is whether the shape being drawn is one
// internal/app/primitives.go should own.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// styleBlocks is how many packages may carry a stylesheet of their own.
//
// Lower this when you delete one. Raising it means a new page has invented a
// look, which is the thing this exists to make somebody say out loud.
const styleBlocks = 62

var styleRe = regexp.MustCompile(`<style>`)

func TestPagesDoNotEachInventALook(t *testing.T) {
	dirs := map[string]int{}
	total := 0
	walkGo(t, func(path string, src string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		n := len(styleRe.FindAllString(src, -1))
		if n == 0 {
			return
		}
		dirs[filepath.Dir(path)] += n
		total += n
	})

	if total > styleBlocks {
		var worst []string
		for d, n := range dirs {
			if n > 1 {
				worst = append(worst, d)
			}
		}
		t.Errorf("%d inline stylesheets across %d packages, over the %d this is "+
			"ratcheting down from.\nA new one means a page has invented a look "+
			"rather than asking mu.css for the usual thing — see "+
			"internal/app/primitives.go.\nPackages with more than one: %v",
			total, len(dirs), styleBlocks, worst)
	}
}

// The shapes that were four names for one thing. Each of these was a hairline
// pill with a 999px radius and a grey border, drawn in its own package under
// its own class, and they had already drifted apart in size and colour.
//
// app.Pill is the answer. This test names the dead classes so that
// reintroducing one is a deliberate act rather than a copy from the file next
// door — which is exactly how there came to be four.
func TestThePillIsNotReinvented(t *testing.T) {
	// ib-reply-go is here for a different reason from the rest, and a sharper
	// one. The others were a hairline pill drawn six ways. That was a black
	// call-to-action drawn once, by hand, instead of app.ActionLink — and
	// mu.css carries `a:visited { color: #000 }`, which outranks a plain class.
	// So Reply was white-on-black until somebody used it and black-on-black
	// afterwards. a.btn already carried `color:#fff !important` against exactly
	// that. The site has one button; a second one re-earns every bug the first
	// one has already fixed.
	gone := []string{"ib-tag", "els-where", "th-tool", "peek-where",
		"chat-sess-where", "run-tool", "ib-reply-go"}
	walkGo(t, func(path, src string) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "primitives.go") {
			return
		}
		for _, class := range gone {
			if strings.Contains(src, `"`+class+`"`) || strings.Contains(src, "."+class+"{") {
				t.Errorf("%s draws %s. That shape is app.Pill — six names for one "+
					"hairline pill is how the site stopped looking like itself",
					path, class)
			}
		}
	})
}

// stripComments removes Go comments before anything is counted.
//
// Both ratchets below count strings that appear in prose as often as in
// markup — a doc comment explaining that a page used to say
// style="margin:6px 0 0" is not a page saying it. Counting comments made the
// tests fail on the commit that documented the thing they exist to discourage,
// which is the wrong way round.
func stripComments(src string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		switch {
		case strings.HasPrefix(src[i:], "//"):
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
		case strings.HasPrefix(src[i:], "/*"):
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 4
		default:
			b.WriteByte(src[i])
			i++
		}
	}
	return b.String()
}

// walkGo calls f for every Go file in the repository, comments removed.
func walkGo(t *testing.T, f func(path, src string)) {
	t.Helper()
	root := ".."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		f(strings.TrimPrefix(path, root+"/"), stripComments(string(b)))
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
}

// inlineStyles is how many style="..." attributes the product may carry.
//
// A ratchet, like styleBlocks above, and for the same reason: hundreds of these
// is not a design somebody chose, it is what happens when a page has no way to
// say "the usual button". Every one is a decision made once, in one file, that
// nothing else can inherit — and worse than verbose: a literal #888 does not
// follow --text-muted, so a page that hard-codes it will not follow the palette
// anywhere it moves.
//
// It goes down as surfaces move onto the components in internal/app/form.go and
// the type scale in mu.css. Raising it means a page invented a look again.
//
// The bulk of what is left is page-specific layout — a flex row here, a border
// there — which is not one shape repeated but forty pages each laying
// themselves out. That is per-page work rather than a sweep, which is why this
// number is coming down in steps.
const inlineStyles = 110

var styleAttr = regexp.MustCompile(`style="`)

func TestInlineStylesAreGoingDown(t *testing.T) {
	total := 0
	worst := map[string]int{}
	walkGo(t, func(path, src string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		n := len(styleAttr.FindAllString(src, -1))
		if n == 0 {
			return
		}
		total += n
		worst[filepath.Dir(path)] += n
	})
	if total > inlineStyles {
		var over []string
		for d, n := range worst {
			if n >= 20 {
				over = append(over, fmt.Sprintf("%s (%d)", d, n))
			}
		}
		sort.Strings(over)
		t.Errorf("%d inline style attributes, over the %d this is ratcheting down from.\n"+
			"A style attribute is a decision nothing else can inherit — see "+
			"internal/app/form.go for the components that replace them.\nHeaviest: %s",
			total, inlineStyles, strings.Join(over, ", "))
	}
}
