package apps

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAppStreamingBridge(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed to execute the browser bridge tests")
	}
	payload, err := json.Marshal(map[string]string{"bridge": appBridgeJS("assistant"), "shim": appShimJS})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bridge.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "testdata/agent-stream.mjs", path).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
