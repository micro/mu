package places

import (
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// One form, and both verbs on it.
//
// There were two: a Search card and a Nearby card, side by side, with the same
// location field, the same radius and the same "use my location" link. Two
// boxes that differ by one input is a choice the page is making the reader make
// before they have said anything.
//
// One set of inputs now, and two buttons — because the verbs genuinely differ
// in what they cost, and a single button carrying one price badge would be
// wrong half the time.
func TestPlacesHasOneFormWithBothVerbs(t *testing.T) {
	b, err := os.ReadFile("places.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if strings.Contains(src, "renderNearbyFormHTML") {
		t.Error("the second form is back")
	}
	if n := strings.Count(src, "<form id="); n > 1 {
		// Save and delete are their own tiny forms; the search form is the one
		// with a query in it.
		if strings.Count(src, `id="places-form"`) > 1 || strings.Count(src, `id="nearby-form"`) > 0 {
			t.Errorf("places renders more than one way to ask (%d forms)", n)
		}
	}
	// Both verbs reachable from the one form, each with its own price.
	for _, want := range []string{`action="/places/search"`, `formaction="/places/nearby"`} {
		if !strings.Contains(src, want) {
			t.Errorf("the form cannot reach %s", want)
		}
	}
}

// Nearby reads the field names the shared form posts.
//
// The form calls the location "near", because to somebody filling it in that
// is what it is whichever button they press next. Nearby used to read only
// "address", so the shared form would have sent a location it ignored and then
// complained there was none.
func TestNearbyReadsTheSharedFormsFields(t *testing.T) {
	b, err := os.ReadFile("places.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{`formValue("near")`, `formValue("near_lat")`, `formValue("near_lon")`} {
		if !strings.Contains(src, want) {
			t.Errorf("nearby does not read %s, so the shared form's location is dropped", want)
		}
	}
}

// Every element the page's JavaScript reaches for is on the page.
//
// This is the bug that came with folding the two forms into one: the city
// cards and the map's geolocation both wrote into the nearby form's fields,
// and deleting that form left them writing into null. Nothing failed loudly —
// the script threw, the city links stopped working, and the page looked fine.
//
// Grepping the source for the old ids would only catch this once. Rendering
// the page and checking that each getElementById has something to find keeps
// catching it.
func TestThePagesScriptsOnlyReachForElementsThatExist(t *testing.T) {
	req := httptest.NewRequest("GET", "/places", nil)
	page := renderPlacesPage(req)

	present := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		present[m[1]] = true
	}

	asked := regexp.MustCompile(`getElementById\('([^']+)'\)`).FindAllStringSubmatch(page, -1)
	if len(asked) == 0 {
		t.Fatal("no getElementById calls found, so this test is not looking at the page")
	}
	for _, m := range asked {
		if !present[m[1]] {
			t.Errorf("the page's script reaches for #%s, which is not on the page", m[1])
		}
	}
}

// Nothing calls a function the page does not define.
//
// updateNearbyLink was wired to oninput on the location field and onchange on
// the radius, and was never defined — so typing a location threw on every
// keystroke. It had presumably kept a link between the two forms in sync, and
// outlived it.
func TestThePageDefinesEveryFunctionItCalls(t *testing.T) {
	req := httptest.NewRequest("GET", "/places", nil)
	page := renderPlacesPage(req)

	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`function (\w+)\(`).FindAllStringSubmatch(page, -1) {
		defined[m[1]] = true
	}
	// Handlers are the ones that go stale: they name a function in an
	// attribute, where nothing checks the name until somebody clicks.
	for _, m := range regexp.MustCompile(`on\w+="(\w+)\(`).FindAllStringSubmatch(page, -1) {
		switch m[1] {
		case "showToast", "return": // defined by the shell, not by this page
			continue
		}
		if !defined[m[1]] {
			t.Errorf("the page calls %s(), which it does not define", m[1])
		}
	}
}
