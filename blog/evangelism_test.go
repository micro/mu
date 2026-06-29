package blog

import "testing"

// TestEvangelismAngles guards the embedded evangelism.json: it must parse and
// carry at least one non-empty angle. This catches a malformed or empty edit
// (e.g. from the marketing pass) in CI before it ships.
func TestEvangelismAngles(t *testing.T) {
	angles := evangelismAngles()
	if len(angles) == 0 {
		t.Fatal("evangelism.json produced no angles (malformed or empty)")
	}
	for name, instruction := range angles {
		if name == "" || instruction == "" {
			t.Errorf("angle with empty name or instruction: %q -> %q", name, instruction)
		}
	}
}

// TestNextEvangelismAngle ensures rotation returns a populated angle.
func TestNextEvangelismAngle(t *testing.T) {
	name, instruction := nextEvangelismAngle()
	if name == "" || instruction == "" {
		t.Fatalf("nextEvangelismAngle returned empty: %q -> %q", name, instruction)
	}
}
