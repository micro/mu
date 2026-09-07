package docs

import (
	"html"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
)

// Import is a file-selection page. Reading a file opens an unsaved draft;
// only the editor's Save action writes a document.
func importPage(r *http.Request, message string) string {
	return `<div class="collection-head"><a class="doc-back" href="/docs">← Documents</a></div>` +
		`<form method="POST" action="/docs?import=1" enctype="multipart/form-data" class="record-editor">` +
		app.CSRFField(auth.CSRFToken(r)) +
		`<label for="doc-file">Choose a Markdown or plain-text file</label>` +
		`<input id="doc-file" type="file" name="file" accept=".txt,.md,.markdown,text/plain,text/markdown" required>` +
		`<p class="text-sm text-muted">UTF-8 text, up to 200 KB. Review the draft before saving.</p>` +
		`<p role="status">` + html.EscapeString(message) + `</p>` +
		`<div class="record-actions"><button type="submit">Open draft</button></div></form>`
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	fail := func(message string) {
		app.Respond(w, r, app.Response{Title: "Import document", HTML: importPage(r, message) + pageCSS})
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody+64*1024)
	if err := r.ParseMultipartForm(maxBody + 64*1024); err != nil {
		fail("Choose a text file up to 200 KB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		fail("Choose a file to import.")
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".txt" && ext != ".md" && ext != ".markdown" {
		fail("Choose a Markdown or plain-text file.")
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBody+1))
	if err != nil || len(content) > maxBody {
		fail("Choose a text file up to 200 KB.")
		return
	}
	if !utf8.Valid(content) || strings.ContainsRune(string(content), '\x00') {
		fail("Use UTF-8 plain text or Markdown.")
		return
	}
	title := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	draft := &Doc{Title: title, Content: strings.TrimPrefix(string(content), "\ufeff")}
	app.Respond(w, r, app.Response{Title: "Docs", HTML: editor(r, draft) + pageCSS})
}
