package files

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves /files — the list for a signed-in person, /files/<id> for one
// file's contents, and the actions on them.
//
// A file's URL has to be fetchable by an ordinary HTTP client for the service
// to be worth anything: an agent that stores a report and hands someone a link
// has not helped if the link only opens inside Mu.
//
// The page began read-only, which made it a window onto storage rather than
// storage: an agent could put a file there and a person could look at it, but
// not add one, remove one or share one. Everything the tools can do, the page
// now does — the same functions underneath, so the two cannot drift.
func Handler(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/files"), "/")

	// Uploads post to /files itself; per-file actions to /files/<id>/<action>.
	if r.Method == http.MethodPost {
		id, action, _ := strings.Cut(rest, "/")
		handleAction(w, r, id, action)
		return
	}

	if rest == "" {
		listPage(w, r)
		return
	}
	serveFile(w, r, rest)
}

// handleAction performs an upload, delete or share and returns to the page.
//
// Plain form posts rather than fetch: a file list should work without
// JavaScript, and the browser's own file picker is better than anything worth
// building here.
func handleAction(w http.ResponseWriter, r *http.Request, id, action string) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	// Cap the body before anything reads it. It cannot wait for upload(): the
	// CSRF check reads _csrf, which parses the multipart form itself, so the
	// first read of the body happens on the line below. A little headroom over
	// MaxBytes covers the form's own overhead so a file at exactly the limit
	// still lands.
	r.Body = http.MaxBytesReader(w, r.Body, MaxBytes+(1<<20))

	if !auth.ValidCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}

	var actErr error
	switch {
	case id == "":
		actErr = upload(r, sess.Account)
	case action == "delete":
		actErr = Delete(sess.Account, id)
	case action == "share":
		_, actErr = Share(sess.Account, id, r.FormValue("public") == "1")
	default:
		app.NotFound(w, r, "Unknown action")
		return
	}

	// Errors a person can act on — too big, out of space, wrong sort of file —
	// belong on the page they came from, not on an error screen they have to
	// navigate back from.
	dest := "/files"
	if actErr != nil {
		dest += "?error=" + neturl.QueryEscape(actErr.Error())
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// upload stores a file chosen in the browser.
func upload(r *http.Request, owner string) error {
	// Usually a no-op — the CSRF check has already parsed the form — but it is
	// what surfaces the error when the body ran past handleAction's cap, and it
	// keeps upload() correct if it is ever called on its own.
	if err := r.ParseMultipartForm(MaxBytes + (1 << 20)); err != nil {
		if strings.Contains(err.Error(), "too large") {
			return fmt.Errorf("that file is larger than the %s limit", human(MaxBytes))
		}
		return fmt.Errorf("could not read the upload: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return fmt.Errorf("choose a file to upload")
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return err
	}
	// Put checks the limit too; catching it here means the message names the
	// file rather than describing bytes.
	if len(raw) > MaxBytes {
		return fmt.Errorf("%s is larger than the %s limit", header.Filename, human(MaxBytes))
	}

	_, err = Put(owner, header.Filename, header.Header.Get("Content-Type"), string(raw), "")
	return err
}

// serveFile writes a file's bytes. Owner or public only — meta() decides, and a
// caller who may not read it gets 404 rather than 403, so the URL does not
// confirm that a file exists.
func serveFile(w http.ResponseWriter, r *http.Request, id string) {
	who := ""
	if sess, _ := auth.TrySession(r); sess != nil {
		who = sess.Account
	}

	f, raw, err := Get(who, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", f.Type)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(raw)))
	// Nothing here is rendered as part of Mu, and a stored file is
	// caller-supplied bytes: served inline, an HTML or SVG upload would run
	// script on this origin. Download it instead.
	w.Header().Set("Content-Disposition", "attachment; filename="+quoteName(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Public {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.Write(raw)
}

func listPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	auth.SetCSRFCookie(w, r)
	stored := List(sess.Account)
	csrf := auth.CSRFToken(r)

	var b strings.Builder
	// No heading here: the page is already titled Files by the shell, and a
	// card that repeats the page title just costs a phone a line of screen.
	b.WriteString(`<div class="card">`)
	b.WriteString(`<p class="text-sm text-muted">Anything you or your agent has stored. Using ` +
		human(UsedBytes(sess.Account)) + ` of ` + human(MaxOwnerBytes) + `.</p>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<p class="text-error">` + html.EscapeString(msg) + `</p>`)
	}

	// Upload. A plain multipart form: the browser's own file picker beats
	// anything worth building here, and the page keeps working without
	// JavaScript.
	fmt.Fprintf(&b, `<form method="POST" action="/files" enctype="multipart/form-data" class="file-upload">
  <input type="hidden" name="_csrf" value="%s">
  <input type="file" name="file" required>
  <button type="submit">Upload</button>
</form>`, html.EscapeString(csrf))
	b.WriteString(`<p class="text-sm text-muted">Up to ` + human(MaxBytes) + ` a file.</p>`)
	b.WriteString(`</div>`)

	if len(stored) == 0 {
		b.WriteString(`<div class="card"><p class="text-sm text-muted">Nothing stored yet. ` +
			`Upload something above, or an agent connected over <a href="/mcp">MCP</a> can put a file here with <code>files_put</code>.</p></div>`)
	} else {
		b.WriteString(`<div class="card"><table class="data-table files-table">`)
		b.WriteString(`<thead><tr><th>Name</th><th>Size</th><th>Visibility</th><th>Stored</th><th></th></tr></thead><tbody>`)
		for _, f := range stored {
			visibility, shareTo, shareLabel := "Private", "1", "Share"
			if f.Public {
				visibility, shareTo, shareLabel = "Public", "0", "Make private"
			}
			// The cells are classed rather than positional because a phone does
			// not render this as a table: the header goes, the row becomes a
			// block, and size/visibility/date collapse onto one line under the
			// name. Five columns at 375px would either overflow the screen or
			// squeeze the name to nothing.
			fmt.Fprintf(&b, `<tr><td class="file-name"><a href="%s">%s</a></td>`+
				`<td class="file-meta">%s</td><td class="file-meta">%s</td><td class="file-meta">%s</td>`+
				`<td class="file-actions">`,
				html.EscapeString(f.URL), html.EscapeString(f.Name),
				human(f.Size), visibility, f.Created.Format("2 Jan 15:04"))

			fmt.Fprintf(&b, `<form method="POST" action="/files/%s/share">
  <input type="hidden" name="_csrf" value="%s"><input type="hidden" name="public" value="%s">
  <button type="submit" class="link-button">%s</button>
</form>`, html.EscapeString(f.ID), html.EscapeString(csrf), shareTo, shareLabel)

			// Deleting a file destroys its contents, so it asks first. The
			// confirm is progressive: without JavaScript the form still posts,
			// which is the right failure — the button is labelled Delete.
			fmt.Fprintf(&b, `<form method="POST" action="/files/%s/delete" onsubmit="return confirm('Delete %s?')">
  <input type="hidden" name="_csrf" value="%s">
  <button type="submit" class="link-button danger">Delete</button>
</form>`, html.EscapeString(f.ID), html.EscapeString(strings.ReplaceAll(f.Name, "'", "\\'")), html.EscapeString(csrf))

			b.WriteString(`</td></tr>`)
		}
		b.WriteString(`</tbody></table></div>`)
	}

	b.WriteString(filesPageCSS)
	app.Respond(w, r, app.Response{Title: "Files", Description: "Your stored files", HTML: b.String()})
}

// filesPageCSS styles the page, and on a narrow screen unmakes the table.
//
// Phones are most of the traffic and a five-column table is not a phone
// layout: it either scrolls sideways or crushes the file name, which is the one
// column that matters. Below 600px the header row goes, each row becomes a
// block — name, then size · visibility · date on one muted line, then the
// actions — and the buttons grow to something a thumb can hit.
const filesPageCSS = `<style>
.file-upload{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin:10px 0 4px}
.file-upload input[type=file]{font-size:14px;max-width:100%;flex:1 1 auto;min-width:0}
/* The card already provides the spacing data-table adds for a bare page. */
.files-table{margin-bottom:0}
.files-table .file-name{word-break:break-word}
.file-actions{white-space:nowrap}
.file-actions form{display:inline}

@media only screen and (max-width:600px){
  .file-upload{flex-direction:column;align-items:stretch}
  .file-upload button{width:100%}

  .files-table,.files-table tbody,.files-table tr,.files-table td{display:block;width:auto}
  .files-table thead{display:none}
  .files-table tr{padding:12px 0;border-bottom:1px solid var(--divider)}
  .files-table tbody tr:last-child{border-bottom:none}
  .files-table td{padding:0;border:none;text-align:left}
  .files-table .file-name{font-weight:var(--font-weight-medium);margin-bottom:2px}
  /* The three facts read as one sentence rather than three stacked lines. */
  .files-table .file-meta{display:inline;color:var(--text-muted);font-size:13px}
  .files-table .file-meta + .file-meta::before{content:" · "}
  /* td.file-actions, not .file-actions: .data-table td:last-child aligns
     right, and on a block row the buttons belong under the file. */
  .files-table td.file-actions{margin-top:6px;text-align:left}
  .files-table .file-actions .link-button{padding:6px 14px 6px 0;font-size:14px}
  /* Striping reads as noise once the rows are blocks. */
  .files-table tbody tr:nth-child(odd){background:none}
}
</style>`

// quoteName makes a file name safe to put in a header, where a quote or a
// newline would otherwise let a caller write headers of their own.
func quoteName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' || r == 127 {
			return '_'
		}
		return r
	}, name)
	return `"` + clean + `"`
}

// encodeForWire returns a file's contents for a JSON response: text as text,
// anything else base64. A model reading a CSV should get the CSV, not base64 it
// has to decode in its head.
func encodeForWire(contentType string, raw []byte) (string, bool) {
	if isText(contentType) && utf8.Valid(raw) {
		return string(raw), false
	}
	return base64.StdEncoding.EncodeToString(raw), true
}

func isText(contentType string) bool {
	t := strings.ToLower(contentType)
	if strings.HasPrefix(t, "text/") {
		return true
	}
	for _, suffix := range []string{"json", "xml", "yaml", "csv", "markdown", "javascript", "x-sh"} {
		if strings.Contains(t, suffix) {
			return true
		}
	}
	return false
}
