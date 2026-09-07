package files

import (
	store "mu/internal/files"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestEditorProtectsOwnershipAndStaleWrites(t *testing.T) {
	c := session(t, "editorowner")
	f, err := store.Replace("editorowner", "", "draft.txt", "text/plain", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	other := session(t, "editorother")
	r := httptest.NewRequest("GET", "/files/"+f.ID+"/edit", nil)
	r.AddCookie(other)
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 404 {
		t.Fatalf("other owner: %d", w.Code)
	}
	w = postAction(t, c, "/files/"+f.ID+"/edit", url.Values{"checksum": {"stale"}, "content": {"replacement"}})
	_, raw, err := Get("editorowner", f.ID)
	if err != nil || string(raw) != "original" {
		t.Fatalf("stale draft overwrote file: %s %v", raw, err)
	}
	w = postAction(t, c, "/files/"+f.ID+"/edit", url.Values{"checksum": {f.Checksum}, "content": {"new\ntext"}})
	if w.Code != 303 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	_, raw, err = Get("editorowner", f.ID)
	if err != nil || string(raw) != "new\ntext" {
		t.Fatalf("save failed: %s %v", raw, err)
	}
}
