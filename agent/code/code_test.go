package code

// What makes this an agent rather than a prompt with a name on it.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/agent/micro"
)

// It is registered where every agent is registered, so how one is run stays
// one mechanism.
//
// The point of this package is not a second way to have an agent. Route,
// Execute and the registry are agent/micro's and are untouched; what moved out
// is the definition and what was added is everything a definition cannot hold.
func TestTheAgentIsInTheOneRegistry(t *testing.T) {
	got := micro.Get(ID)
	if got == nil {
		t.Fatal("nothing is registered as the code agent, so /agent/code is a 404")
	}
	if got != Agent {
		t.Error("something else registered itself as the code agent")
	}
	if micro.Get("micro") == nil {
		t.Error("moving Code out of the registry took Micro with it")
	}
}

// Its scope is the substance of it.
//
// Micro is every tool and no scope. This one is a machine and somewhere to put
// what it makes, and that narrowing is what makes it a second agent worth
// having rather than the first one wearing a coat — given every tool a run
// spends its attention deciding which of a hundred things it does not want.
func TestTheAgentIsScopedToAMachineAndSomewhereToPutThings(t *testing.T) {
	if len(Agent.Tools) == 0 {
		t.Fatal("the code agent has every tool, which is Micro with a different name")
	}
	want := map[string]bool{"shell": false, "apps": false}
	for _, tool := range Agent.Tools {
		if _, ok := want[tool]; !ok {
			t.Errorf("the code agent may reach %q, which is not a machine or a place to host", tool)
			continue
		}
		want[tool] = true
	}
	for tool, seen := range want {
		if !seen {
			t.Errorf("the code agent cannot reach %s", tool)
		}
	}
	// Its own memory, because what it learns is about somebody's projects.
	if Agent.MemoryScope == "" {
		t.Error("the code agent shares Micro's memory pool")
	}
}

// The rail belongs to this agent and to no other.
//
// The page renders the same rail for every agent and asks this package what to
// add. Asking unconditionally is what keeps the condition here: agent/ should
// not learn which agent has a machine.
func TestOnlyThisAgentGetsAWorkspace(t *testing.T) {
	if got := RailSection("someone", "micro"); got != "" {
		t.Errorf("Micro's page draws a workspace: %q", got)
	}
	if got := RailSection("someone", ""); got != "" {
		t.Errorf("the default agent's page draws a workspace: %q", got)
	}
	if got := RailSection("", ID); got != "" {
		t.Errorf("a signed-out reader is shown a workspace: %q", got)
	}

	got := RailSection("someone", ID)
	for _, want := range []string{"Workspace", "Apps"} {
		if !strings.Contains(got, want) {
			t.Errorf("the code agent's rail has no %s section: %q", want, got)
		}
	}
}

// A machine that is asleep or absent is said to be, not shown as empty.
//
// The two are different facts and only one of them means your work is gone.
// This box has no container runtime in a test, which is the same shape as an
// instance that does not have one — so what it says here is what a self-hoster
// without Docker sees.
func TestTheRailSaysWhyThereAreNoFiles(t *testing.T) {
	got := RailSection("someone", ID)
	work := got[:strings.Index(got, `<div class="chat-sess-head">Apps`)]
	if !strings.Contains(work, "chat-sess-empty") {
		t.Errorf("the workspace section shows nothing and explains nothing: %q", work)
	}
	// Whatever the reason is, it is a sentence rather than a blank column.
	if strings.Contains(work, "><span") || len(strings.TrimSpace(work)) < 80 {
		t.Errorf("the workspace section is empty rather than explained: %q", work)
	}
}

// /code is a real address.
//
// It was linked from two places on /apps as the way to build an app, gated in
// the route table, and served by nothing — so it fell to the catch-all and
// rendered the front page. See internal/server's link test, which could not
// see the link because ActionLink renders the anchor.
func TestCodeIsTheFrontDoor(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest(http.MethodGet, "/code", nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("/code answered %d, want a redirect", w.Code)
	}
	if got := w.Header().Get("Location"); got != Path {
		t.Errorf("/code goes to %q, want %q", got, Path)
	}
}

// Somebody else's files are not a query parameter away.
func TestReadingAFileNeedsASession(t *testing.T) {
	w := httptest.NewRecorder()
	FileHandler(w, httptest.NewRequest(http.MethodGet, "/code/file?name=notes.txt", nil))
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("a signed-out reader got %d for somebody's file", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "/login") {
		t.Errorf("a signed-out reader was sent to %q rather than to sign in", loc)
	}
}

// A file size is for reading, not for arithmetic.
func TestSizesAreReadable(t *testing.T) {
	for _, c := range []struct {
		in   int64
		want string
	}{{0, "0B"}, {512, "512B"}, {2048, "2K"}, {3 << 20, "3.0M"}} {
		if got := size(c.in); got != c.want {
			t.Errorf("size(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
