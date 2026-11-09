package images

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/blob"
	"mu/internal/userdb"
)

func TestImageDeletionFromPage(t *testing.T) {
	const owner = "image-delete-owner"
	const other = "image-delete-other"
	for _, id := range []string{owner, other} {
		auth.SetAccountForTest(&auth.Account{ID: id})
		defer auth.RemoveAccountForTest(id)
	}
	session, _ := auth.CreateSession(owner)
	foreignSession, _ := auth.CreateSession(other)
	record, err := userdb.Create(ns, owner, collection, map[string]interface{}{"prompt": "My picture", "url": "https://example.org/image.png"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer userdb.Delete(ns, owner, collection, record.ID)
	key := genPrefix(record.ID) + ".png"
	if err := blob.Put(key, []byte("image bytes"), "image/png"); err != nil {
		t.Fatal(err)
	}
	defer blob.Delete(key)
	record.Data["file"] = key
	if _, err := userdb.Update(ns, owner, collection, record.ID, record.Data, true); err != nil {
		t.Fatal(err)
	}
	request := func(method, token string, csrf bool) *http.Request {
		r := httptest.NewRequest(method, "/images", strings.NewReader(fmt.Sprintf(`{"action":"delete","id":%q}`, record.ID)))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	for _, tc := range []struct {
		token  string
		csrf   bool
		status int
	}{{"", false, 401}, {session.Token, false, 403}, {foreignSession.Token, true, 404}} {
		w := httptest.NewRecorder()
		Handler(w, request("POST", tc.token, tc.csrf))
		if w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
		if _, err := userdb.Get(ns, owner, collection, record.ID); err != nil {
			t.Fatal("unauthorised delete removed image")
		}
	}
	for _, tc := range []struct {
		token    string
		controls bool
	}{{session.Token, true}, {foreignSession.Token, false}, {"", false}} {
		w := httptest.NewRecorder()
		Handler(w, request("GET", tc.token, false))
		if strings.Contains(w.Body.String(), "data-image-delete") != tc.controls {
			t.Fatal("incorrect owner controls")
		}
	}
	w := httptest.NewRecorder()
	Handler(w, request("POST", session.Token, true))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted":true`) {
		t.Fatalf("delete failed: %d %s", w.Code, w.Body.String())
	}
	if _, err := userdb.Get(ns, owner, collection, record.ID); err == nil {
		t.Fatal("record remains")
	}
	if _, err := blob.Get(key); err == nil {
		t.Fatal("image bytes remain")
	}
	w = httptest.NewRecorder()
	serveGenerated(owner, w, httptest.NewRequest("GET", "/images/file/"+record.ID, nil))
	if w.Code != 404 {
		t.Fatal("deleted image still served")
	}
}
