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
	err := (Server{}).Create(service.WithAccount(context.Background(), owner), &CreateRequest{Name: "Private widget", Slug: "private-widget-test", HTML: "<p>private widget content</p>", Private: true}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Item == nil {
		t.Fatal("no widget result")
	}
	a := GetApp(out.Item.ID)
	defer func() { mutex.Lock(); delete(apps, a.Slug); mutex.Unlock() }()
	if a.Public {
		t.Fatal("personal widget is public")
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
