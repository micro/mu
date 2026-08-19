package app

// The components a settings page is made of.

import (
	"strings"
	"testing"
)

// A section is a heading and a body, and the body is markup somebody else
// composed — so it is not escaped and the heading is.
func TestASectionEscapesItsTitleAndNotItsBody(t *testing.T) {
	got := Section(`<script>x</script>`, `<p class="mine">body</p>`)
	if strings.Contains(got, "<script>") {
		t.Errorf("the title was not escaped: %s", got)
	}
	if !strings.Contains(got, `<p class="mine">body</p>`) {
		t.Errorf("the body was escaped, so a section cannot hold a form: %s", got)
	}
	if !strings.Contains(SectionID("x", "T"), `id="x"`) {
		t.Error("SectionID lost its anchor")
	}
}

// One struct for every kind of input, because the difference between them is
// which fields are set — and changing a text box into a select should not mean
// finding a different function.
func TestAFieldIsWhicheverKindItsValuesSay(t *testing.T) {
	for name, c := range map[string]struct {
		f             Field
		want, notWant string
	}{
		"text":     {Field{Name: "a", Value: "v"}, `<input class="field" type="text"`, "<textarea"},
		"textarea": {Field{Name: "a", Rows: 4}, "<textarea", "<input"},
		"select":   {Field{Name: "a", Options: []Option{{Value: "x", Label: "X"}}}, "<select", "<input"},
		"hidden":   {Field{Name: "a", Type: "hidden", Value: "v"}, `type="hidden"`, "<label"},
		"labelled": {Field{Name: "a", Label: "Name"}, `<label class="field-label">Name`, ""},
	} {
		got := c.f.HTML()
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: missing %q in %s", name, c.want, got)
		}
		if c.notWant != "" && strings.Contains(got, c.notWant) {
			t.Errorf("%s: unexpectedly contains %q in %s", name, c.notWant, got)
		}
	}

	// The selected option, which is the one thing a select gets wrong silently.
	got := Field{Name: "lang", Options: []Option{
		{Value: "en", Label: "English"}, {Value: "ar", Label: "Arabic", On: true},
	}}.HTML()
	if !strings.Contains(got, `<option value="ar" selected>`) {
		t.Errorf("the chosen option is not marked: %s", got)
	}
}

// A confirm is a JavaScript string inside an HTML attribute, so it needs both
// layers escaped. Escaping one and not the other is the bug that looks fine
// until a display name has an apostrophe in it.
func TestAConfirmIsEscapedTwice(t *testing.T) {
	got := Button{Label: "Delete", Confirm: `it's gone. <b>really?</b>`}.HTML()
	if strings.Contains(got, `it's`) {
		t.Errorf("the apostrophe would end the JavaScript string early: %s", got)
	}
	if strings.Contains(got, "<b>") {
		t.Errorf("markup survived into the attribute: %s", got)
	}
}

// A form carries its token as a field rather than as something the caller
// remembers, because auth.StrictCSRF refuses a POST without one and every form
// here changes something.
func TestAFormCarriesItsToken(t *testing.T) {
	got := Form{Action: "/x", CSRF: "tok", Submit: "Save",
		Fields: []Field{{Name: "a"}}}.HTML()
	for _, want := range []string{`name="_csrf"`, `value="tok"`, `action="/x"`,
		`method="POST"`, `class="btn"`, `>Save<`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s: %s", want, got)
		}
	}
	// No token, no field — a GET search form should not carry one.
	if strings.Contains(Form{Action: "/x", Method: "GET"}.HTML(), "_csrf") {
		t.Error("a form with no token was given an empty one")
	}
}

// Hidden fields render in a stable order. A map iterates differently every
// time, which shows up as a diff in anything comparing two renders of the same
// page — including the card cache, which decides whether to redraw on a hash.
func TestHiddenFieldsDoNotShuffle(t *testing.T) {
	f := Form{Action: "/x", Hidden: map[string]string{"z": "1", "a": "2", "m": "3"}}
	first := f.HTML()
	for i := 0; i < 20; i++ {
		if f.HTML() != first {
			t.Fatal("two renders of the same form differ, so the hidden fields are unordered")
		}
	}
	if strings.Index(first, `name="a"`) > strings.Index(first, `name="z"`) {
		t.Error("hidden fields are not in name order")
	}
}

// Everything here escapes what it is handed. The components exist to make this
// true once instead of at 777 call sites.
func TestTheseComponentsEscapeToo(t *testing.T) {
	const bad = `<script>alert(1)</script>`
	for name, got := range map[string]string{
		"Section.title": Section(bad),
		"Note.text":     Note(bad),
		"Notice.text":   Notice(bad),
		"Problem.text":  Problem(bad),
		"Field.value":   Field{Name: "a", Value: bad}.HTML(),
		"Field.label":   Field{Name: "a", Label: bad}.HTML(),
		"Option.label":  Field{Name: "a", Options: []Option{{Value: "v", Label: bad}}}.HTML(),
		"Button.label":  Button{Label: bad}.HTML(),
		"Links.label":   Links([2]string{"/x", bad}),
		"Form.action":   Form{Action: bad}.HTML(),
	} {
		if strings.Contains(got, bad) {
			t.Errorf("%s passed a script tag through:\n%s", name, got)
		}
	}
}
