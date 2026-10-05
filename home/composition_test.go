package home

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/notes"
	"mu/internal/service"
	"mu/service/apps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHomeViewsKeepPersonalCardsOutOfFeed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"home-public-view-test", "home-personal-view-test"} {
		card := service.Glance(func() string { return "public test update" })
		if strings.Contains(name, "personal") {
			card = service.Personal(func(v service.Viewer) string { return "private test context" })
		}
		if err := service.Register(service.Spec{Name: name, Handler: &apps.Server{}, Page: "/" + name, Card: card}); err != nil {
			t.Fatal(err)
		}
	}
	acc := &auth.Account{ID: "home-view-owner", Approved: true, Pinned: []string{"home-public-view-test", "home-personal-view-test"}}
	auth.SetAccountForTest(acc)
	defer auth.RemoveAccountForTest(acc.ID)
	sess, _ := auth.CreateSession(acc.ID)
	overviewCache.Lock()
	overviewCache.values[acc.ID] = overviewSnapshot{key: overviewKey(acc), at: time.Now(), cards: map[string]string{"home-public-view-test": "public test update", "home-personal-view-test": "private test context"}}
	overviewCache.Unlock()
	for _, feed := range []bool{false, true} {
		for _, fragment := range []bool{false, true} {
			path := "/home"
			if feed {
				path += "?view=feed"
			}
			r := httptest.NewRequest("GET", path, nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
			if fragment {
				r.Header.Set("Accept", "application/json")
			}
			w := httptest.NewRecorder()
			Handler(w, r)
			body := w.Body.String()
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			if fragment {
				var data struct {
					HTML string `json:"html"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				body = data.HTML
			}
			if strings.Contains(body, "private test context") == feed || strings.Contains(body, "public test update") != feed {
				t.Fatalf("mixed views: feed=%v fragment=%v", feed, fragment)
			}
			if feed && strings.Contains(body, `data-path="/agent/micro"`) {
				t.Fatal("feed contains composer")
			}
		}
	}
}

func TestLibraryRequiresAccountAndKeepsNotesPrivate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "library-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	notes.Add(owner, "My library note", "A useful reminder")
	notes.Add("library-other", "Foreign secret", "Private to someone else")
	sess, _ := auth.CreateSession(owner)
	r := httptest.NewRequest("GET", "/home/library?type=note", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	LibraryHandler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "My library note") || strings.Contains(w.Body.String(), "Foreign secret") {
		t.Fatal("incorrect library ownership")
	}
	w = httptest.NewRecorder()
	LibraryHandler(w, httptest.NewRequest("GET", "/home/library", nil))
	if w.Code != 303 {
		t.Fatal("library requires login")
	}
	r = httptest.NewRequest("POST", "/home/library", strings.NewReader("action=file&name=test"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w = httptest.NewRecorder()
	LibraryHandler(w, r)
	if w.Code != 403 {
		t.Fatal("file preview requires CSRF")
	}
}
