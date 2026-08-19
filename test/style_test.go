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
	"os"
	"path/filepath"
	"regexp"
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
	gone := []string{"ib-tag", "els-where", "th-tool", "peek-where",
		"chat-sess-where", "run-tool"}
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

// walkGo calls f for every Go file in the repository.
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
		f(strings.TrimPrefix(path, root+"/"), string(b))
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
}
