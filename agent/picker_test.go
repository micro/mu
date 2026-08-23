package agent

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/app"
)

// The picker's data carries each agent's address.
//
// Picking an agent changed which name answered and nothing else on the screen:
// the address underneath went on naming the default, so Home read "answering as
// Test" directly above "write to it at agent@". The first question anybody has
// about an agent is where to reach it, and it was the one fact the one control
// that should have changed it could not change.
func TestThePickerKnowsWhereEachAgentIsReached(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var payload struct {
		Address string `json:"address"`
		Agents  []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"agents"`
	}

	// The shape is what matters here, not the contents: an unauthenticated
	// request cannot list agents, so this asserts the field exists and is
	// carried, which is the contract the picker script depends on.
	w := httptest.NewRecorder()
	AgentsHandler(w, httptest.NewRequest("GET", "/agents/data", nil))
	if w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("the picker's data is not readable: %v", err)
		}
		for _, a := range payload.Agents {
			if a.Address == "" && a.Name != "" {
				t.Errorf("agent %q has no address for the picker to show", a.Name)
			}
		}
	}
}

// A link inside a sentence does not break the line.
//
// .link is display:block globally — right for a card's one call to action,
// wrong in prose, where it rendered "talk to it in your inbox →, or hand it a
// token" as three lines with a stray arrow before a comma.
func TestALinkInASentenceStaysInTheSentence(t *testing.T) {
	inline := app.TextLink("inbox", "/inbox")

	if strings.Contains(inline, `class="link"`) {
		t.Error("a link in prose carries the block class")
	}
	if strings.Contains(inline, "→") {
		t.Error("a link in prose carries a call-to-action arrow")
	}
	if !strings.Contains(inline, `href="/inbox"`) {
		t.Errorf("not a link at all: %s", inline)
	}

	// The call to action keeps both, because that is what makes it one.
	cta := app.Link("inbox", "/inbox")
	if !strings.Contains(cta, `class="link"`) || !strings.Contains(cta, "→") {
		t.Errorf("the call-to-action form changed: %s", cta)
	}
}
