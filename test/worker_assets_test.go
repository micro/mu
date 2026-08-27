package test

// Every file the service worker pre-caches has to exist.
//
// The worker's install handler fills a cache from a list of icons. It used
// caches.addAll, which rejects the entire batch if one request fails — and a
// rejected promise inside an install event's waitUntil means the worker does
// not install. It never activates, never handles a push, and says so nowhere.
// The page registers without complaint, the push service accepts every
// notification, and the handset has no worker to wake.
//
// '/reminder.svg' was in that list and 404ed on the live instance: an icon
// nothing else referenced, left behind by a rename. One missing decoration
// switched off notifications on every device, for everyone, silently. The
// record could only report "sent — the device has not said it arrived", which
// is also what a phone with no signal looks like, so it read as a delivery
// problem for days.
//
// The handler no longer batches, so a missing file can no longer stop the
// install. This is the other half: the list should be true, and a name that
// points at nothing is a bug worth failing a build over rather than absorbing.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// staticList grabs the STATIC_CACHE array from the worker.
var staticList = regexp.MustCompile(`(?s)var STATIC_CACHE = \[(.*?)\]`)

// quoted is one '/path' entry inside it.
var quoted = regexp.MustCompile(`'([^']+)'`)

func TestTheServiceWorkerOnlyPreCachesFilesThatExist(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(root + "/internal/app/html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	m := staticList.FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("no STATIC_CACHE in mu.js; this test is stale")
	}

	entries := quoted.FindAllStringSubmatch(m[1], -1)
	if len(entries) == 0 {
		t.Fatal("STATIC_CACHE parsed as empty")
	}
	for _, e := range entries {
		name := strings.TrimPrefix(e[1], "/")
		if _, err := os.Stat(filepath.Join(root, "internal", "app", "html", name)); err != nil {
			t.Errorf("the worker pre-caches %s and internal/app/html has no such file", e[1])
		}
	}
}

// And a missing one must not be able to stop the worker installing, whatever
// this list says. The list is checked above; this is the belt.
func TestAMissingIconCannotStopTheWorkerInstalling(t *testing.T) {
	b, err := os.ReadFile(repoRoot(t) + "/internal/app/html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if strings.Contains(src, "cache.addAll(STATIC_CACHE)") {
		t.Error("install uses addAll, which rejects the whole batch on one failure " +
			"— and a rejected install means no worker, so no notifications at all")
	}
	if !strings.Contains(src, "cache.add(url).catch(") {
		t.Error("the files are not added individually with the failure swallowed")
	}
	// skipWaiting has to run even when caching fails, or a broken cache leaves
	// the old worker in place for as long as a tab stays open.
	if n := strings.Count(src, "self.skipWaiting()"); n < 2 {
		t.Errorf("skipWaiting appears %d time(s); it must also run on the failure "+
			"path, or a cache error pins the device to its old worker", n)
	}
}
