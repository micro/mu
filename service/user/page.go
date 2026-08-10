package user

// The page for the things this account has decided about.
//
// Saving, hiding and blocking all worked and none of them could be reviewed in
// one place: saved items were at /app/saved, blocked accounts at /app/blocked,
// two links buried in the Settings card, and hidden items were nowhere at all —
// hiding was write-only, so dismissing something removed it from view with no
// way to find out what you had dismissed or to undo it.
//
// This is the service's own page, at /user, the same as every other service
// with something to show. Three lists and an undo beside each row, because the
// undo is the point: a control you cannot reverse is one people learn not to
// touch.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/flag"
)

// Handler serves /user.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	who := sess.Account

	saved := app.GetSavedList(who)
	hidden := app.GetHiddenList(who)
	blocked := sortedKeys(app.GetBlockedUsers(who))

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{
			"saved": saved, "hidden": hidden, "blocked": blocked,
		})
		return
	}

	var saveRows, hideRows, blockRows strings.Builder
	for _, it := range saved {
		saveRows.WriteString(row(
			`<a href="`+html.EscapeString(it.URL)+`">`+html.EscapeString(it.Title)+`</a>`,
			html.EscapeString(it.Type)+" · "+it.SavedAt.Format("2 Jan 15:04"),
			"remove", "/user/unsave?type="+html.EscapeString(it.Type)+"&id="+html.EscapeString(it.ID)))
	}
	for _, it := range hidden {
		hideRows.WriteString(row(
			`<a href="`+html.EscapeString(it.URL)+`">`+html.EscapeString(it.Title)+`</a>`,
			html.EscapeString(it.Type)+" · "+it.SavedAt.Format("2 Jan 15:04"),
			"show again", "/user/unhide?type="+html.EscapeString(it.Type)+"&id="+html.EscapeString(it.ID)))
	}
	for _, e := range blocked {
		blockRows.WriteString(row(
			`<a href="/@`+html.EscapeString(e.key)+`">@`+html.EscapeString(e.key)+`</a>`,
			e.at.Format("2 Jan 2006"),
			"unblock", "/user/unblock?user="+html.EscapeString(e.key)))
	}

	var b strings.Builder
	b.WriteString(`<p class="text-sm text-muted">What you have kept, hidden and blocked. Only you see this.</p>`)
	b.WriteString(card("Saved", saveRows.String(), "Nothing saved yet — use the ☆ on any item to keep it."))
	b.WriteString(card("Hidden", hideRows.String(), "Nothing hidden. Hiding an item removes it from your view and nobody else's."))
	b.WriteString(card("Blocked", blockRows.String(), "Nobody blocked."))
	b.WriteString(undoScript)
	w.Write([]byte(app.RenderHTMLForRequest("User", "What you have saved, hidden and blocked", b.String(), r)))
}

// undoScript posts the undo and reloads.
//
// The CSRF token goes with it. The pages this replaces posted without one,
// which worked because the routes did not check — a state-changing POST that
// anybody's page can make on a signed-in visitor's behalf is the definition of
// the thing the token is for, and it is one line.
const undoScript = `<style>.user-row{padding:8px 0;border-bottom:1px solid var(--border-color,#f0f0f0)}
.user-row:last-child{border-bottom:0}</style>
<script>
function muUndo(url){
  var h={},m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
  if(m)h['X-CSRF-Token']=decodeURIComponent(m[1]);
  fetch(url,{method:'POST',credentials:'same-origin',headers:h}).then(function(){location.reload()});
}
</script>`

// card is one whole section: it opens and closes its own markup.
//
// It used to open the div and leave the caller to close it, which is how a
// function comes to close three more tags than it opens and ends the layout it
// was handed. Rows are built first and passed in.
func card(title, rows, empty string) string {
	body := rows
	if body == "" {
		body = `<p class="text-muted text-sm">` + empty + `</p>`
	}
	return `<div class="card"><h3>` + title + `</h3>` + body + `</div>`
}

// row is one item and the way to undo it.
func row(label, meta, action, href string) string {
	return fmt.Sprintf(`<div class="user-row">%s<span class="text-sm text-muted"> · %s · `+
		`<a href="#" onclick="muUndo('%s');return false;">%s</a></span></div>`,
		label, meta, href, action)
}

type entry struct {
	key string
	at  time.Time
}

func sortedKeys(m map[string]time.Time) []entry {
	out := make([]entry, 0, len(m))
	for k, t := range m {
		out = append(out, entry{k, t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out
}

func split(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return key, ""
	}
	return parts[0], parts[1]
}

// UndoHandler serves the write and undo routes under /user/.
//
// POST only. These change state, and a GET that changes state is one a browser
// prefetch or a link scanner can fire on somebody's behalf.
func UndoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	who := sess.Account
	q := r.URL.Query()

	switch strings.TrimPrefix(r.URL.Path, "/user/") {
	case "save":
		app.SaveItem(who, q.Get("type"), q.Get("id"))
	case "hide":
		app.DismissItem(who, q.Get("type"), q.Get("id"))
	case "flag":
		if _, _, err := flag.Add(q.Get("type"), q.Get("id"), who); err != nil {
			app.RespondError(w, http.StatusInternalServerError, "could not report that")
			return
		}
	case "block":
		if u := q.Get("user"); u != "" && u != who {
			app.BlockUser(who, u)
		}
	case "unsave":
		app.UnsaveItem(who, q.Get("type"), q.Get("id"))
	case "unhide":
		app.Unhide(who, q.Get("type"), q.Get("id"))
	case "unblock":
		app.UnblockUser(who, q.Get("user"))
	default:
		app.NotFound(w, r, "no such action")
		return
	}
	app.RespondJSON(w, map[string]any{"status": "ok"})
}
