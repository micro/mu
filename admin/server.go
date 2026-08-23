package admin

// What this process is doing and what it is sitting on.
//
// It used to also deploy: Update ran `git pull` and `go install` and
// `systemctl restart mu`, Restart ran the last of those, Generate Digest
// kicked a background job, and a fourth button offered to delete files the
// SQLite migration had superseded.
//
// All four are gone. The three deploy actions are a shell command each, on a
// box whose owner has a shell — and having them here meant a web request could
// pull whatever was on main and restart the server, which is a deploy pipeline
// with a single admin cookie in front of it. The digest generates on its own
// schedule and did not need a fifth way to start it. The migration sweep now
// happens at boot in internal/data, because "superseded stores" is a sentence
// about our storage layer and the person reading it only runs the thing.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// UpdateHandler shows the server page.
func UpdateHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	content := `<p><a href="/admin">← Admin</a></p>
	<h2>Server</h2>
	` + app.RenderInternalStatusHTML() + storesTable()

	app.Respond(w, r, app.Response{Title: "Admin", Description: "Server", HTML: content})
}

// storesShown is how much of the data directory the table lists. The question
// it answers is "has anything got big", and the answer is at the top.
const storesShown = 12

// storesTable is what is on disk, largest first.
//
// Every store is a whole-file blob rewritten in one go, so a file that has
// quietly become the largest thing in the directory is a page or a background
// loop paying for its whole size on every write. Nothing said so anywhere, and
// the only way to find out was to go and look with ls on the box.
func storesTable() string {
	stores := data.Stores()
	if len(stores) == 0 {
		return ""
	}

	var total int64
	for _, s := range stores {
		total += s.Size
	}
	if len(stores) > storesShown {
		stores = stores[:storesShown]
	}

	// Which files the live backend does not write, so a large one can be read
	// as an archive rather than as a cost. Two index implementations leave two
	// files, and from outside a hot index and a dead one look the same.
	stale := map[string]bool{}
	for _, name := range data.Stale() {
		stale[name] = true
	}

	var b strings.Builder
	b.WriteString(`<h3>Stores</h3><table class="stats-table">`)
	for _, s := range stores {
		where := html.EscapeString(s.Name)
		if s.Files > 1 {
			where += fmt.Sprintf(` <span class="text-muted text-sm">%d files</span>`, s.Files)
		}
		if stale[strings.TrimSuffix(s.Name, "/")] {
			where += ` <span class="text-muted text-sm">not written</span>`
		}
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td></tr>`, where, app.Bytes(s.Size)))
	}
	b.WriteString(`</table>`)
	b.WriteString(fmt.Sprintf(`<p class="text-muted text-sm">%s in the data directory. `+
		`Each of these is rewritten whole when it changes, except where marked. `+
		`Search index: %s.</p>`, app.Bytes(total), html.EscapeString(data.SearchBackend())))
	return b.String()
}
