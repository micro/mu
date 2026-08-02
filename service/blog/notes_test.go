package blog

import "testing"

// TestNoteAngles guards the embedded notes.json: it must parse and carry at
// least one non-empty angle. This catches a malformed or empty edit (e.g. from
// the marketing pass) in CI before it ships.
func TestNoteAngles(t *testing.T) {
	angles := noteAngles()
	if len(angles) == 0 {
		t.Fatal("notes.json produced no angles (malformed or empty)")
	}
	for name, instruction := range angles {
		if name == "" || instruction == "" {
			t.Errorf("angle with empty name or instruction: %q -> %q", name, instruction)
		}
	}
}

// TestNextNote ensures rotation returns a populated angle.
func TestNextNote(t *testing.T) {
	name, instruction := nextNote()
	if name == "" || instruction == "" {
		t.Fatalf("nextNote returned empty: %q -> %q", name, instruction)
	}
}
