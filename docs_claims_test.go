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

	"mu/internal/api"
	"mu/internal/service"
)

// tableGroup is one sub-heading of a section and the table rows under it. A
// section with no sub-headings is a single group with an empty title.
type tableGroup struct {
	title string
	rows  [][]string
}

// tableGroups returns the markdown tables in the section that starts at
// heading and ends at the next heading of the same level, split by the `###`
// sub-headings between them. Each row is split into trimmed cells.
//
// A section used to be one table, and reading stopped at the first blank line.
// The README's tool table is now several, because one alphabetical list of
// twenty-three services asserts they are equally important — so the reader
// supplies a ranking from position, and Contacts outranks Context by being
// spelled with an S. Grouping is the fix, and it has to be a group of tables:
// prose bullets would take those services out of the table these tests read,
// which is the same as removing them from the checks.
func tableGroups(t *testing.T, file, heading string) []tableGroup {
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

	groups := []tableGroup{{}}
	for n, line := range strings.Split(body[i:], "\n") {
		line = strings.TrimSpace(line)
		if n > 0 && strings.HasPrefix(line, "## ") {
			break // the section ended
		}
		if strings.HasPrefix(line, "### ") {
			groups = append(groups, tableGroup{title: strings.TrimSpace(line[4:])})
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		// Skip the header and the |---|---| separator.
		if strings.HasPrefix(cells[0], "---") || cells[0] == "Service" || cells[0] == "Area" {
			continue
		}
		g := &groups[len(groups)-1]
		g.rows = append(g.rows, cells)
	}

	var out []tableGroup
	for _, g := range groups {
		if len(g.rows) > 0 {
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s: found no table rows under %q", file, heading)
	}
	return out
}

// tableRows is every row in the section, in the order it is read, ignoring
// which group it fell in.
func tableRows(t *testing.T, file, heading string) [][]string {
	t.Helper()
	var rows [][]string
	for _, g := range tableGroups(t, file, heading) {
		rows = append(rows, g.rows...)
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
// tool prefix. They drifted once already: Calendar for events, Faith for what
// is now prayer, Web for a service the sidebar calls Search.
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

	// Every registered service should be findable in the table, page or no
	// page. It used to be only the ones with a page, which let the headless
	// ones go undocumented without anything failing: context and memory —
	// four tools, the first thing an agent should call — were missing from the
	// README entirely and this test was satisfied.
	for _, s := range allSpecs() {
		if !seen[s.Name] {
			t.Errorf("%s is registered but has no tools in the README table", s.Name)
		}
	}
}

// The README table names tools, and a name that no longer exists reads exactly
// like one that does. This is the same rot toolrefs_test.go catches in shipped
// descriptions, in the one document most people read first: the table went on
// listing `chat` after it was removed, and `hadith`, `save` and `dismiss` for a
// commit after they were namespaced.
//
// Aliases count. Keeping the old name working is a deliberate kindness to
// anything already calling it; printing the old name in the table is different,
// because it teaches the alias instead of the tool.
func TestReadmeToolTableNamesToolsThatExist(t *testing.T) {
	registerAll(t)
	api.DeriveTools()

	real := map[string]bool{}
	for _, tool := range api.Commands() {
		real[tool.Name] = true
	}
	// Tools main() registers rather than deriving from a Spec.
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`Name:\s*"([a-z][a-z0-9_]*)"`).FindAllStringSubmatch(string(src), -1) {
		real[m[1]] = true
	}

	var gone []string
	for _, cells := range tableRows(t, "README.md", "## The tools") {
		if len(cells) < 2 {
			continue
		}
		for _, m := range backticked.FindAllStringSubmatch(cells[1], -1) {
			name := m[1]
			// Only things shaped like a tool; the cells also contain prose in
			// backticks, like `mu.db` and `id`.
			if !strings.Contains(name, "_") || strings.Contains(name, ".") {
				continue
			}
			if !real[name] {
				gone = append(gone, name+" (under "+strings.Trim(cells[0], "*")+")")
			}
		}
	}
	if len(gone) > 0 {
		t.Errorf("the README lists %d tool(s) that do not exist:\n  %s",
			len(gone), strings.Join(gone, "\n  "))
	}
}

// The tool table is alphabetical, and a new row appended to the bottom looks
// fine in a diff and wrong on the page. Database went in below Video, between
// V and W, because appending is what you do to a list you are not looking at.
//
// Alphabetical *within a group*, since the groups carry the ranking now. Across
// them it is meaningless — Context leads because it is what an agent should
// read first, not because C sorts early.
//
// Platform is last on purpose: it is the tools with no service in front of the
// underscore, so it is a remainder rather than a name to sort.
func TestReadmeToolTableIsAlphabetical(t *testing.T) {
	groups := tableGroups(t, "README.md", "## The tools")

	var all []string
	for _, g := range groups {
		var labels []string
		for _, cells := range g.rows {
			if len(cells) < 2 {
				continue
			}
			labels = append(labels, strings.Trim(cells[0], "*"))
		}
		all = append(all, labels...)

		// Platform sorts nowhere, so it is excluded wherever it appears.
		if n := len(labels); n > 0 && labels[n-1] == "Platform" {
			labels = labels[:n-1]
		}
		for i := 1; i < len(labels); i++ {
			if strings.ToLower(labels[i]) < strings.ToLower(labels[i-1]) {
				t.Errorf("under %q, %q comes after %q in the table but before it alphabetically",
					g.title, labels[i], labels[i-1])
			}
		}
	}

	if len(all) < 2 {
		t.Fatalf("found %d rows in the tool table", len(all))
	}
	if last := all[len(all)-1]; last != "Platform" {
		t.Errorf("the last row is %q; Platform is the remainder and belongs at the end", last)
	}
}
