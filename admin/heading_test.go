package admin

// A page says its name once.
//
// Five of these pages were titled "Admin" and then opened with an <h2> naming
// what they actually were — so the screen read "Admin" in the h1 the shell
// draws and "Users" directly under it, which is the same heading twice with
// one of them wrong. The inbox had the identical bug with a message subject
// and it was reported the same way: "title is the subject, then below it, its
// there again".
//
// It is easy to make: app.Response.Title is set at the bottom of a handler and
// the markup is written at the top, far enough apart that neither reads like a
// duplicate of the other.
//
// So this is a source scan rather than a rendered check — the failure is
// something somebody writes, and it should be caught where it is written.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	respondTitle = regexp.MustCompile(`app\.Response\{Title:\s*"([^"]*)"`)
	topHeading   = regexp.MustCompile(`<h2>`)
)

func TestNoAdminPageIsTitledAdmin(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	titles := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Without the comments, which in this package explain the very thing
		// being scanned for — the two files that removed an <h2> say so by
		// quoting it, and were the first two failures.
		body := code(string(b))

		for _, m := range respondTitle.FindAllStringSubmatch(body, -1) {
			titles++
			// The dashboard itself is allowed to be called Admin. It is the
			// only page whose subject is the whole of it.
			if m[1] == "Admin" && !strings.HasPrefix(f, "admin.go") {
				t.Errorf(`%s serves a page titled "Admin" — the shell draws that `+
					`as the heading, so the page has no name on screen and `+
					`whatever it is about gets written again underneath`, f)
			}
			if m[1] == "" {
				t.Errorf("%s serves a page with no title", f)
			}
		}

		// And nothing opens with a heading, which is what the title is for.
		if topHeading.MatchString(body) {
			t.Errorf("%s draws an <h2> — the page shell already draws the title "+
				"as an h1, and a heading under it repeats it", f)
		}
	}

	if titles < 8 {
		t.Fatalf("found %d titled pages — this scan is broken, not the code", titles)
	}
}

// code is the source with its line comments taken out.
func code(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
