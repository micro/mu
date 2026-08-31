package test

// AGENTS.md counts what it claims to count.
//
// The file opens by saying how many services and tools this binary carries.
// That is the most useful sentence in it for somebody arriving — and it is a
// number in prose, which is the kind of claim that is true the day it is
// written and quietly wrong three services later. Nothing else in the file has
// that property: every other statement is about a rule, and a rule that stops
// holding fails a test somewhere.
//
// So this is the test somewhere. It reads the numbers out of the sentence and
// asks the registry, which is the same thing `mu tools` prints and the same
// thing /mcp serves.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// claimed pulls "35 services and 119 tools" out of the opening sentence.
var claimed = regexp.MustCompile(`(\d+) services\s+and (\d+) tools`)

func TestTheOpeningCountsAreTrue(t *testing.T) {
	b, err := os.ReadFile(at("AGENTS.md"))
	if err != nil {
		t.Fatalf("reading AGENTS.md: %v", err)
	}
	m := claimed.FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("AGENTS.md no longer says how many services and tools there " +
			"are. If the sentence was deliberately dropped, drop this test with " +
			"it; if it was reworded, teach the regexp the new wording — a count " +
			"nothing checks is a count that goes stale")
	}
	wantServices, _ := strconv.Atoi(m[1])
	wantTools, _ := strconv.Atoi(m[2])

	// allSpecs, not service.Specs(). The registry is filled by each service's
	// Load(), which the server calls at boot and this package does not — so
	// service.Specs() is empty here unless some other test in the run happened
	// to load first, and a test that depends on that is a test that passes for
	// the wrong reason. allSpecs is the list spec_policy_test.go keeps, and
	// TestEveryServiceIsLoadedAtBoot is what holds it to what main() registers.
	specs := allSpecs()
	tools := 0
	for _, sp := range specs {
		tools += len(sp.Endpoints)
	}

	if len(specs) != wantServices {
		t.Errorf("AGENTS.md says %d services and there are %d",
			wantServices, len(specs))
	}
	if tools != wantTools {
		t.Errorf("AGENTS.md says %d tools and there are %d", wantTools, tools)
	}
}

// And the doors it lists are doors that exist.
//
// The table under "What is in the binary" names five routes. A table of
// entry points is the first thing somebody checks and the last thing anybody
// updates, so each one is asserted against the source that serves it rather
// than against a memory of it.
func TestTheDoorsInTheOpeningAreReal(t *testing.T) {
	doc, err := os.ReadFile(at("AGENTS.md"))
	if err != nil {
		t.Fatalf("reading AGENTS.md: %v", err)
	}
	routes, err := os.ReadFile(at("internal/server/routes.go"))
	if err != nil {
		t.Fatalf("reading routes.go: %v", err)
	}

	for _, door := range []struct{ doc, served string }{
		{"/api/v1/", `"/api/v1/`},
		{"/mcp", `"/mcp"`},
	} {
		if !strings.Contains(string(doc), door.doc) {
			t.Errorf("AGENTS.md no longer names the %s door", door.doc)
		}
		if !strings.Contains(string(routes), door.served) {
			t.Errorf("AGENTS.md names %s and routes.go does not serve it",
				door.doc)
		}
	}

	// The protocol servers, each named with the file that listens. A path in
	// prose is a promise about where to look, and these are the files somebody
	// arriving is most likely to want.
	for _, f := range []string{
		"service/mail/smtp.go",
		"service/mail/imap.go",
		"service/chat/xmpp.go",
		"service/shell/ssh.go",
	} {
		if !strings.Contains(string(doc), f) {
			continue // not every file has to be cited
		}
		if _, err := os.Stat(at(f)); err != nil {
			t.Errorf("AGENTS.md points at %s and it is not there: %v", f, err)
		}
	}
}
