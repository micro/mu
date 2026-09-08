package user

import (
	"strings"
	"testing"
)

func TestStatusIsOwnedBoundedAndClearable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SetStatus("status-alice", "Working on Mu"); err != nil {
		t.Fatal(err)
	}
	if Status("status-alice") != "Working on Mu" || Status("status-bob") != "" {
		t.Fatal("status was stored against the wrong user")
	}
	if err := SetStatus("status-alice", strings.Repeat("x", 161)); err == nil {
		t.Fatal("overlong status accepted")
	}
	if Status("status-alice") != "Working on Mu" {
		t.Fatal("rejected status overwrote the old value")
	}
	if err := SetStatus("status-alice", ""); err != nil || Status("status-alice") != "" {
		t.Fatalf("clear failed: %v", err)
	}
}
