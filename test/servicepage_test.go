package test

// Which services draw their own page, and which are derived from their Spec.
//
// This exists because the sweep that produced it was, briefly, half done: two
// services converted, one converted and reverted, twenty-nine untouched and no
// rule written down anywhere. A codebase in that state is not partway through a
// migration — it is one where the next person has to guess, and guessing
// produces a third convention.
//
// So the boundary is a list, and the list is a test. Moving a service across it
// is a line in a diff with a reason beside it, which is what it should have been
// the first time. The rule the list follows is in
// internal/api/service_page.go: a service keeps its page when the page shows
// something the card cannot, for one of three checkable reasons — it is your
// data, it takes an argument to say anything, or it is browsable.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// derived are the services whose page is api.ServicePage — card, agent, tools,
// doors, no hand-written HTML.
//
// Short, and expected to stay short. Adding one means its page had nothing the
// card did not; removing one means the opposite was discovered, which is what
// happened to flights.
var derived = map[string]string{
	"weather": "the page was a location box over the same forecast the card shows",
	"hazards": "a magnitude picker over the same list the card shows",
}

// keptItsPage records why, for the ones that draw their own. The reason is the
// point: an entry with no reason is a page nobody has asked the question about.
var keptItsPage = map[string]string{
	// 1. It is your data.
	"mail": "your inbox", "notes": "your notes", "files": "your files",
	"contacts": "your address book", "tasks": "your tasks", "images": "your images",
	"docs": "your documents", "events": "your calendar",
	"recall": "your own past", "sms": "your messages",
	"wallet": "your key",

	// 2. It takes an argument to say anything.
	"food": "a barcode or a product name", "places": "a search", "web": "a query",
	"markets": "one of five categories", "routes": "two endpoints",
	"transit": "a stop", "flights": "a callsign, and a scope the card cannot draw",
	"maps": "a map you move around, which is the one thing a card cannot be",
	"browser": "a box you put a URL in and a page that comes back — the claim is " +
		"that it reads what a fetch cannot, and the only way to show that is to let " +
		"somebody try a page a fetch cannot read",
	"text": "the text to work on",

	// 3. It is browsable.
	"news": "many stories, paged", "video": "many videos", "blog": "many posts",
	"social": "a feed", "apps": "a directory you run things from",
	"chat": "rooms you talk in", "archive": "a search over everything kept",
	"stream": "this instance's timeline", "prayer": "the day's reflection, the " +
		"full timetable and the qibla — the card carries the verse and the next prayer",
}

// Every service is on exactly one side, and the sides are the services.
//
// The failure this catches is the one that was actually live: a new service
// lands, nobody asks which kind of page it should have, and it gets a
// hand-written one by default because that is what the file next door did.
func TestEveryServiceHasDecidedAboutItsPage(t *testing.T) {
	dirs, err := os.ReadDir("../service")
	if err != nil {
		t.Fatalf("reading service/: %v", err)
	}

	var missing, both []string
	seen := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		seen[name] = true
		_, isDerived := derived[name]
		_, isKept := keptItsPage[name]
		switch {
		case isDerived && isKept:
			both = append(both, name)
		case !isDerived && !isKept:
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("no decision recorded about the page for: %s\n"+
			"Every service draws its own page or is derived from its Spec. The rule is "+
			"in internal/api/service_page.go — add it to derived or to keptItsPage with "+
			"the reason, which is the part that matters.", strings.Join(missing, ", "))
	}
	for _, n := range both {
		t.Errorf("%s is listed as both derived and keeping its page", n)
	}

	// And nothing named that is not a service, so the lists cannot rot into a
	// record of directories that were deleted years ago.
	for _, lists := range []map[string]string{derived, keptItsPage} {
		for name := range lists {
			if !seen[name] {
				t.Errorf("%q is listed but there is no service/%s", name, name)
			}
		}
	}
}

// A derived page is derived: the service does not draw one.
//
// The check is that its package holds no <style> block. A service that has
// handed its page to api.ServicePage has no markup of its own to style, so a
// stylesheet there means somebody started writing a page again.
func TestADerivedServiceDrawsNothing(t *testing.T) {
	for name := range derived {
		dir := filepath.Join("..", "service", name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Errorf("reading %s: %v", dir, err)
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
				strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(b), "<style>") {
				t.Errorf("%s/%s carries a stylesheet, but %s takes its page from its "+
					"Spec. Either the page is coming back — say so by moving it to "+
					"keptItsPage — or this is left over.", name, e.Name(), name)
			}
		}
	}
}
