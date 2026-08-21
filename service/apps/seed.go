package apps

// The apps that ship with every instance, as files.
//
// They were Go string literals — templates.go was 1,351 lines of HTML in
// backticks — and there is no formatter, no diff worth reading and no way to
// open one in a browser. An embedded file lives with the package that owns it,
// which is what the other twelve embeds in this repo do, so a seed is a
// directory: app.json, index.html, and icon.svg where it has one.
//
// Adding an app is adding a directory. That matters more than the tidiness,
// because the ones worth shipping next are composites over several services —
// Maps, Groups, a wallet you can send from — and those are not things anybody
// writes inside a quoted string.
//
// templates.go also held ten apps nothing could reach. GetTemplate had one
// caller, this file, and it named seven of the seventeen; the other ten had
// been written and never shipped, four of them composites over the very
// services the app SDK exists to reach. They are here now, in git, with
// "seed": false — an app nobody has run is not something to put our name on,
// and flipping that flag is a one-line change once one has been.

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"mu/internal/app"

	"github.com/google/uuid"
)

//go:embed seeds
var seedFS embed.FS

// seed is one shipped app, read from its directory.
type seed struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tags        string `json:"tags"`

	// Order is where it sits in the directory, low first. Explicit rather than
	// alphabetical, because what leads the list is a decision — ours are the
	// answer to "what can this thing do", and a calculator is not it.
	Order int `json:"order"`

	// Seed is whether it ships. See the note at the top of this file.
	Seed bool `json:"seed"`

	// Note says why one is held back, for whoever opens the directory
	// wondering. Read by nothing; the point is that it sits beside the app
	// rather than in a commit message somebody has to go looking for.
	Note string `json:"note,omitempty"`

	html string
	icon string
}

// seeds reads every shipped app off the embedded tree, in the order they are
// meant to be shown.
func seeds() []seed {
	entries, err := fs.ReadDir(seedFS, "seeds")
	if err != nil {
		app.Log("apps", "no seeds: %v", err)
		return nil
	}

	var out []seed
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join("seeds", e.Name())

		b, err := seedFS.ReadFile(path.Join(dir, "app.json"))
		if err != nil {
			app.Log("apps", "seed %s has no app.json", e.Name())
			continue
		}
		var s seed
		if err := json.Unmarshal(b, &s); err != nil {
			app.Log("apps", "seed %s: %v", e.Name(), err)
			continue
		}
		if s.Slug == "" {
			s.Slug = e.Name()
		}
		if !s.Seed {
			continue
		}

		h, err := seedFS.ReadFile(path.Join(dir, "index.html"))
		if err != nil || len(strings.TrimSpace(string(h))) == 0 {
			app.Log("apps", "seed %s has no page", s.Slug)
			continue
		}
		s.html = string(h)

		if ic, err := seedFS.ReadFile(path.Join(dir, "icon.svg")); err == nil {
			s.icon = strings.TrimSpace(string(ic))
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// ensureBuiltins makes sure every built-in ("mu"-authored) app exists. It runs
// on every startup — not just first run — so a newly-added built-in appears on
// existing instances too. It only fills gaps: it never overwrites a user's app,
// or a user's in-place edits to a built-in. Think of these as the apps that ship
// with the OS; you can still build and run your own on top.
func ensureBuiltins() {
	added := 0
	mutex.Lock()

	// Official is decided here, on every start, and nowhere else.
	//
	// Cleared first, so the only thing that can make it true is being in this
	// directory right now. apps.json is a file on disk an operator can edit and
	// an app can be forked, renamed and made public; a flag meaning "we wrote
	// this" that survives either of those means nothing. It is also why an app
	// that shipped in an older release and has since been dropped loses the
	// badge on the next start rather than keeping it forever.
	for _, a := range apps {
		a.Official = false
		a.Order = 0
	}

	for _, a := range builtinApps() {
		if _, exists := apps[a.Slug]; !exists {
			apps[a.Slug] = a
			added++
		}
		apps[a.Slug].Official = true
		apps[a.Slug].Order = a.Order
	}
	mutex.Unlock()
	if added > 0 {
		save()
		app.Log("apps", "Added %d built-in apps", added)
	}
}

// builtinApps returns the apps that ship with every instance.
func builtinApps() []*App {
	now := time.Now()
	var out []*App
	for _, s := range seeds() {
		out = append(out, &App{
			ID:          uuid.New().String(),
			Slug:        s.Slug,
			Name:        s.Name,
			Description: s.Description,
			AuthorID:    "mu",
			Author:      "mu",
			Icon:        s.icon,
			Mode:        "raw",
			HTML:        s.html,
			Tags:        s.Tags,
			Order:       s.Order,
			Official:    true,
			Public:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return out
}
