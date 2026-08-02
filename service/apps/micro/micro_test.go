package micro

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		ok   bool
	}{
		{"tracker ok", Spec{Type: "tracker", Title: "Expenses", Fields: []Field{{Name: "Amount", Type: "number"}}, Sum: "Amount"}, true},
		{"tracker no fields", Spec{Type: "tracker", Title: "Expenses"}, false},
		{"tracker bad field type", Spec{Type: "tracker", Title: "X", Fields: []Field{{Name: "A", Type: "money"}}}, false},
		{"tracker sum not a field", Spec{Type: "tracker", Title: "X", Fields: []Field{{Name: "A", Type: "number"}}, Sum: "B"}, false},
		{"checklist ok", Spec{Type: "checklist", Title: "Pack", Items: []string{"Socks"}}, true},
		{"checklist empty", Spec{Type: "checklist", Title: "Pack"}, false},
		{"counter ok", Spec{Type: "counter", Title: "Water", Counters: []Counter{{Label: "Glasses"}}}, true},
		{"counter no label", Spec{Type: "counter", Title: "Water", Counters: []Counter{{Step: 2}}}, false},
		{"no title", Spec{Type: "counter", Counters: []Counter{{Label: "X"}}}, false},
		{"unknown type", Spec{Type: "graph", Title: "X"}, false},
	}
	for _, c := range cases {
		err := c.spec.Validate()
		if c.ok && err != nil {
			t.Errorf("%s: expected valid, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: expected invalid, got nil", c.name)
		}
	}
}

func TestRender(t *testing.T) {
	s := &Spec{Type: "tracker", Title: "My <Expenses>", Emoji: "💸", Fields: []Field{{Name: "Amount", Type: "number"}}, Sum: "Amount"}
	html, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing doctype")
	}
	// Title is HTML-escaped in the <title>.
	if !strings.Contains(html, "My &lt;Expenses&gt;") {
		t.Error("title not escaped in markup")
	}
	// Spec JSON is embedded for the runtime; json.Marshal escapes < and >.
	if !strings.Contains(html, `var SPEC={`) {
		t.Error("spec not embedded")
	}
	if strings.Contains(html, "var SPEC=") && strings.Contains(html, "<Expenses>") {
		t.Error("raw < leaked into embedded JSON")
	}
}

func TestRenderInvalid(t *testing.T) {
	if _, err := Render(&Spec{Type: "tracker", Title: "X"}); err == nil {
		t.Error("expected render to reject an invalid spec")
	}
}
