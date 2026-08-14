package test

// What a service is, and what it is not.
//
// Three sentences decide most of the arguments this repo has had with itself:
//
//	Services are the fundamental building block. Tools are derived from
//	services. Agents use tools.
//
// Everything follows. A service answers a question about state: request in,
// response out, deterministic given the data, callable by anything. That shape
// is what makes a tool derivable from it, which is why the derivation exists at
// all. An agent takes a goal and decides which questions to ask — it consumes
// the catalogue, so it cannot be in it.
//
// Two things were wrong when this was written, and both were the same mistake
// in different clothes: something that is not a service had been given a Spec
// so that it could have tools.
//
//   - agent_ask and agent_list. An MCP client calling agent_ask already holds
//     every tool this instance's agent holds; it was paying for a second model
//     to decide what to call. Listing your agents is a page, and there is one.
//   - wallet_balance and wallet_check, as they were then. That wallet was the
//     credit ledger — how an account pays — which is account furniture, the
//     same shelf as changing your email or rotating a token, and it was in the
//     catalogue at /services between Video and Weather until a boolean called
//     Staple was invented to hide it there. Deleting the flag was the fix; the
//     flag was the error made legible. The ledger is in account/ now, and the
//     word wallet has been given back to the thing that holds a key — which is
//     a service, and is in the catalogue on its own merits.
//
// So this asserts the rule rather than the two cases, because the cases are
// gone and the rule is what stops the next one.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Anything holding a Spec is a service, and services live under service/.
//
// One exception would be one too many: the moment a Spec lives somewhere else,
// "what is a service" stops having an answer you can check and starts having an
// answer you have to remember.
func TestEverySpecLivesUnderService(t *testing.T) {
	found := 0
	err := filepath.Walk(at(""), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !regexp.MustCompile(`Spec = service\.Spec\{`).Match(b) {
			return nil
		}
		found++
		rel, _ := filepath.Rel(at(""), path)
		if !strings.HasPrefix(rel, "service"+string(filepath.Separator)) {
			t.Errorf("%s declares a Spec outside service/ — a Spec is what makes "+
				"something a service, so either it belongs under service/ or it "+
				"should not have one", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 15 {
		t.Fatalf("only found %d Specs — the scan is broken, not the code", found)
	}
}

// Nothing that consumes tools declares a Spec.
//
// The agent is the case that matters: it reads the catalogue to decide what to
// call, so a Spec on it would put the consumer inside the thing it consumes.
// Same for anything else that reaches for the registry to choose.
func TestNothingThatUsesToolsIsAService(t *testing.T) {
	for _, consumer := range []string{"agent", "home", "client", "admin", "account"} {
		dir := at(consumer)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if regexp.MustCompile(`Spec = service\.Spec\{`).Match(b) {
				rel, _ := filepath.Rel(at(""), path)
				t.Errorf("%s declares a Spec. %s uses tools; it is not a place "+
					"they come from", rel, consumer)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// And the assembly registers no tools of its own.
//
// internal/server/tools.go was a thousand lines: every capability that had no
// service to declare it, written out by hand and dispatched through whatever
// door happened to be nearest. It is gone. A tool with nowhere to come from is
// a service that has not been written yet.
func TestTheServerRegistersNoToolsByHand(t *testing.T) {
	entries, err := os.ReadDir(at("internal", "server"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
			strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(at("internal", "server", e.Name()))
		if err != nil {
			continue
		}
		if regexp.MustCompile(`api\.RegisterTool`).Match(b) {
			t.Errorf("internal/server/%s registers a tool by hand — declare the "+
				"capability on a service and let it derive", e.Name())
		}
	}
}

// TestEveryDirectoryUnderServiceIsAService is the other half of "every Spec
// lives under service/".
//
// One exception was one too many. service/search held the /search page and its
// providers while the capability itself was the web service, and it cost twice:
// a directory under service/ that answered to no entry in the catalogue, and a
// sideways import from web to reach its own provider. The rule only means
// anything while it is true of every directory, and until this test existed
// nothing could notice a new one that was not a service.
//
// This checks the top level only, which is deliberate but was read as
// permission. The comment here used to name service/news/digest as the fine
// case — "part of news, not a service beside it" — and it was neither: it was
// an agent, reading markets and video by name to compose a blog post, one
// directory below where the sideways rule stopped looking. It is agent/digest
// now. A subdirectory is still fine when it is an implementation detail of the
// service above it; what it must not be is somewhere the rules do not reach,
// and TestServicesDoNotImportEachOther walks the whole subtree for that reason.
func TestEveryDirectoryUnderServiceIsAService(t *testing.T) {
	spec := regexp.MustCompile(`Spec = service\.Spec\{`)

	dirs, err := os.ReadDir(at("service"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		found++
		files, _ := filepath.Glob(filepath.Join(at("service", d.Name()), "*.go"))
		declares := false
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err == nil && spec.Match(b) {
				declares = true
				break
			}
		}
		if !declares {
			t.Errorf("service/%s declares no Spec — a directory here is a service, "+
				"and shared code belongs in internal/", d.Name())
		}
	}
	if found < 15 {
		t.Fatalf("only found %d directories — the scan is broken, not the code", found)
	}
}
