package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"mu/internal/auth"
)

// Run the same dependency-free wire client used against a deployed runtime.
// No Python mock replaces the Go routing, parser or authorization boundary.
func TestMUCPIndependentProbe(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("python3 is required for MUCP interoperability checks")
		}
		t.Skip("python3 unavailable")
	}
	const owner = "mucp-independent-probe"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	_, token, err := auth.CreateToken(owner, "conformance", []string{"read", "api:agent", "api:services"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(IndexHandler))
	defer server.Close()
	cmd := exec.Command(python, "../../proto/conformance/probe.py", server.URL, "--wire", "--reads")
	cmd.Env = append(os.Environ(), "MU_TOKEN="+token)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("independent probe: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
