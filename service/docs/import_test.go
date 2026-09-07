package docs

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImportOpensDraftWithoutSaving(t *testing.T) {
	const owner = "import_draft"
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "A plan.md")
	if err != nil {
		t.Fatal(err)
	}
	file.Write([]byte("# A plan\n\nRead before saving."))
	form.Close()
	r := signedIn(t, owner, "POST", "/docs?import=1", nil)
	r.Body = io.NopCloser(&body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Read before saving.") || !strings.Contains(w.Body.String(), `value="A plan"`) {
		t.Fatal("file did not open as an editable draft")
	}
	if len(shown(t, owner)) != 0 {
		t.Fatal("import saved without review")
	}
	if strings.Contains(w.Body.String(), `type="file"`) {
		t.Fatal("file picker leaked into editor")
	}
}

func TestImportRejectsInvalidFiles(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"bad.pdf", "text", "Markdown or plain-text"},
		{"bad.txt", string([]byte{255}), "UTF-8"},
		{"bad.md", "text\x00binary", "UTF-8"},
		{"large.txt", strings.Repeat("a", maxBody+1), "200 KB"},
	} {
		t.Run(tc.name+tc.want, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			file, err := form.CreateFormFile("file", tc.name)
			if err != nil {
				t.Fatal(err)
			}
			file.Write([]byte(tc.content))
			form.Close()
			r := signedIn(t, "import_invalid", "POST", "/docs?import=1", nil)
			r.Body = io.NopCloser(&body)
			r.Header.Set("Content-Type", form.FormDataContentType())
			w := httptest.NewRecorder()
			Handler(w, r)
			if !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), `id="doc-body"`) {
				t.Fatal("invalid file opened as a draft")
			}
		})
	}
}
