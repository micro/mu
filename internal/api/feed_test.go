package api

import (
	"mu/internal/service"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFeedIsCachedPublicAndDoesNotFetchDuringNavigation(t *testing.T) {
	called := 0
	for _, s := range []service.Spec{
		{Name: "feedpublic", Handler: new(CardProbe), Page: "/feedpublic", Card: service.Glance(func() string { called++; return "Public fixture" })},
		{Name: "feedprivate", Handler: new(CardProbe), Page: "/feedprivate", Scoped: true, Card: service.Glance(func() string { return "Private fixture" })},
		{Name: "feedpersonal", Handler: new(CardProbe), Page: "/feedpersonal", Card: service.Personal(func(service.Viewer) string { return "Personal fixture" })},
	} {
		if err := service.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	serviceFeed(w, httptest.NewRequest("GET", "/services?view=feed", nil))
	if w.Code != 200 || called != 0 {
		t.Fatal("feed navigation fetched upstream content")
	}
	refreshFeed()
	feedCache.Lock()
	body := feedCache.html
	at := feedCache.at
	feedCache.Unlock()
	if !strings.Contains(body, "Public fixture") || strings.Contains(body, "Private fixture") || strings.Contains(body, "Personal fixture") || time.Since(at) > time.Minute {
		t.Fatal("shared feed included private content or did not cache")
	}
	r := httptest.NewRequest("GET", "/services?view=feed", nil)
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	serviceFeed(w, r)
	if called != 1 || !strings.Contains(w.Body.String(), "Public fixture") {
		t.Fatal("cached content refetched")
	}
}
