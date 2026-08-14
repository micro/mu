package test

// An app is text a stranger wrote.
//
// Apps are public, forkable and one click to open, and the SDK dispatches with
// the viewer bound as the caller. The service allowlist keeps them out of the
// scoped services — mail, files, notes, contacts, tasks, the wallet — and that
// is per service, which turned out to be one level too coarse.
//
// blog is deliberately unscoped, because reading a blog is public. blog.Delete
// is marked Destructive, because deleting one is not. So an app could call
// mu.blog.delete and the viewer's posts were gone, with nobody tricked into
// anything: opening the app was the whole attack.
//
// Destructive already names this exactly — an irreversible effect nobody asked
// for. The agent honours it because the agent reads text strangers wrote. An
// app *is* text a stranger wrote, which is the same argument with one step
// removed, and the app SDK was not asking.
//
// This lives in test/ rather than beside the handler because it needs the real
// registry: service/apps cannot import service/blog, and without the services
// registered the call is refused as "unknown service" before it reaches the
// check being tested. The first version of this test sat in service/apps and
// passed for exactly that reason — 404 is not 403, and the assertion only knew
// about 403.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/service/apps"
)

func callSDKService(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/apps/demo/sdk/service", strings.NewReader(body))
	rr := httptest.NewRecorder()
	apps.Handler(rr, r)
	return rr
}

func TestAnAppCannotDeleteTheViewersPosts(t *testing.T) {
	registerAll(t)

	rr := callSDKService(t, `{"service":"blog","method":"Delete","args":{"title":"anything"}}`)

	if rr.Code == http.StatusNotFound {
		t.Fatalf("the blog service is not registered in this binary, so this test is "+
			"not testing anything: %s", rr.Body.String())
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("blog.Delete from an app answered %d, want 403 — an app that can "+
			"delete the viewer's posts is the sandbox not holding: %s",
			rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "not by an app") {
		t.Errorf("the refusal does not say why: %s", rr.Body.String())
	}
}

// Whatever spelling arrives. The SDK builds accessors in lower case.
func TestTheAppRefusalIsNotCaseSensitive(t *testing.T) {
	registerAll(t)

	for _, m := range []string{"delete", "Delete", "DELETE"} {
		rr := callSDKService(t, `{"service":"blog","method":"`+m+`","args":{}}`)
		if rr.Code != http.StatusForbidden {
			t.Errorf("method %q answered %d, want 403: %s", m, rr.Code, rr.Body.String())
		}
	}
}

// And reading is untouched. A guard that refuses everything is a guard somebody
// deletes.
func TestAnAppCanStillReadAPublicService(t *testing.T) {
	registerAll(t)

	rr := callSDKService(t, `{"service":"blog","method":"List","args":{}}`)

	if rr.Code == http.StatusForbidden || rr.Code == http.StatusNotFound {
		t.Errorf("blog.List answered %d — reading a public blog is what apps are for: %s",
			rr.Code, rr.Body.String())
	}
}

// The catalogue does not advertise what will always be refused.
func TestTheAppCatalogueHidesDestructiveMethods(t *testing.T) {
	registerAll(t)

	r := httptest.NewRequest(http.MethodGet, "/apps/demo/sdk/services", nil)
	rr := httptest.NewRecorder()
	apps.Handler(rr, r)

	if strings.Contains(strings.ToLower(rr.Body.String()), `"delete"`) {
		t.Errorf("the catalogue offers a delete method to apps: %s", rr.Body.String())
	}
}
