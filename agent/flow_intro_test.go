package agent

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A signed-out visitor arrives from the landing's "See it working" and needs to
// be told what they are looking at. A bare chat box is the same box as on any
// other site; the claim is that the answers come from tools running here.
func TestGuestAgentPageSaysItIsUsingTools(t *testing.T) {
	rec := httptest.NewRecorder()
	servePage(rec, httptest.NewRequest("GET", "/agent", nil))
	body := rec.Body.String()

	for _, want := range []string{"agent-intro", `href="/tools"`, `href="/signup"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the guest inbox is missing %q, so the landing's promise goes unexplained", want)
		}
	}
}
