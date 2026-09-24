package home

import (
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/apps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestSavedAppContinuesOwnedConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "saved-app-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: "App Owner", Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(apps.Spec); err != nil {
		t.Fatal(err)
	}
	origin := thread.Open(owner, thread.WebClient, "original-app-conversation")
	a, err := apps.CreateApp(owner, "A useful packing checklist for a long journey", "saved-app-test", "A reusable checklist with your own saved entries.", "", "<p>Checklist</p>", "", 0, false, origin.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, slug string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/home/apps", strings.NewReader(url.Values{"slug": {slug}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		AppsHandler(w, r)
		return w
	}
	if got := request("POST", a.Slug, false).Code; got != 403 {
		t.Fatalf("csrf: %d", got)
	}
	if got := request("POST", "not-owned", true).Code; got != 404 {
		t.Fatalf("foreign app: %d", got)
	}
	for i := 0; i < 2; i++ {
		w := request("POST", a.Slug, true)
		if w.Code != 303 || w.Header().Get("Location") != "/agent/micro?session="+origin.ID {
			t.Fatalf("continuation: %d %s", w.Code, w.Header().Get("Location"))
		}
	}
	if len(thread.Messages(owner, origin.ID, 0)) != 1 {
		t.Fatal("duplicate context")
	}
	w := request("GET", "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Continue with Micro") {
		t.Fatal("collection missing")
	}
	if dir := os.Getenv("MU_UI_FIXTURES"); dir != "" {
		if err := os.WriteFile(dir+"/apps.html", w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A legacy app without provenance gets one stable conversation, never a fresh
	// conversation on every return.
	legacy, err := apps.CreateApp(owner, "Legacy tool", "legacy-app-test", "", "", "<p>Legacy</p>", "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	first := request("POST", legacy.Slug, true)
	second := request("POST", legacy.Slug, true)
	if first.Code != 303 || first.Header().Get("Location") != second.Header().Get("Location") {
		t.Fatal("legacy conversation not stable")
	}
}
