package test

// A tool the agent holds is a tool prompt injection holds.
//
// Destructive marks the methods withheld from the model: deleting a file, a
// note, a contact, an event, a task, a post; texting somebody; blocking an
// account. The test is an irreversible effect nobody asked for. Deleting your
// own file from a page is fine. Having it deleted because a web page said so is
// not, and the agent reads web pages.
//
// The flag was enforced at one door and unknown at the next. agent/native.go
// wrapped every tool call and refused the destructive ones. agent/micro — the
// specialists, whose general agent is declared with Tools: nil meaning *every*
// tool, and which the router falls back to whenever it is unsure — had no such
// check in either half: it listed them for the planner and ran whatever came
// back through ExecuteToolAs, as the account.
//
// So this asks the question of every door at once. A third one will be written
// eventually; this is what notices.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/service"
)

// TestEveryModelPlanIsFilteredForDestructiveTools finds the execution sites
// itself rather than reading a list somebody maintains.
//
// A list would have been written when there were two of them and would not have
// grown to four. What identifies one is the shape: a tool name taken from a
// parsed plan — tc.Tool — handed to ExecuteTool or ExecuteToolAs. Every such
// line must be preceded by a refusal.
//
// Not MCP: a client holding a token is a program somebody wrote, not a model
// choosing from a catalogue after reading a stranger's web page. It gets the
// destructive tools with an annotation saying so, which is what the annotation
// is for.
func TestEveryModelPlanIsFilteredForDestructiveTools(t *testing.T) {
	var checked int

	for _, dir := range []string{"agent", "agent/micro"} {
		files, _ := filepath.Glob(filepath.Join(at(""), dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			lines := strings.Split(string(b), "\n")
			for i, line := range lines {
				if !strings.Contains(line, "tc.Tool, tc.Args") {
					continue
				}
				if !strings.Contains(line, "ExecuteTool") {
					continue
				}
				checked++

				// The refusal has to be above it, in the same loop. Fifteen
				// lines is generous for the guard clauses that live there.
				lo := i - 15
				if lo < 0 {
					lo = 0
				}
				before := strings.Join(lines[lo:i], "\n")
				if !strings.Contains(before, "toolBlocked(tc.Tool)") &&
					!strings.Contains(before, "DestructiveTool(tc.Tool)") {
					rel, _ := filepath.Rel(at(""), f)
					t.Errorf("%s:%d runs a tool the model named with nothing refusing the "+
						"destructive ones — an injected page can reach files_delete from "+
						"here", rel, i+1)
				}
			}
		}
	}

	if checked < 4 {
		t.Fatalf("found only %d model-plan execution sites — this scan is broken, not "+
			"the code", checked)
	}
}

// The specialists check twice, and both halves matter.
//
// Filtering the list is not a control: a model can name a tool nobody told it
// about, from its training or from a suggestion in something it just read. The
// refusal at the point of execution is what actually stops it. Leaving only the
// list would look right in review and hold nothing.
func TestTheSpecialistsRefuseAsWellAsWithhold(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(at(""), "agent/micro/execute.go"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "DestructiveTool("); n < 2 {
		t.Errorf("agent/micro/execute.go asks %d times, want both: once so the tool is "+
			"never listed for the planner, and once so a tool the model named anyway "+
			"does not run", n)
	}
}

// And there really are destructive tools to withhold, so the checks above are
// guarding something rather than agreeing with an empty set.
func TestThereAreDestructiveToolsToWithhold(t *testing.T) {
	registerAll(t)

	var found []string
	for _, s := range allSpecs() {
		for name, ep := range s.Endpoints {
			if ep.Destructive {
				found = append(found, s.Name+"_"+strings.ToLower(name))
			}
		}
	}
	if len(found) < 5 {
		t.Fatalf("only %d destructive endpoints (%v) — either the flag has been dropped "+
			"from the specs, or this scan is broken", len(found), found)
	}

	// And the shared check recognises them by the name a tool call arrives as.
	for _, name := range found {
		if !service.DestructiveTool(name) {
			t.Errorf("service.DestructiveTool(%q) is false, so every door that asks it "+
				"would let this through", name)
		}
	}

	// It does not over-reach: reading is not deleting.
	for _, safe := range []string{"news_list", "web_search", "notes_get", "files_list", "wallet_balance"} {
		if service.DestructiveTool(safe) {
			t.Errorf("service.DestructiveTool(%q) is true — withholding a harmless tool "+
				"is how the flag stops being believed", safe)
		}
	}
}
