// Package micro is a constrained, spec-driven micro-app generator.
//
// Instead of asking an LLM to emit a complete HTML/JS app in one shot (the
// hard, unreliable path), the model emits only a small JSON *spec* describing
// one of a few known micro-app shapes. A deterministic renderer turns that spec
// into a guaranteed-working app. The model describes intent; our code builds
// the artifact — so it can't produce broken markup or JS.
package micro

import (
	"fmt"
	"strings"
)

// Spec is a constrained micro-app description.
type Spec struct {
	Type  string `json:"type"`  // tracker | checklist | counter
	Title string `json:"title"` // shown as the app heading
	Emoji string `json:"emoji,omitempty"`

	// tracker: a list you add dated entries to, optionally totalling a number.
	Fields []Field `json:"fields,omitempty"`
	Sum    string  `json:"sum,omitempty"` // name of a number field to total

	// checklist: a list of checkable items.
	Items []string `json:"items,omitempty"`

	// counter: one or more +/- tallies.
	Counters []Counter `json:"counters,omitempty"`
}

// Field is one column of a tracker entry.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"` // text | number | date
}

// Counter is one tally in a counter app.
type Counter struct {
	Label string `json:"label"`
	Step  int    `json:"step,omitempty"` // increment size (default 1)
}

// Types lists the supported micro-app types.
var Types = []string{"tracker", "checklist", "counter"}

var validFieldType = map[string]bool{"text": true, "number": true, "date": true}

// Validate reports whether the spec is well-formed and renderable.
func (s *Spec) Validate() error {
	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("title is required")
	}
	switch s.Type {
	case "tracker":
		if len(s.Fields) == 0 {
			return fmt.Errorf("tracker needs at least one field")
		}
		names := map[string]bool{}
		for _, f := range s.Fields {
			if strings.TrimSpace(f.Name) == "" {
				return fmt.Errorf("every field needs a name")
			}
			if !validFieldType[f.Type] {
				return fmt.Errorf("field %q has invalid type %q (want text, number or date)", f.Name, f.Type)
			}
			names[f.Name] = true
		}
		if s.Sum != "" && !names[s.Sum] {
			return fmt.Errorf("sum field %q is not one of the fields", s.Sum)
		}
	case "checklist":
		if len(s.Items) == 0 {
			return fmt.Errorf("checklist needs at least one item")
		}
	case "counter":
		if len(s.Counters) == 0 {
			return fmt.Errorf("counter needs at least one counter")
		}
		for _, c := range s.Counters {
			if strings.TrimSpace(c.Label) == "" {
				return fmt.Errorf("every counter needs a label")
			}
		}
	default:
		return fmt.Errorf("unknown type %q (want one of: %s)", s.Type, strings.Join(Types, ", "))
	}
	return nil
}
