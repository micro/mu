package shell

import (
	"context"
	"strings"
	"testing"
)

// Reading a file out of the workspace is bounded by the workspace.
//
// The name arrives in a URL — /code/file?name=… — so this is the boundary
// between "your files" and "this container's filesystem". It is checked before
// anything runs, which is what makes it testable on a box with no container
// runtime at all: the refusal happens in under() and never reaches the machine.
func TestAFileOutsideTheWorkspaceIsRefused(t *testing.T) {
	for _, name := range []string{
		"../../etc/passwd",
		"/etc/passwd",
		"/work/../etc/shadow",
		"..",
	} {
		_, _, err := ReadFile(context.Background(), "someone", name, 1024)
		if err == nil {
			t.Errorf("%q was not refused — that is a path out of the workspace", name)
			continue
		}
		if !strings.Contains(err.Error(), "outside") {
			t.Errorf("%q was refused for the wrong reason: %v", name, err)
		}
	}
}

// The machine is matched by its whole name.
//
// The runtime filters containers by substring, so asking whether "mu-shell-ann"
// is up also returns "mu-shell-anna" — two accounts, one answer, and which one
// arrives depends on the order the daemon lists them. A workspace listing that
// believed a machine was awake because somebody else's is would then try to
// read files out of it.
func TestOnlyAnExactMachineNameCounts(t *testing.T) {
	up := []string{"mu-shell-anna", "mu-shell-annex"}
	if listed(up, "mu-shell-ann") {
		t.Error("a prefix of another account's machine counted as this account's")
	}
	if !listed(up, "mu-shell-anna") {
		t.Error("an exact name did not match")
	}
}

// An account with no machine is not an error, and an instance with no runtime
// is not either. Both are the ordinary state of a box nobody has built anything
// on, and a page has to be able to draw them.
func TestAnAbsentMachineIsNotAFailure(t *testing.T) {
	ws, err := WorkspaceOf(context.Background(), "nobody-has-this-account")
	if err != nil {
		t.Fatalf("looking for a machine that is not there failed: %v", err)
	}
	if ws.Awake {
		t.Error("a machine nobody started reports itself awake")
	}
	if len(ws.Files) != 0 {
		t.Errorf("a machine that was never asked reported files: %#v", ws.Files)
	}
	if ws.Home == "" {
		t.Error("the workspace does not say where the files would be")
	}
}

// And an empty account id asks nothing of the runtime.
func TestNoAccountNoLook(t *testing.T) {
	ws, err := WorkspaceOf(context.Background(), "  ")
	if err != nil || ws.Awake {
		t.Errorf("an empty account went looking for a machine: %#v %v", ws, err)
	}
}
