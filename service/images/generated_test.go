package images

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mu/internal/userdb"
)

// Storage is rooted at $HOME/.mu, so each run gets its own HOME rather than
// writing into a developer's real one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-images-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// imageServer serves one PNG and counts how many times it was asked for.
func imageServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG\r\n\x1a\nthe bytes"))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// An image someone paid for must be served by this instance, not borrowed from
// the provider on every view. A cross-origin embed is refusable by any content
// blocker, hotlink rule or resource policy, and the provider URL expires — both
// of which render as a broken image on a page that has just charged 15 credits.
func TestGeneratedImagesAreServedFromHere(t *testing.T) {
	srv, hits := imageServer(t)

	rec, err := userdb.Create(ns, "alice", collection, map[string]interface{}{
		"prompt": "a cat astronaut",
		"url":    srv.URL + "/generated.png",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer userdb.Delete(ns, "alice", collection, rec.ID)

	get := func(caller string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		serveGenerated(caller, w, httptest.NewRequest("GET", DisplayURL(rec.ID), nil))
		return w
	}

	w := get("alice")
	if w.Code != http.StatusOK {
		t.Fatalf("serving a generated image returned %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "the bytes") {
		t.Errorf("wrong body: %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type %q", ct)
	}

	// The first view takes our own copy, so the provider is asked once and
	// never again — that is what makes the image outlive the link.
	if *hits != 1 {
		t.Fatalf("provider was fetched %d times on first view, want 1", *hits)
	}
	if w := get("alice"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "the bytes") {
		t.Fatalf("second view failed: %d", w.Code)
	}
	if *hits != 1 {
		t.Errorf("provider was fetched again (%d times); the copy is not being used", *hits)
	}

	stored, err := userdb.Get(ns, "alice", collection, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if file, _ := stored.Data["file"].(string); file == "" {
		t.Error("the record was not updated with the stored key, so every view re-fetches")
	}
}

// A generated image is private until its owner shares it.
func TestPrivateGeneratedImagesAreNotReadableByOthers(t *testing.T) {
	srv, _ := imageServer(t)

	rec, err := userdb.Create(ns, "alice", collection, map[string]interface{}{
		"prompt": "private", "url": srv.URL + "/p.png",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer userdb.Delete(ns, "alice", collection, rec.ID)

	for _, caller := range []string{"bob", ""} {
		w := httptest.NewRecorder()
		serveGenerated(caller, w, httptest.NewRequest("GET", DisplayURL(rec.ID), nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("caller %q got %d for another account's private image, want 404", caller, w.Code)
		}
	}

	// Shared, it is readable by anyone — that is what the stock pool is.
	if _, err := userdb.Update(ns, "alice", collection, rec.ID, rec.Data, true); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	serveGenerated("", w, httptest.NewRequest("GET", DisplayURL(rec.ID), nil))
	if w.Code != http.StatusOK {
		t.Errorf("a shared image returned %d to a guest", w.Code)
	}
}

// When the provider link is dead and we never got a copy, there is nothing to
// serve — but the caller should get a clean 404, not a hang or a panic.
func TestAnUnfetchableImageIs404(t *testing.T) {
	rec, err := userdb.Create(ns, "alice", collection, map[string]interface{}{
		"prompt": "gone", "url": "not-a-url",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer userdb.Delete(ns, "alice", collection, rec.ID)

	w := httptest.NewRecorder()
	serveGenerated("alice", w, httptest.NewRequest("GET", DisplayURL(rec.ID), nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", w.Code)
	}
}

// The grid must point at this instance. It used to render the provider URL
// straight into src, which is the whole bug.
func TestTheGridRendersOurOwnURL(t *testing.T) {
	html := imageGrid([]userdb.Record{{
		ID:   "abc123",
		Data: map[string]interface{}{"prompt": "x", "url": "https://provider.example/tmp/xyz.png"},
	}})

	if !strings.Contains(html, `src="/images/file/abc123"`) {
		t.Errorf("the grid does not serve from here: %s", html)
	}
	if strings.Contains(html, "provider.example") {
		t.Errorf("the grid still embeds the provider URL: %s", html)
	}
}

// Generating used to render the result and then immediately reload the page, so
// nobody ever saw their image — you were dropped back on /images and had to
// scroll to find it.
func TestGeneratingDoesNotReloadThePage(t *testing.T) {
	w := httptest.NewRecorder()
	handleHTML(w, httptest.NewRequest("GET", "/images", nil))

	if strings.Contains(w.Body.String(), "location.reload") {
		t.Error("the images page still reloads after generating, throwing away the image it just rendered")
	}
}
