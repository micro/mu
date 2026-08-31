package test

// Both names for the notification endpoints are registered.
//
// They moved from /push/* to /notify/*, which is what the feature is called
// everywhere else — there was a /notify service with tools and a page, and four
// /push endpoints doing the same feature under a second name.
//
// The old paths have to stay. A service worker is installed, not loaded: a
// phone that took a copy of mu.js months ago has "/push/received" compiled into
// it and will keep posting there until the browser takes a new copy. A receipt
// that 404s looks exactly like a device that never woke up, which is the one
// distinction the receipts exist to make.
//
// Read out of the source rather than by starting a server: the route table
// registers on the global mux at boot, and a test that stands one up here
// would be asserting against whatever else the suite has already registered.

import (
	"os"
	"strings"
	"testing"
)

func TestBothNotificationPathsAreRegistered(t *testing.T) {
	b, err := os.ReadFile(repoRoot(t) + "/internal/server/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	// The registration is a loop over the two prefixes. Assert on the prefixes
	// and on each endpoint, so dropping either half fails here.
	for _, prefix := range []string{`"/notify/"`, `"/push/"`} {
		if !strings.Contains(src, prefix) {
			t.Errorf("%s is not registered; devices holding that path get a 404", prefix)
		}
	}
	for _, ep := range []string{"subscribe", "unsubscribe", "test", "received"} {
		if !strings.Contains(src, `p+"`+ep+`"`) {
			t.Errorf("the %q endpoint is not registered under both prefixes", ep)
		}
	}
}
