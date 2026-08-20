package app

// Sections, fields and buttons — the shapes a settings page is made of.
//
// Derived from what the product already draws rather than invented: across the
// repository there are 232 hand-written <button>, 141 <form>, 52 text inputs, 24
// textareas, 14 selects and 57 <h4> inside a card, and 771 inline style
// attributes holding them together. /account alone is ten titled cards with a
// form in most of them.
//
// Every one of these ships with a caller. The last component library here had
// twenty-four helpers and seventeen with no users, which is why every page
// hand-rolled its markup beside it; the rule now is that a component arrives
// with the page that needed it or it does not arrive. See ui.go.
//
// Each escapes what it is handed and carries classes defined in
// internal/app/html/mu.css. A call site should never write html.EscapeString
// for anything that goes through here — that is the whole point, and there are
// 777 hand-written escapes in the product to argue it.

import (
	htmlpkg "html"
	"strconv"
	"strings"
)

// Section is a titled block, which is what nearly every page is a list of.
//
// The body is markup — a form, a table, a paragraph somebody else composed — so
// it is not escaped. Everything else here is text and is.
func Section(title string, body ...string) string {
	var b strings.Builder
	b.WriteString(`<div class="card">`)
	if title != "" {
		b.WriteString(`<h4>` + htmlpkg.EscapeString(title) + `</h4>`)
	}
	for _, s := range body {
		b.WriteString(s)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// SectionID is Section with an anchor, for a block something links to.
func SectionID(id, title string, body ...string) string {
	s := Section(title, body...)
	return strings.Replace(s, `<div class="card">`,
		`<div class="card" id="`+htmlpkg.EscapeString(id)+`">`, 1)
}

// Note is the quiet line under a control saying what it does.
//
// It was `<p class="text-sm text-muted mt-half m-0">` written out
// 159 times with the margin guessed each time.
func Note(text string) string {
	return `<p class="note">` + htmlpkg.EscapeString(text) + `</p>`
}

// NoteHTML is Note where the sentence contains a link.
func NoteHTML(markup string) string { return `<p class="note">` + markup + `</p>` }

// Notice is what just happened: a save that worked, or one that did not.
//
// Two kinds and no more. Three shades of severity is a design somebody has to
// remember; a thing either worked or it did not.
func Notice(text string) string {
	return `<div class="notice-ok">` + htmlpkg.EscapeString(text) + `</div>`
}

// Problem is a notice that something failed.
func Problem(text string) string {
	return `<div class="notice-bad">` + htmlpkg.EscapeString(text) + `</div>`
}

// Option is one choice in a select.
type Option struct {
	Value, Label string
	On           bool
}

// Field is one input.
//
// One struct rather than a function per input type, because the difference
// between a text box, a textarea and a select is which fields are set — and a
// caller that wants to change one into another should not have to find a
// different function.
type Field struct {
	Name        string
	Label       string // shown above the input; empty for a bare field
	Value       string
	Placeholder string
	Type        string // text (default), email, password, number, hidden
	Max         int    // maxlength
	Rows        int    // > 0 makes it a textarea
	Options     []Option
	Required    bool
	ID          string // when something else has to find it
	Wide        bool   // full width rather than sitting in a row
}

// HTML renders the field.
func (f Field) HTML() string {
	attrs := ` name="` + htmlpkg.EscapeString(f.Name) + `"`
	if f.ID != "" {
		attrs += ` id="` + htmlpkg.EscapeString(f.ID) + `"`
	}
	if f.Required {
		attrs += ` required`
	}
	if f.Max > 0 {
		attrs += ` maxlength="` + strconv.Itoa(f.Max) + `"`
	}
	if f.Placeholder != "" {
		attrs += ` placeholder="` + htmlpkg.EscapeString(f.Placeholder) + `"`
	}

	cls := "field"
	if f.Wide {
		cls += " field-wide"
	}

	var input string
	switch {
	case f.Type == "hidden":
		// No label, no wrapper: a hidden field is not something anybody sees.
		return `<input type="hidden"` + attrs + ` value="` + htmlpkg.EscapeString(f.Value) + `">`
	case len(f.Options) > 0:
		var b strings.Builder
		b.WriteString(`<select class="` + cls + `"` + attrs + `>`)
		for _, o := range f.Options {
			sel := ""
			if o.On {
				sel = " selected"
			}
			b.WriteString(`<option value="` + htmlpkg.EscapeString(o.Value) + `"` + sel + `>` +
				htmlpkg.EscapeString(o.Label) + `</option>`)
		}
		b.WriteString(`</select>`)
		input = b.String()
	case f.Rows > 0:
		input = `<textarea class="` + cls + `"` + attrs + ` rows="` + strconv.Itoa(f.Rows) + `">` +
			htmlpkg.EscapeString(f.Value) + `</textarea>`
	default:
		t := f.Type
		if t == "" {
			t = "text"
		}
		input = `<input class="` + cls + `" type="` + htmlpkg.EscapeString(t) + `"` + attrs +
			` value="` + htmlpkg.EscapeString(f.Value) + `">`
	}

	if f.Label == "" {
		return input
	}
	return `<label class="field-label">` + htmlpkg.EscapeString(f.Label) + input + `</label>`
}

// Kind is how loud a button is.
type Kind int

const (
	// Primary is the one thing this form is for. One per form.
	Primary Kind = iota
	// Quiet is a second action beside it.
	Quiet
	// Danger is destructive and asks first.
	Danger
)

// Button is a control that does something.
type Button struct {
	Label   string
	Kind    Kind
	Name    string // with Value, for a form with more than one button
	Value   string
	Href    string // renders a link that looks like a button
	Confirm string // asks before submitting
	Type    string // "submit" (default) or "button"
	OnClick string // for a control the page wires up itself
}

// HTML renders the button.
func (b Button) HTML() string {
	cls := "btn"
	switch b.Kind {
	case Quiet:
		cls = "btn btn-quiet"
	case Danger:
		cls = "btn btn-danger"
	}

	if b.Href != "" {
		return `<a class="` + cls + `" href="` + htmlpkg.EscapeString(b.Href) + `">` +
			htmlpkg.EscapeString(b.Label) + `</a>`
	}

	t := b.Type
	if t == "" {
		t = "submit"
	}
	attrs := ` type="` + htmlpkg.EscapeString(t) + `" class="` + cls + `"`
	if b.Name != "" {
		attrs += ` name="` + htmlpkg.EscapeString(b.Name) + `" value="` +
			htmlpkg.EscapeString(b.Value) + `"`
	}
	if b.Confirm != "" {
		attrs += ` onclick="return confirm('` + jsQuote(b.Confirm) + `')"`
	} else if b.OnClick != "" {
		attrs += ` onclick="` + htmlpkg.EscapeString(b.OnClick) + `"`
	}
	return `<button` + attrs + `>` + htmlpkg.EscapeString(b.Label) + `</button>`
}

// jsQuote escapes a string for a single-quoted JavaScript literal inside an
// HTML attribute. Both layers, because the attribute is parsed as HTML first
// and the value as JavaScript second — escaping one and not the other is the
// bug that looks fine until somebody's display name has an apostrophe in it.
func jsQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return htmlpkg.EscapeString(s)
}

// Form is fields and a button that submits them.
//
// CSRF is the token, and it is a field rather than something the caller
// remembers to add: a POST that leans on the session cookie alone is refused by
// auth.StrictCSRF, and every form here changes something.
type Form struct {
	Action  string
	Method  string // POST (default) or GET
	CSRF    string
	Hidden  map[string]string
	Fields  []Field
	Submit  string // the primary button's label; empty renders no button
	Extra   []Button
	Inline  bool   // one row rather than stacked
	Class   string // an extra class, for a form with a layout of its own
	Confirm string // asks before submitting
}

// HTML renders the form.
func (f Form) HTML() string {
	method := f.Method
	if method == "" {
		method = "POST"
	}
	cls := "form"
	if f.Inline {
		cls += " form-inline"
	}
	if f.Class != "" {
		cls += " " + f.Class
	}

	var b strings.Builder
	b.WriteString(`<form class="` + cls + `" method="` + htmlpkg.EscapeString(method) +
		`" action="` + htmlpkg.EscapeString(f.Action) + `"`)
	if f.Confirm != "" {
		b.WriteString(` onsubmit="return confirm('` + jsQuote(f.Confirm) + `')"`)
	}
	b.WriteString(`>`)

	if f.CSRF != "" {
		b.WriteString(Field{Name: "_csrf", Type: "hidden", Value: f.CSRF}.HTML())
	}
	// Sorted, so the markup does not change between renders for no reason —
	// a map iterates in a different order every time and that shows up as a
	// diff in anything that compares two renders.
	for _, k := range sortedKeys(f.Hidden) {
		b.WriteString(Field{Name: k, Type: "hidden", Value: f.Hidden[k]}.HTML())
	}
	for _, fl := range f.Fields {
		b.WriteString(fl.HTML())
	}
	if f.Submit != "" || len(f.Extra) > 0 {
		b.WriteString(`<div class="form-actions">`)
		if f.Submit != "" {
			b.WriteString(Button{Label: f.Submit}.HTML())
		}
		for _, x := range f.Extra {
			b.WriteString(x.HTML())
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</form>`)
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Links is a list of places to go, which is what a settings block usually is
// once the forms are taken out of it.
func Links(pairs ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<div class="links">`)
	for _, p := range pairs {
		b.WriteString(`<a href="` + htmlpkg.EscapeString(p[0]) + `">` +
			htmlpkg.EscapeString(p[1]) + ` &rarr;</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
