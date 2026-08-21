package apps

// The apps that ship, as files. See the note at the top of seed.go.

import (
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
)

// Every directory under seeds is a whole app: a manifest that parses, a slug
// that matches the directory, and a page.
//
// A seed that half-exists is worse than one that does not — ensureBuiltins runs
// on every start and skips what is already there, so a broken one is a gap
// nobody sees until somebody opens the directory.
func TestEverySeedIsComplete(t *testing.T) {
	entries, err := fs.ReadDir(seedFS, "seeds")
	if err != nil {
		t.Fatalf("reading seeds: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("only %d seeds — this scan is broken", len(entries))
	}

	slugs := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("seeds/%s is not a directory; a seed is a directory", e.Name())
			continue
		}
		dir := path.Join("seeds", e.Name())

		b, err := seedFS.ReadFile(path.Join(dir, "app.json"))
		if err != nil {
			t.Errorf("seeds/%s has no app.json", e.Name())
			continue
		}
		var s seed
		if err := json.Unmarshal(b, &s); err != nil {
			t.Errorf("seeds/%s/app.json: %v", e.Name(), err)
			continue
		}
		if s.Slug != e.Name() {
			t.Errorf("seeds/%s declares the slug %q; the directory is the slug",
				e.Name(), s.Slug)
		}
		if slugs[s.Slug] {
			t.Errorf("two seeds claim the slug %q", s.Slug)
		}
		slugs[s.Slug] = true

		if s.Name == "" || s.Description == "" {
			t.Errorf("seeds/%s has no name or no description, so its tile is blank",
				e.Name())
		}

		h, err := seedFS.ReadFile(path.Join(dir, "index.html"))
		if err != nil || len(strings.TrimSpace(string(h))) == 0 {
			t.Errorf("seeds/%s has no page", e.Name())
		}
	}
}

// A shipped app carries our name, so it has everything a tile needs — an icon
// included, which an unshipped one does not have to have yet.
func TestAShippedSeedHasAnIcon(t *testing.T) {
	got := seeds()
	if len(got) < 5 {
		t.Fatalf("only %d seeds ship — this scan is broken", len(got))
	}
	for _, s := range got {
		if s.icon == "" {
			t.Errorf("%s ships with no icon", s.Slug)
		}
		if !strings.Contains(s.icon, "<svg") {
			t.Errorf("%s has an icon that is not an svg", s.Slug)
		}
	}
}

// seeds() returns them in the order they are meant to be shown, and it is the
// declared order rather than the alphabetical one the filesystem hands back.
func TestSeedsComeBackInTheirDeclaredOrder(t *testing.T) {
	got := seeds()

	orders := make([]int, len(got))
	for i, s := range got {
		orders[i] = s.Order
	}
	if !sort.IntsAreSorted(orders) {
		t.Errorf("seeds are not in declared order: %v", orders)
	}

	// And that is not simply alphabetical, or the sort is doing nothing.
	alpha := make([]string, len(got))
	for i, s := range got {
		alpha[i] = s.Slug
	}
	sorted := append([]string(nil), alpha...)
	sort.Strings(sorted)
	same := true
	for i := range alpha {
		if alpha[i] != sorted[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("the shipped order is alphabetical, so nothing is choosing it")
	}
}

// An app nobody has run does not ship. Ten were written and never reachable —
// GetTemplate named seven of seventeen — and they are in the tree now rather
// than in the directory.
func TestAnUnshippedSeedIsNotBuiltIn(t *testing.T) {
	shipping := map[string]bool{}
	for _, s := range seeds() {
		shipping[s.Slug] = true
	}

	entries, _ := fs.ReadDir(seedFS, "seeds")
	var held int
	for _, e := range entries {
		if !shipping[e.Name()] {
			held++
		}
	}
	if held == 0 {
		t.Skip("everything ships, so there is nothing to hold back")
	}

	for _, a := range builtinApps() {
		if !shipping[a.Slug] {
			t.Errorf("%s does not ship but reached the directory", a.Slug)
		}
	}
}

// Ours lead the directory, and the rest are sorted by their own rules under
// them. The heading on the page reads off this order — see handleList — so a
// comparator that puts one official app below a community one puts a heading in
// the middle of the community list.
func TestTheDirectoryPutsOursFirst(t *testing.T) {
	list := []*App{
		{Slug: "popular", Installs: 900},
		{Slug: "second", Official: true, Order: 2},
		{Slug: "quiet", Installs: 1},
		{Slug: "first", Official: true, Order: 1},
	}
	sortForDirectory(list)

	if list[0].Slug != "first" || list[1].Slug != "second" {
		t.Fatalf("official apps are out of order: %s, %s", list[0].Slug, list[1].Slug)
	}
	if list[2].Slug != "popular" {
		t.Errorf("community apps lost their own ordering: %s came third", list[2].Slug)
	}
	// Nothing may be interleaved, or the two headings bracket the wrong rows.
	seenTheirs := false
	for _, a := range list {
		if !a.Official {
			seenTheirs = true
		} else if seenTheirs {
			t.Fatal("an official app sorted below a community one")
		}
	}
}

// Official is decided at boot and cannot be claimed.
//
// apps.json is a file an operator can edit, and an app can be forked, renamed
// and made public — so a flag meaning "we wrote this" that survives any of
// those means nothing. ensureBuiltins clears it before it sets it, which also
// takes the badge off an app that shipped in an older release and has since
// been dropped.
func TestOfficialCannotBeClaimed(t *testing.T) {
	mutex.Lock()
	saved := apps
	apps = map[string]*App{
		"impostor": {Slug: "impostor", Name: "Not ours", Official: true, Order: 1},
		"retired":  {Slug: "retired", Name: "Shipped once", Official: true, Order: 2},
	}
	mutex.Unlock()
	defer func() { mutex.Lock(); apps = saved; mutex.Unlock() }()

	ensureBuiltins()

	mutex.RLock()
	defer mutex.RUnlock()
	for _, slug := range []string{"impostor", "retired"} {
		if apps[slug].Official {
			t.Errorf("%s kept a claim to being ours", slug)
		}
		if apps[slug].Order != 0 {
			t.Errorf("%s kept a place in our ordering", slug)
		}
	}
	// And the real ones are marked, or clearing has eaten everything.
	for _, s := range seeds() {
		if a := apps[s.Slug]; a == nil || !a.Official {
			t.Errorf("%s did not come back marked as ours", s.Slug)
		}
	}
}
