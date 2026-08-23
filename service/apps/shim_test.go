package apps

// The web platform, put back inside the sandbox.
//
// An app runs in an opaque origin, so localStorage throws and fetch is dead —
// connect-src 'none'. Everything an app could do was therefore spelled
// mu.something, and that is a real cost rather than a matter of taste: a model
// has seen localStorage.setItem a billion times and mu.store.set never, so
// every generated app paid the difference. These two names are the fix, and it
// is not a longer prompt.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/data"
)

func TestTheShimPutsBackLocalStorageAndFetch(t *testing.T) {
	for _, want := range []string{
		`Object.defineProperty(window,'localStorage'`,
		`Object.defineProperty(window,'sessionStorage'`,
		`window.fetch=function(input,init)`,
	} {
		if !strings.Contains(appShimJS, want) {
			t.Errorf("the shim does not define %s", want)
		}
	}

	// localStorage persists per user; sessionStorage must not, or a promise the
	// web makes everywhere else quietly becomes permanent here.
	if !strings.Contains(appShimJS, "mu.store.set(k,mem[k])") {
		t.Error("localStorage does not write through to the store")
	}
	if strings.Contains(appShimJS, "temp[String(k)]=String(v);mu.store") {
		t.Error("sessionStorage writes to the server")
	}
}

// fetch is the API door and nothing else. mu.get(path) was removed precisely
// because it took any path on this origin with the viewer's cookies, and a
// fetch shim that accepted anything would put that back.
func TestTheFetchShimOnlyReachesTheAPIDoor(t *testing.T) {
	if !strings.Contains(appShimJS, `/api/v1/`) {
		t.Fatal("the fetch shim does not name the API door")
	}
	if !strings.Contains(appShimJS, "mu.service(m[1],m[2],args)") {
		t.Error("the fetch shim does not route through the service proxy, which is " +
			"where the caller is bound and the price is charged")
	}
	// Refused in words. An app that tried an external API should be told why,
	// not left waiting for a promise that never settles.
	if !strings.Contains(appShimJS, "Anything else is blocked by the sandbox") {
		t.Error("a blocked URL is not explained")
	}
}

// The values have to be in the document before the script runs, because
// localStorage answers synchronously and the bridge does not.
func TestTheStoreIsSeededIntoTheDocument(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defer os.RemoveAll(os.Getenv("HOME"))

	const who = "seed-reader"
	if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}); err != nil {
		t.Fatal(err)
	}

	// Nothing saved, nothing inlined — an empty object in front of every page
	// is noise.
	if got := appSeedJS("nothing-here", who); got != "" {
		t.Errorf("an app with no saved values still got a seed: %q", got)
	}
	if got := appSeedJS("some-app", ""); got != "" {
		t.Error("a signed-out reader got somebody's saved values")
	}

	data.SaveJSON("apps/seeded/"+who+".json", map[string]interface{}{"count": 7, "name": "Ada"}) //nolint:errcheck
	got := appSeedJS("seeded", who)
	if !strings.HasPrefix(got, `<script>window.__muStore=`) {
		t.Fatalf("the seed is not a script: %q", got)
	}

	raw := strings.TrimSuffix(strings.TrimPrefix(got, `<script>window.__muStore=`), `;</script>`)
	var back map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatalf("the seed is not JSON: %v (%q)", err, raw)
	}
	if back["name"] != "Ada" {
		t.Errorf("the values did not survive: %+v", back)
	}

	// And the shim reads it from where the seed puts it.
	if !strings.Contains(appShimJS, "window.__muStore") {
		t.Error("the shim does not read the seeded values")
	}
}
