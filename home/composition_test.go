package home

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/notes"
	"mu/internal/service"
	"mu/service/apps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHomeShowsPinsAndConversationsAndFeedIncludesPrayer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"images", "prayer", "apps"} {
		card := service.Glance(func() string { return "public test update" })
		if name == "prayer" {
			card = service.Personal(func(v service.Viewer) string { return "private test context" })
		}
		if err := service.Register(service.Spec{Name: name, Handler: &apps.Server{}, Page: "/" + name, Card: card}); err != nil {
			t.Fatal(err)
		}
	}
	acc := &auth.Account{ID: "home-view-owner", Approved: true, Pinned: []string{"images", "prayer", "apps"}}
	auth.SetAccountForTest(acc)
	defer auth.RemoveAccountForTest(acc.ID)
	sess, _ := auth.CreateSession(acc.ID)
	overviewCache.Lock()
	overviewCache.values[acc.ID] = overviewSnapshot{key: overviewKey(acc), at: time.Now(), cards: map[string]string{"images": "public test update", "prayer": "private test context"}}
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
			if !strings.Contains(body, `id="home-groups"`) || !strings.Contains(body, `href="/groups?new=1"`) {
				t.Fatal("group entry missing", body)
			}
			if strings.Contains(body, "private test context") != feed || strings.Contains(body, "public test update") != feed {
				t.Fatalf("mixed views: feed=%v fragment=%v", feed, fragment)
			}
			if !feed && (!strings.Contains(body, `id="home-conversations"`) || !strings.Contains(body, `href="/agent?view=chats"`) || !strings.Contains(body, `aria-label="Pinned apps"`) || !strings.Contains(body, `href="/apps"`)) {
				t.Fatal("Chats or pinned apps missing")
			}
			if !feed && !strings.Contains(body, `id="home-scheduled-card"`) {
				t.Fatal("Scheduled card missing")
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

func TestHomeGroupsAreAccountScoped(t *testing.T) {
	for _, owner := range []string{"home-group-owner", "home-group-other"} {
		auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
		defer auth.RemoveAccountForTest(owner)
	}
	own, err := group.Create("home-group-owner", "Family <home>", false)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Change(own.ID, "home-group-owner", "delete", "", "")
	other, err := group.Create("home-group-other", "Other secret group", false)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Change(other.ID, "home-group-other", "delete", "", "")
	body := groupsCard("home-group-owner")
	if !strings.Contains(body, "/groups?new=1") || !strings.Contains(body, "/groups?id="+own.ID) || !strings.Contains(body, "Family &lt;home&gt;") || strings.Contains(body, other.ID) || strings.Contains(body, other.Name) {
		t.Fatal(body)
	}
}
