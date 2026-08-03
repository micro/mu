package main

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

	"mu/internal/service"
)

// tableRows returns the rows of the first markdown table after heading, each
// split into trimmed cells.
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

// The architecture doc's registry table is the list a reader trusts for what
// this instance can do. Every registered service must have a row, every row
// must be a registered service, and the page and account-scoped columns must
// say what the Spec says.
func TestArchitectureTableMatchesTheRegistry(t *testing.T) {
	registerAll(t)

	documented := map[string]bool{}
	for _, cells := range tableRows(t, "docs/ARCHITECTURE.md", "## What is registered") {
		if len(cells) < 4 {
			t.Errorf("malformed row: %v", cells)
			continue
		}
		name := strings.Trim(cells[0], "`")
		documented[name] = true

		spec, ok := service.SpecFor(name)
		if !ok {
			t.Errorf("the table lists %q, which is not a registered service", name)
			continue
		}

		page := cells[1]
		if page == "—" {
			page = ""
		}
		if page != spec.Page {
			t.Errorf("%s: table says page %q, Spec says %q", name, cells[1], spec.Page)
		}

		scoped := cells[3] != ""
		if scoped != spec.Scoped {
			t.Errorf("%s: table says account-scoped=%v, Spec says %v — a wrong answer here "+
				"tells a reader a service is closed to guests when it is open, or the reverse",
				name, scoped, spec.Scoped)
		}
	}

	for _, s := range allSpecs() {
		if !documented[s.Name] {
			t.Errorf("%s is registered but has no row in docs/ARCHITECTURE.md", s.Name)
		}
	}
}

// The README's tool table is grouped by service, and the group names have to be
// the names those services go by everywhere else — the sidebar, /tools, the
// tool prefix. They drifted once already: Calendar for events, Faith for islam,
// Web for a service the sidebar calls Search.
func TestReadmeToolTableUsesServiceNames(t *testing.T) {
	registerAll(t)

	seen := map[string]bool{}
	for _, cells := range tableRows(t, "README.md", "## The tools") {
		if len(cells) < 2 {
			t.Errorf("malformed row: %v", cells)
			continue
		}
		label := strings.Trim(cells[0], "*")

		// Tools with no service in front of the underscore — agent, quran,
		// save, the moderation verbs — have no service to be named after and
		// live in one row together.
		if label == "Platform" {
			for _, m := range backticked.FindAllStringSubmatch(cells[1], -1) {
				if svc, _, ok := strings.Cut(m[1], "_"); ok {
					if _, registered := service.SpecFor(svc); registered {
						t.Errorf("%s belongs under %s, not Platform", m[1], service.Label(svc))
					}
				}
			}
			continue
		}

		for _, m := range backticked.FindAllStringSubmatch(cells[1], -1) {
			svc, _, hasPrefix := strings.Cut(m[1], "_")
			if !hasPrefix {
				svc = m[1] // a bare tool named for its service, like chat
			}
			_, registered := service.SpecFor(svc)
			if !registered && !hasPrefix {
				continue // backticked prose, not a tool name
			}
			if !registered {
				t.Errorf("%s is listed under %q but %q is not a registered service", m[1], label, svc)
				continue
			}
			seen[svc] = true
			if want := service.Label(svc); label != want {
				t.Errorf("%s is listed under %q; %s is called %q in the sidebar and on /tools",
					m[1], label, svc, want)
			}
		}
	}

	// Every service a person can see should be findable in the table.
	for _, s := range allSpecs() {
		if s.Page != "" && !seen[s.Name] {
			t.Errorf("%s has a page at %s but no tools in the README table", s.Name, s.Page)
		}
	}
}
