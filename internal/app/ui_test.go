package app

// The components, and the one property they all have to hold.
//
// This file was 336 lines testing twenty-four helpers, seventeen of which had
// no caller anywhere in the product. The tests were the only thing keeping them
// alive, which is why the library looked healthy while every page hand-rolled
// its markup beside it.
//
// So the tests follow the components: one per survivor, and the escaping test
// that matters more than any of them.

import (
	"strings"
	"testing"
)

func TestSearchBar(t *testing.T) {
	got := SearchBar("/find", "Search notes", "milk", "tok")
	for _, want := range []string{
		`action="/find"`, `placeholder="Search notes"`, `value="milk"`,
		// It posts, and carries the token to do so. A search box that went back
		// to GET would put what somebody typed in the URL, and from there into
		// the browser history and the reverse proxy's access log — see AGENTS.md,
		// "What may travel in a URL".
		`method="POST"`, `name="_csrf" value="tok"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, `method="GET"`) {
		t.Errorf("the search box is a GET again: %s", got)
	}
	// No token, no field — a signed-out reader has no session to derive one from
	// and is not checked.
	if got := SearchBar("/find", "x", "", ""); strings.Contains(got, "_csrf") {
		t.Errorf("an empty token still rendered a field: %s", got)
	}
}

func TestActionLink(t *testing.T) {
	got := ActionLink("/notes/new", "+ New note")
	if !strings.Contains(got, `href="/notes/new"`) || !strings.Contains(got, "+ New note") {
		t.Errorf("bad action link: %s", got)
	}
}

func TestListAndEmptyAndDesc(t *testing.T) {
	if got := List("<p>x</p>"); !strings.Contains(got, `class="card-list"`) {
		t.Errorf("List lost its container: %s", got)
	}
	if got := Empty("nothing yet"); !strings.Contains(got, "nothing yet") {
		t.Errorf("Empty lost its message: %s", got)
	}
	if got := Desc("what it does"); !strings.Contains(got, `class="card-desc"`) {
		t.Errorf("Desc lost its class: %s", got)
	}
}

func TestPage(t *testing.T) {
	got := Page(PageOpts{Search: "/find", Action: "/new", Content: `<div id="c"></div>`})
	for _, want := range []string{`action="/find"`, `href="/new"`, `id="c"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s", want)
		}
	}
	if got := Page(PageOpts{Empty: "nothing here"}); !strings.Contains(got, "nothing here") {
		t.Error("an empty page did not say so")
	}
}

// The property every one of these has to hold, tested in one place rather than
// once per component.
//
// A component that does not escape is worse than no component, because the call
// site trusted it. This is the reason to have them at all — 777 hand-written
// EscapeString calls across the product is 777 chances to forget one.
func TestEveryComponentEscapesWhatItIsGiven(t *testing.T) {
	const bad = `<script>alert(1)</script>`
	for name, got := range map[string]string{
		"SearchBar.query":   SearchBar("/x", "p", bad, "t"),
		"SearchBar.csrf":    SearchBar("/x", "p", "", bad),
		"CSRFField.token":   CSRFField(bad),
		"ActionLink.label":  ActionLink("/x", bad),
		"Empty.message":     Empty(bad),
		"Desc.text":         Desc(bad),
		"Pill.label":        Pill(bad),
		"PillLink.label":    PillLink(bad, "/x", false),
		"Page.emptyMessage": Page(PageOpts{Empty: bad}),
	} {
		if strings.Contains(got, bad) {
			t.Errorf("%s passed a script tag through unescaped:\n%s", name, got)
		}
		if !strings.Contains(got, "&lt;script&gt;") {
			t.Errorf("%s did not escape at all:\n%s", name, got)
		}
	}
}

// Actions is a bar of controls that are already markup, so it does not escape
// them — and that is the one exception, worth a test that says so out loud
// rather than a reader wondering whether it was forgotten.
func TestActionsTakesMarkup(t *testing.T) {
	got := Actions(TextLink("← Back", "/x"), `<button class="pill">Delete</button>`)
	if !strings.Contains(got, `<button class="pill">Delete</button>`) {
		t.Errorf("Actions escaped a control it was handed: %s", got)
	}
	if !strings.Contains(got, `class="actions"`) {
		t.Errorf("Actions lost its container: %s", got)
	}
}
