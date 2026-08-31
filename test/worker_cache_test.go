package test

// The service worker must never be served from a cache.
//
// It was: /mu.js came back with `Cache-Control: public, max-age=86400`, from a
// rule that treats every .js file the same. A service worker is not like every
// other .js file. It is installed rather than loaded, and the browser replaces
// it only when it fetches the script and finds different bytes — a fetch that,
// under the default updateViaCache, goes through the HTTP cache. So for a day
// after any visit, every update check on that device got the cached copy back,
// found it identical, and kept the worker it already had. On a phone that opens
// the app most days, that is never.
//
// What it cost: the worker handling push on a real handset predated the code
// that reports a notification arrived. Every send recorded "sent — the device
// has not said it arrived", which is also exactly what a device that never woke
// up looks like. Days went into the sending half, which was correct the whole
// time, because the record could not tell those two apart and the reason it
// could not was a cache header on an unrelated line.
//
// Both halves are asserted: the header, so there is nothing stale to find, and
// updateViaCache:'none' on the registration, so the fetch does not consult the
// cache at all. Either one alone leaves a device that can still be a day behind.

import (
	"os"
	"strings"
	"testing"
)

func TestTheServiceWorkerIsNotCached(t *testing.T) {
	root := repoRoot(t)

	b, err := os.ReadFile(root + "/internal/app/app.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if !strings.Contains(src, `w.Header().Set("Cache-Control", "no-cache")`) {
		t.Error("nothing serves the worker with no-cache, so a device can run a " +
			"copy up to a day old — and registration.update() cannot get past it either")
	}
	// And it has to be mu.js specifically that gets it, not everything.
	if !strings.Contains(src, `r.URL.Path == "/mu.js"`) {
		t.Error("the no-cache branch does not name /mu.js")
	}

	// Every registration, not just the app shell's: the front page has its own.
	for _, file := range []string{"/internal/app/app.go", "/home/index.go"} {
		b, err := os.ReadFile(root + file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "serviceWorker.register") {
			continue
		}
		if !strings.Contains(s, "updateViaCache: 'none'") {
			t.Errorf("%s registers the worker without updateViaCache:'none', so the "+
				"update check reads the browser's HTTP cache", file)
		}
	}
}
