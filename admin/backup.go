package admin

// Whether the backups are actually happening.
//
// A backup nobody has looked at is a hope, not a backup, and the failure mode
// is silent by construction: snapshots stop, everything keeps working, and you
// find out on the day you needed one. So this page answers three questions in
// the order somebody panicking would ask them — is it running, what is there,
// and did anything fail to load.
//
// The button is here because "back up now" is what you want immediately before
// doing something you are not sure about, and reaching for a shell at that
// moment is how it gets skipped.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/backup"
	"mu/internal/data"
	"mu/internal/usage"
)

// BackupHandler shows /admin/backup.
func BackupHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireAdmin(r); err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	var notice string
	if r.Method == http.MethodPost {
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		snap, err := backup.Take()
		if err != nil {
			notice = `<div class="card backup-bad">Could not take a snapshot: ` +
				html.EscapeString(err.Error()) + `</div>`
		} else {
			notice = fmt.Sprintf(`<div class="card backup-ok">Snapshot taken: %d files, %s.</div>`,
				snap.Files, size(snap.Bytes))
		}
	}

	snaps := backup.List()

	var sb strings.Builder
	sb.WriteString(usage.CSS)
	sb.WriteString(backupCSS)
	sb.WriteString(notice)

	sb.WriteString(`<div class="card"><div class="traffic-stats">`)
	usage.Stat(&sb, "Snapshots", len(snaps))
	sb.WriteString(`<div class="traffic-stat"><span class="traffic-stat-n">` +
		html.EscapeString(lastRun(snaps)) + `</span><span class="traffic-stat-l">last taken</span></div>`)
	sb.WriteString(`<div class="traffic-stat"><span class="traffic-stat-n">` +
		html.EscapeString(size(onDisk(snaps))) + `</span><span class="traffic-stat-l">stored (before hardlinks)</span></div>`)
	sb.WriteString(`</div>`)
	fmt.Fprintf(&sb, `<p class="card-desc">One a day, and one at startup. Files that `+
		`have not changed are hardlinked to the previous snapshot, so the real cost `+
		`on disk is far below the figure above — that is the size if every copy `+
		`were a copy. The whole directory is held under %s: a count is not a `+
		`budget, and a backup that fills the disk stops the thing it protects.</p>`,
		html.EscapeString(size(backup.MaxBytes)))
	// The token from the cookie, in the form: a POST resting on the session
	// needs it, because StrictCSRF refuses a request that simply omits one.
	fmt.Fprintf(&sb, `<form method="POST" class="d-inline">`+
		`<input type="hidden" name="csrf_token" value="%s">`+
		`<button type="submit" class="btn-sm">Back up now</button></form>`,
		html.EscapeString(auth.CSRFToken(r)))
	sb.WriteString(`</div>`)

	sb.WriteString(offBox())
	sb.WriteString(quarantine())
	sb.WriteString(snapshotTable(snaps))
	sb.WriteString(backupCaveats())

	app.Respond(w, r, app.Response{
		Title:       "Backup",
		Description: "Snapshots of this instance's data",
		HTML:        sb.String(),
	})
}

// endpointNote says what the object-store location means, because provider
// consoles often show a bucket-qualified endpoint and the bucket is configured
// separately here.
func endpointNote() string {
	return `<p class="card-desc"><code>S3_ENDPOINT</code> is the region — ` +
		`<code>https://lon1.digitaloceanspaces.com</code>, not the address with ` +
		`the bucket in front of it. The bucket is shared with file storage: user ` +
		`files live under <code>files/</code> and archives under <code>` +
		backup.DefaultPrefix + `/</code>.</p>`
}

// offBox says whether anything leaves this machine.
//
// Separately from the snapshots, and above them, because they answer different
// questions: the snapshots survive a bad write, and only this survives losing
// the disk. An operator reading a page full of green snapshots should not come
// away thinking they are covered for the second thing.
func offBox() string {
	if !backup.PushEnabled() {
		return `<div class="card"><span class="card-title">Off-box copy</span>` +
			`<p class="card-desc">Not configured. Everything above is on the same ` +
			`disk as the data it protects, so none of it survives losing this ` +
			`machine. Set <code>S3_BUCKET</code> and the keys beside it in ` +
			`<a href="/admin/config">Environment</a>, then <code>BACKUP_S3</code>, ` +
			`and an archive goes out daily — the data, the search index, and the ` +
			`encryption keys, because a restore without them is an inbox nobody ` +
			`can read.</p>` + endpointNote() + `</div>`
	}
	at, key, failure := backup.LastPush()
	var sb strings.Builder
	class := "backup-ok"
	if failure != "" || at.IsZero() {
		class = "backup-bad"
	}
	fmt.Fprintf(&sb, `<div class="card %s"><span class="card-title">Off-box copy</span>`, class)
	switch {
	case failure != "":
		fmt.Fprintf(&sb, `<p class="card-desc">The last attempt failed: <code>%s</code>. `+
			`Switched on is not the same as working.</p>`, html.EscapeString(failure))
	case at.IsZero():
		sb.WriteString(`<p class="card-desc">Switched on, and nothing has gone out yet. ` +
			`The first archive goes shortly after startup.</p>`)
	default:
		fmt.Fprintf(&sb, `<p class="card-desc">Last archive %s, as <code>%s</code>. `+
			`It holds the data, the search index and the encryption keys — treat the `+
			`bucket accordingly.</p>`,
			html.EscapeString(app.TimeAgo(at)), html.EscapeString(key))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// quarantine reports stores that would not load.
//
// Loudly, and at the top, because this is the state where data is sitting in a
// file under another name and nobody knows: the instance runs, the pages
// render, and one store is quietly empty.
func quarantine() string {
	bad := data.Quarantined()
	if len(bad) == 0 {
		return ""
	}
	keys := make([]string, 0, len(bad))
	for k := range bad {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(`<div class="card backup-bad"><span class="card-title">A store did not load</span>`)
	sb.WriteString(`<p class="card-desc">These files could not be read and were moved ` +
		`out of the way before anything could overwrite them. What was in them is ` +
		`still on disk under the name below. Each of these stores started empty.</p><ul class="caveats">`)
	for _, k := range keys {
		fmt.Fprintf(&sb, `<li><b>%s</b> → <code>%s</code></li>`,
			html.EscapeString(k), html.EscapeString(bad[k]))
	}
	sb.WriteString(`</ul></div>`)
	return sb.String()
}

func snapshotTable(snaps []backup.Snapshot) string {
	var sb strings.Builder
	sb.WriteString(`<div class="card"><span class="card-title">Snapshots</span>`)
	if len(snaps) == 0 {
		sb.WriteString(`<p class="card-desc">None yet. One is taken at startup, so ` +
			`either this instance has just come up or something is wrong — the ` +
			`button above will say which.</p></div>`)
		return sb.String()
	}
	sb.WriteString(`<div class="cohort-scroll"><table class="cohort">`)
	sb.WriteString(`<tr><th>Taken</th><th>Age</th><th>Files</th><th>Size</th></tr>`)
	for _, s := range snaps {
		fmt.Fprintf(&sb, `<tr><td>%s</td><td>%s</td><td class="n">%d</td><td class="n">%s</td></tr>`,
			html.EscapeString(s.At.Format("2 Jan 15:04")),
			html.EscapeString(app.TimeAgo(s.At)),
			s.Files, html.EscapeString(size(s.Bytes)))
	}
	sb.WriteString(`</table></div>`)
	fmt.Fprintf(&sb, `<p class="card-desc">In <code>%s</code>. Recent ones are kept in `+
		`full; older ones thin to one a day, for a month. Restoring is copying a `+
		`file back — they are ordinary JSON.</p></div>`, html.EscapeString(backup.Dir()))
	return sb.String()
}

func backupCaveats() string {
	return `<div class="card"><span class="card-title">What this does not cover</span><ul class="caveats">` +
		`<li><b>The disk.</b> These snapshots are on the same machine as the thing ` +
		`they protect. They survive a bad write and an operator mistake; they do ` +
		`not survive losing the instance. That needs a copy somewhere else.</li>` +
		`<li><b>The search index.</b> Deliberately not here. It is the largest ` +
		`thing in the data directory, it changes constantly, and it cannot be ` +
		`hardlinked — so every snapshot would cost another whole copy of it. It ` +
		`also protects nothing: the event that loses the index is losing the disk, ` +
		`which loses these snapshots too. It belongs in the off-box copy.</li>` +
		`<li><b>The keys.</b> Snapshots cover the data directory. The encryption ` +
		`key, the mail signing key and the CLI's wallet seed live in ` +
		`<code>keys/</code> — without them a copy of encrypted mail is unreadable, ` +
		`so an off-box backup has to include them and therefore has to be treated ` +
		`as the most sensitive thing you own.</li>` +
		`<li><b>Consistency across files, not within them.</b> Every file is whole, ` +
		`because every write is atomic. A snapshot taken mid-flight can still catch ` +
		`one store updated and a related one not.</li>` +
		`<li><b>A restore nobody has tried.</b> Nothing here has been restored from. ` +
		`That is the usual reason backups fail.</li>` +
		`</ul></div>`
}

func lastRun(snaps []backup.Snapshot) string {
	at := backup.Last()
	if at.IsZero() {
		if len(snaps) == 0 {
			return "never"
		}
		at = snaps[0].At
	}
	return app.TimeAgo(at)
}

func onDisk(snaps []backup.Snapshot) int64 {
	var n int64
	for _, s := range snaps {
		n += s.Bytes
	}
	return n
}

func size(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

var _ = time.Now

const backupCSS = `<style>
.backup-ok { border-left: 3px solid #1f6f5c; }
.backup-bad { border-left: 3px solid #a8321b; }
.backup-bad code { font-size: 12px; word-break: break-all; }
</style>`
