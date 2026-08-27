package test

// A link that says it has no underline still gets one.
//
// mu.css declares a global `a:hover { text-decoration: underline }`. That
// selector's specificity is (0,1,1) — one element, one pseudo-class — and a
// plain class like .agent-name is (0,1,0). So a component that sets
// text-decoration:none does *not* keep it on hover: the global rule outranks
// it, and the only fix is a :hover rule of its own at (0,2,0).
//
// This was found by being told the underline was still there after it had been
// "removed", which is the second time today a rule lost to one somewhere else
// — the first was .btn-secondary losing to .btn on source order.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestALinkThatDeclaresNoUnderlineKeepsItOnHover(t *testing.T) {
	css, anchors := gatherStyles(t), anchorClasses(t)
	classes := map[string]bool{}
	for _, set := range anchors {
		for c := range set {
			classes[c] = true
		}
	}

	noUnderline := regexp.MustCompile(`\.([a-z0-9-]+)\s*\{[^{}]*text-decoration\s*:\s*none`)

	// A :hover rule that says something about the underline — not merely a
	// :hover rule.
	//
	// This asked only whether ".class:hover" appeared anywhere, and .tool-tile
	// had one: it set a grey border. So the tiles on /services and /tools were
	// counted as covered while underlining their whole name on hover, which is
	// exactly the bug this test exists to catch, sitting inside the test's own
	// blind spot.
	cancels := map[string]bool{}
	hoverRule := regexp.MustCompile(`\.([a-z0-9-]+):hover[^{}]*\{([^{}]*)\}`)
	for _, m := range hoverRule.FindAllStringSubmatch(css, -1) {
		if strings.Contains(m[2], "text-decoration") {
			cancels[m[1]] = true
		}
	}

	// Covered by a companion, too. A card that is entirely a link carries
	// .card-hover, and mu.css cancels the underline there once for all of them
	// — asking each card's own class to repeat the rule is how they drifted
	// apart in the first place.
	covered := func(class string) bool {
		if cancels[class] {
			return true
		}
		for _, set := range anchors {
			if !set[class] {
				continue
			}
			ok := false
			for c := range set {
				if cancels[c] {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}

	// A ratchet, not a pass/fail. Some of these may even be wanted — a link that
	// should underline on hover looks exactly the same in the stylesheet as one
	// that forgot. What is not wanted is one more, added by somebody who wrote
	// text-decoration:none and believed it, which is what happened on
	// .agent-name and again on .tool-tile.
	//
	// This was 12 while the check only asked whether a :hover rule existed. It
	// is 24 now that it asks whether the :hover rule says anything about the
	// underline, and the twelve that appeared were always underlining — they
	// were hidden behind a :hover that set a border or a colour. The number went
	// up because the test got honest, not because the stylesheet got worse.
	const known = 24

	var latent []string
	seen := map[string]bool{}
	for _, m := range noUnderline.FindAllStringSubmatch(css, -1) {
		class := m[1]
		if seen[class] || !classes[class] {
			continue // counted already, or not used on an <a>
		}
		seen[class] = true
		if !covered(class) {
			latent = append(latent, "."+class)
		}
	}
	sort.Strings(latent)

	if len(latent) > known {
		t.Errorf("%d classes set text-decoration:none on an <a> with no :hover rule, "+
			"up from %d:\n  %s\n\n"+
			"The global a:hover underline is (0,1,1); a plain class is (0,1,0), so it "+
			"loses and the link underlines anyway. A .name:hover rule at (0,2,0) is "+
			"the only thing that stops it.",
			len(latent), known, strings.Join(latent, "\n  "))
	}
}

// gatherStyles is every stylesheet plus the CSS written inline in Go.
func gatherStyles(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if n := info.Name(); n == ".git" || n == "node_modules" || n == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasSuffix(path, ".css"),
			strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
			if body, err := os.ReadFile(path); err == nil {
				b.Write(body)
				b.WriteString("\n")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking: %v", err)
	}
	return b.String()
}

// anchorClasses is every class list this repository puts on an <a>, kept as
// sets rather than flattened — which class travels with which is the thing the
// caller needs, now that one class in the list can cancel the underline for the
// whole anchor.
func anchorClasses(t *testing.T) []map[string]bool {
	t.Helper()
	var out []map[string]bool
	re := regexp.MustCompile(`<a [^>]*class="([^"]+)"`)
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range re.FindAllStringSubmatch(string(body), -1) {
			set := map[string]bool{}
			for _, c := range strings.Fields(m[1]) {
				set[c] = true
			}
			out = append(out, set)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking: %v", err)
	}
	return out
}
