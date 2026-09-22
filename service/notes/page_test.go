package notes

import (
	"mu/internal/auth"
	store "mu/internal/notes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoteReadFormattingAndOwnership(t *testing.T) {
	owner := "note_page_owner"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	store.Add(owner, "Formatted", "First line\nSecond line\n\n## Heading\n\n- First\n- Second\n\n<script>alert(1)</script>")
	t.Cleanup(func() { store.Delete(owner, "Formatted") })
	id := store.All(owner)[0].ID
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/notes?id="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w := httptest.NewRecorder()
	Handler(w, r)
	body := w.Body.String()
	if !strings.Contains(body, "First line<br") || !strings.Contains(body, "<li>First<br>") || !strings.Contains(body, "<article") || strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatal("note formatting is missing or unsafe")
	}
	other := "note_page_other"
	if err := auth.Create(&auth.Account{ID: other}); err != nil {
		t.Fatal(err)
	}
	otherSession, err := auth.CreateSession(other)
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("GET", "/notes?id="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: otherSession.Token})
	w = httptest.NewRecorder()
	Handler(w, r)
	if strings.Contains(w.Body.String(), "Formatted") || strings.Contains(w.Body.String(), "Second") {
		t.Fatal("another account can read the note")
	}
}
