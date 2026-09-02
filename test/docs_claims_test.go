package test

// The documentation makes checkable claims about the registry: which services
// exist, where each one lives, which are closed to guests, and what the tools
// are called. Prose has no compiler, so those claims rot — the architecture doc
// was carrying a service that had been renamed, a scoping flag that had been
// reversed, and a directory list two packages out of date, all of which read
// perfectly well.
//
// These tests read the markdown and compare it with the Specs. They cover the
// tables, which are the parts that are mechanically checkable; the prose around
// them still needs a human.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"mu/internal/tool"

	"mu/internal/api"
	"mu/internal/service"
)

// tableRows returns the rows of the markdown table after heading, each split
// into trimmed cells.
func tableRows(t *testing.T, file, heading string) [][]string {
	t.Helper()

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	body := string(b)

	i := strings.Index(body, heading)
	if i < 0 {
		t.Fatalf("%s has no %q section", file, heading)
	}

	var rows [][]string
	started := false
	for _, line := range strings.Split(body[i:], "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if started {
				break // the table ended
			}
			continue
		}
		started = true
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		// Skip the header and the |---|---| separator.
		if strings.HasPrefix(cells[0], "---") || cells[0] == "Service" || cells[0] == "Area" {
			continue
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		t.Fatalf("%s: found no table rows under %q", file, heading)
	}
	return rows
}

var backticked = regexp.MustCompile("`([a-z0-9_]+)`")

// The README's command examples name real tools.
//
// There was a "## Services" table here and three tests read it: that every
// group was named what the service is called elsewhere, that every tool in it
// existed, and that it was alphabetical. The README has been compacted and the
// table is gone, so all three were reading a heading that is not there.
//
// The claim survives the table, which is why this is a repoint rather than a
// deletion. The CLI section is a block of worked examples — "mu news search",
// "mu markets list" — and an example naming a renamed service reads exactly as
// well as one that does not. That is the same rot the table tests caught, in
// the part of the document people actually copy from.
//
// Only examples whose first word is a registered service are checked. The rest
// of the block is CLI verbs — login, config, ask, agent, help — which are not
// tools and have no service in front of them.
func TestTheReadmesCommandExamplesNameToolsThatExist(t *testing.T) {
	registerAll(t)
	tool.DeriveTools()

	real := map[string]bool{}
	for _, c := range api.Commands() {
		real[c.Name] = true
	}
	for _, m := range regexp.MustCompile(`Name:\s*"([a-z][a-z0-9_]*)"`).
		FindAllStringSubmatch(registrationSource(t), -1) {
		real[m[1]] = true
	}

	b, err := os.ReadFile(at("README.md"))
	if err != nil {
		t.Fatal(err)
	}

	// "mu <service> <method>" at the start of a line in a fenced block, which
	// is how every example in the file is written.
	example := regexp.MustCompile(`(?m)^mu ([a-z][a-z0-9]*) ([a-z][a-z0-9_]*)`)
	found := example.FindAllStringSubmatch(string(b), -1)
	if len(found) == 0 {
		t.Fatal("no `mu <service> <method>` examples in the README — the CLI " +
			"section has moved and this test is reading the wrong thing")
	}

	checked := 0
	for _, m := range found {
		svc, method := m[1], m[2]
		if _, registered := service.SpecFor(svc); !registered {
			continue // a CLI verb, not a service: login, config, ask, help
		}
		checked++
		if name := svc + "_" + method; !real[name] {
			t.Errorf("the README shows `mu %s %s`, and %s is not a tool that exists",
				svc, method, name)
		}
	}
	if checked == 0 {
		t.Error("not one README example named a registered service, so this test " +
			"passed without checking anything")
	}
}
