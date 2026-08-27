package server

// Every link this product draws goes somewhere.
//
// Three dead links shipped in one afternoon, each in a different way, and none
// of them failed anything:
//
//   - "Saved" in the account menu pointed at /user, which had been deleted
//     along with the feed controls it held.
//   - The Billing entry pointed at /credits.svg, an icon that does not exist —
//     the name was picked by reading the only line that referenced it, which
//     was the same line.
//   - The credits indicator in the header pointed at /account#balance after
//     the balance moved to /wallet.
//
// A link is the one piece of the product with no compiler behind it: it is a
// string, it is right or wrong at the moment somebody clicks it, and the only
// symptom is a page that is not the one they wanted. Deleting a page is
// exactly when links to it go stale, and this repository deletes pages often.
//
// So the links are read out of the source and asked of the mux — net/http
// deciding, the same as the registration test beside this one. The icons are
// held separately, in internal/app, where the embedded files are.
//
// # What this does not check
//
// Fragments. /wallet resolves to /wallet here whether or not
// anything on that page has id="balance", because knowing that means
// rendering the page for an account that does not exist. The path is the half
// that breaks when a page moves; the anchor is the half that breaks when a
// card is renamed, and it fails softly — you land on the right page at the
// top of it.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// registerOnce so this and TestEveryRouteRegistersWithoutConflict can both
// have the routes without registering them twice — which net/http answers with
// the panic that test exists to catch.
var registerOnce sync.Once

func routesReady(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering the routes panics, so this instance cannot boot:\n%v", r)
		}
	}()
	registerOnce.Do(registerRoutes)
}

// linkIn finds href="/somewhere" in Go source. Only whole literals: a path
// built by concatenation has a variable in it, and what that variable holds is
// not a question source text can answer.
var linkIn = regexp.MustCompile(`href="(/[^"'<>{}%\s` + "`" + `]*)"`)

func TestEveryLinkInTheProductGoesSomewhere(t *testing.T) {
	routesReady(t)

	found := map[string][]string{} // path → the files that link to it
	err := filepath.Walk("../..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range linkIn.FindAllStringSubmatch(string(b), -1) {
			// The path alone. A query is arguments to the page and a fragment
			// is a place on it; neither decides whether it exists.
			p := m[1]
			p = strings.SplitN(p, "?", 2)[0]
			p = strings.SplitN(p, "#", 2)[0]
			if p == "" {
				continue
			}
			found[p] = append(found[p], path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(found) < 40 {
		t.Fatalf("found %d linked paths — this scan is broken, not the code", len(found))
	}

	for p, where := range found {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		_, pattern := http.DefaultServeMux.Handler(req)

		// Matched a route of its own: it exists. Whether it then refuses the
		// caller is not this test's business — a redirect to /login is a page
		// doing its job.
		if pattern != "" && pattern != "/" {
			continue
		}

		// Everything else fell through to the catch-all, which serves the
		// embedded static files. The only way to know whether it has this one
		// is to ask it.
		if p != "/" {
			w := httptest.NewRecorder()
			http.DefaultServeMux.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound {
				t.Errorf("%s is linked from %s and nothing serves it",
					p, strings.Join(uniq(where), ", "))
			}
		}
	}
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
