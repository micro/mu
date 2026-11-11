package app

import (
	"os/exec"
	"testing"
)

func TestVisibleViewPolling(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	if out, err := exec.Command(node, "testdata/polling.cjs").CombinedOutput(); err != nil {
		t.Fatalf("view polling: %v\n%s", err, out)
	}
}
