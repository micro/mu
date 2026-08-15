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

// TestNoAgentReachesTheRawDoor is the rule stated so a new caller cannot start
// with it off.
//
// This used to look for the guard clause above each execution site: find every
// line handing tc.Tool to ExecuteTool, then require a refusal within fifteen
// lines above it. That caught the four that existed, and it was still the wrong
// shape — it says the guards must be written out again at the fifth site, and
// the reason there were only three working guards in the first place is that
// somebody wrote a site and did not write the guards.
//
// So the guards moved into api.RunPlanned and api.RunPlannedAs, and what is
// checked is that nothing under agent/ calls the raw doors. A new execution
// site has one function to call, and it asks.
//
// Not MCP: a client holding a token is a program somebody wrote, not a model
// choosing from a catalogue after reading a stranger's web page. It reaches
// ExecuteTool directly and gets the destructive tools with an annotation saying
// so, which is what the annotation is for.
func TestNoAgentReachesTheRawDoor(t *testing.T) {
	var planned int

	err := filepath.Walk(at("agent"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(at(""), path)
		for i, line := range strings.Split(string(b), "\n") {
			switch {
			case strings.Contains(line, "api.RunPlanned"):
				planned++
			case strings.Contains(line, "api.ExecuteTool"):
				// Unless the agent named the tool itself. agent/blog calls
				// blog_create, web_search and web_fetch as literals — it decided
				// to publish, and going through the tool layer is what makes it
				// a counted, charged caller rather than one reaching into the
				// package. A literal is a decision a programmer made and can be
				// read in review; a variable is a decision a model made after
				// reading a web page. That is the whole distinction, and it is
				// visible right here in the argument.
				call := line[strings.Index(line, "api.ExecuteTool"):]
				if strings.Contains(call, `"`) {
					continue
				}
				t.Errorf("%s:%d hands the raw door a tool name it did not write — an "+
					"agent runs a model's chosen tool through api.RunPlanned or "+
					"api.RunPlannedAs, which refuse the destructive ones and the ones "+
					"a guest may not have. From here an injected page reaches "+
					"files_delete", rel, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if planned < 4 {
		t.Fatalf("found only %d planned execution sites — this scan is broken, not "+
			"the code", planned)
	}
}

// And the one path that cannot use that door still asks the same question.
//
// The native path is dispatched by go-micro rather than by us, so it wraps the
// call instead of routing it. That is a legitimate second site and the reason
// AllowPlanned is exported; what would not be legitimate is a second opinion,
// so this pins it to the shared function rather than to a re-derivation.
func TestTheNativePathAsksTheSameFunction(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(at(""), "agent/native.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "api.AllowPlanned(") {
		t.Error("agent/native.go no longer asks api.AllowPlanned — the go-micro path " +
			"dispatches its own calls, so if it is deciding for itself the two doors " +
			"can disagree about what is destructive")
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
	src := string(b)
	if !strings.Contains(src, "api.AllowPlanned(") {
		t.Error("agent/micro/execute.go does not filter the list it shows the planner — " +
			"the general agent has Tools: nil, meaning every tool, and the router " +
			"falls back to it whenever it is unsure")
	}
	if !strings.Contains(src, "api.RunPlannedAs(") {
		t.Error("agent/micro/execute.go does not refuse at the point of execution — a " +
			"model can name a tool nobody listed for it")
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
