package apps

import (
	"context"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersonalWidgetPrivacyAndPrice(t *testing.T) {
	const owner = "widget_owner_test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: "Widget owner"})
	defer auth.RemoveAccountForTest(owner)
	var out CreateResponse
	err := (Server{}).Create(service.WithAccount(context.Background(), owner), &CreateRequest{Name: "Private widget", Slug: "private-widget-test", HTML: "<p>private widget content</p>"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Item == nil {
		t.Fatal("no widget result")
	}
	a := GetApp(out.Item.ID)
	defer func() { mutex.Lock(); delete(apps, a.Slug); mutex.Unlock() }()
	if Spec.Endpoints["Read"].Needs != service.Caller {
		t.Fatal("Read must bind the authenticated caller")
	}
	var read AppReadResponse
	if err := (Server{}).Read(service.WithAccount(context.Background(), owner), &AppReadRequest{Slug: a.Slug}, &read); err != nil || read.HTML == "" {
		t.Fatal("owner cannot inspect private app source", err)
	}
	if a.Public {
		t.Fatal("personal widget is public")
	}
	var found AppSearchResponse
	if err := (Server{}).Search(service.WithAccount(context.Background(), owner), &AppSearchRequest{Query: "Private widget"}, &found); err != nil || !strings.Contains(found.Text, a.Slug) {
		t.Fatal("owner cannot find private app")
	}
	var hidden AppSearchResponse
	_ = (Server{}).Search(service.WithAccount(context.Background(), "another-owner"), &AppSearchRequest{Query: "Private widget"}, &hidden)
	if strings.Contains(hidden.Text, a.Slug) {
		t.Fatal("search leaked private app")
	}
	var embed EmbedResponse
	if err := (Server{}).Embed(service.WithAccount(context.Background(), owner), &EmbedRequest{Slug: a.Slug}, &embed); err != nil || embed.Item == nil || embed.Item.ID != a.Slug {
		t.Fatal("no typed private embed")
	}
	if GetApp(a.Slug).Public {
		t.Fatal("embedding published the app")
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"?widget=1", "?raw=1", ""} {
		r := httptest.NewRequest("GET", "/apps/"+a.Slug+suffix, nil)
		w := httptest.NewRecorder()
		Handler(w, r)
		if w.Code != 404 || strings.Contains(w.Body.String(), "private widget content") {
			t.Fatalf("private app leaked: %s %d", suffix, w.Code)
		}
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w = httptest.NewRecorder()
		Handler(w, r)
		if w.Code != 200 {
			t.Fatalf("owner blocked: %s %d", suffix, w.Code)
		}
		if suffix == "?widget=1" && strings.Contains(w.Body.String(), "app-return") {
			t.Fatal("widget includes app directory navigation")
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("app response may be shared cached")
		}
	}
	for _, suffix := range []string{"", "/versions", "/icon.svg"} {
		r := httptest.NewRequest("GET", "/apps/"+a.Slug+suffix, nil)
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		Handler(w, r)
		if w.Code != 404 {
			t.Fatalf("private JSON route exposed: %s %d", suffix, w.Code)
		}
	}
	auth.SetAccountForTest(&auth.Account{ID: "widget_other_test"})
	defer auth.RemoveAccountForTest("widget_other_test")
	other, err := auth.CreateSession("widget_other_test")
	if err != nil {
		t.Fatal(err)
	}
	rOther := httptest.NewRequest("GET", "/apps/"+a.Slug, nil)
	rOther.Header.Set("Accept", "application/json")
	rOther.AddCookie(&http.Cookie{Name: "session", Value: other.Token})
	denied := httptest.NewRecorder()
	Handler(denied, rOther)
	if denied.Code != 404 {
		t.Fatal("another account read private widget JSON")
	}
	if _, err := ForkApp(a.Slug, "stolen-widget", "widget_other_test", "Other"); err == nil {
		t.Fatal("private app forked by another account")
	}
	if TestApp(a.Slug, "widget_other_test") != nil {
		t.Fatal("private app tested by another account")
	}
	ownFork, err := ForkApp(a.Slug, "private-widget-copy", owner, "Owner")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { mutex.Lock(); delete(apps, ownFork.Slug); mutex.Unlock() }()
	if ownFork.Public {
		t.Fatal("private app fork became public")
	}
	a.Price = 5
	r := httptest.NewRequest("GET", "/apps/"+a.Slug+"?widget=1", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 402 || appResult(a) != nil {
		t.Fatal("paid app automatically embedded")
	}
}
