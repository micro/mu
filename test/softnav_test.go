package test

// A page script that only wires itself on DOMContentLoaded stops working.
//
// Navigation between screens is a soft navigation: #content is replaced and the
// scripts inside it are re-created so they run again. DOMContentLoaded is not
// re-fired — it happened once, on the first real page load — so an init wrapped
// in it runs on a reload and never again. The recent-search lists on /search and
// /video were both written that way, so arriving by clicking a link left them
// blank and reloading fixed them, which is exactly the shape of bug nobody
// reports as a bug.
//
// The fix is one line at the bottom of each script: run now if the document is
// already parsed. This holds it, because the next page written the same way
// would fail the same way and the symptom is invisible in a unit test.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pageInit finds a script registering an init on DOMContentLoaded.
var pageInit = regexp.MustCompile(`addEventListener\(\s*['"]DOMContentLoaded['"]`)

func TestPageScriptsRunAfterASoftNavigation(t *testing.T) {
	var checked int

	for _, dir := range []string{"service", "home", "account", "agent", "admin"} {
		err := filepath.Walk(at(dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
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
			if !pageInit.MatchString(src) {
				return nil
			}
			checked++

			// Guarded, so the init still runs when the event has already fired.
			// readyState is the check; listening for mu:navigated is the other
			// way to say it, and either is fine.
			if strings.Contains(src, "document.readyState") ||
				strings.Contains(src, "mu:navigated") {
				return nil
			}

			rel, _ := filepath.Rel(at(""), path)
			t.Errorf("%s wires its page on DOMContentLoaded and nothing else — that "+
				"event fires once per real page load, so arriving here by clicking a "+
				"link leaves the page unwired and reloading appears to fix it. Guard "+
				"it with document.readyState, or re-wire on mu:navigated", rel)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if checked == 0 {
		t.Fatal("no page script uses DOMContentLoaded at all — this scan is broken, " +
			"not the code")
	}
}

// The soft navigation reads and writes the element that actually scrolls.
//
// This layout gives body a fixed height and overflow-y:auto, so body is the
// scroller and the root element never moves — which makes window.scrollY
// permanently 0 and window.scrollTo a no-op. Every one of them was used here:
// the position saved into history was always 0, restoring it did nothing, and
// scrolling to the top on a forward navigation did nothing, so following a link
// from halfway down one page landed you halfway down the next.
func TestTheSoftNavigationScrollsTheRightElement(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(at(""), "internal/app/app.go"))
	if err != nil {
		t.Fatal(err)
	}
	nav := string(b)
	i := strings.Index(nav, "var content = document.getElementById('content');")
	if i < 0 {
		t.Fatal("the soft navigation script has moved; this test cannot find it")
	}
	j := strings.Index(nav[i:], "})();")
	if j < 0 {
		t.Fatal("cannot find the end of the soft navigation script")
	}
	nav = nav[i : i+j]

	// Without the comments. They explain why window.scrollY is wrong here, and
	// a scan that cannot tell an explanation from a call fails on the sentence
	// describing the bug it is looking for.
	var code []string
	for _, line := range strings.Split(nav, "\n") {
		if t := strings.TrimSpace(line); !strings.HasPrefix(t, "//") {
			code = append(code, line)
		}
	}
	nav = strings.Join(code, "\n")

	for _, banned := range []string{"window.scrollTo(", "window.scrollY"} {
		if strings.Contains(nav, banned) {
			t.Errorf("the soft navigation uses %s — body is the scroll container "+
				"here, so that reads 0 and writes nothing", banned)
		}
	}
	if !strings.Contains(nav, "function scroller()") {
		t.Error("the soft navigation no longer picks the scrolling element, so it is " +
			"back to guessing which one moves")
	}
}
