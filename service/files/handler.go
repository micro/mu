package files

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"strings"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves /files — the list for a signed-in person, and /files/<id> for
// one file's contents.
//
// A file's URL has to be fetchable by an ordinary HTTP client for the service
// to be worth anything: an agent that stores a report and hands someone a link
// has not helped if the link only opens inside Mu.
func Handler(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/files"), "/")
	if id == "" {
		listPage(w, r)
		return
	}
	serveFile(w, r, id)
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

	stored := List(sess.Account)

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Files</h3>`)
	b.WriteString(`<p class="text-sm text-muted">Anything you or your agent has stored. Using ` +
		human(UsedBytes(sess.Account)) + ` of ` + human(MaxOwnerBytes) + `.</p>`)
	b.WriteString(`</div>`)

	if len(stored) == 0 {
		b.WriteString(`<div class="card"><p class="text-sm text-muted">Nothing stored yet. ` +
			`An agent connected over <a href="/mcp">MCP</a> can put a file here with <code>files_put</code>.</p></div>`)
	} else {
		b.WriteString(`<div class="card"><table class="data-table">`)
		b.WriteString(`<tr><th>Name</th><th>Size</th><th>Visibility</th><th>Stored</th></tr>`)
		for _, f := range stored {
			visibility := "Private"
			if f.Public {
				visibility = "Public"
			}
			fmt.Fprintf(&b, `<tr><td><a href="%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(f.URL), html.EscapeString(f.Name),
				human(f.Size), visibility, f.Created.Format("2 Jan 15:04"))
		}
		b.WriteString(`</table></div>`)
	}

	w.Write([]byte(app.RenderHTMLForRequest("Files", "Your stored files", b.String(), r)))
}

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
