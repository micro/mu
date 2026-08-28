package test

// What you offer somebody with no account has to open without one.
//
// TestEveryFooterLinkOpensForAStranger holds this for the footer, and the bug it
// was written for turned up again one page over the moment the footer was fixed:
// the signed-out notice in a chat room said "You can still try Mu without an
// account in the public agent" and linked /agent, which checks auth in its
// handler and redirects to /login. A reader with no account was told there was a
// way in, took it, and landed on the login page they had just declined.
//
// The chat test at the time asserted the notice contained "/agent" and the words
// "Try Mu without an account". Both were true. Neither was the promise.
//
// So the rule is not about the footer, it is about the audience: any block of
// markup that renders a "log in" or "sign up" link is by construction being
// shown to somebody who has no session. Every other link in that same block is
// an offer made to that person, and it must not be a path that turns them away.
//
// The scan is over string literals in the source rather than rendered pages
// because these notices are built in half a dozen packages, most of them
// unexported, and a rule you can only check where you remember to render is the
// rule that was already missed twice.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// signInLink marks a literal as addressed to somebody signed out.
var signInLink = regexp.MustCompile(`/(?:login|signup)\?redirect=`)

// anyHref finds the destinations offered in the same literal.
var anyHref = regexp.MustCompile(`href=["'](/[a-zA-Z0-9/_-]*)["']`)

func TestNoSignedOutCTASendsAStrangerToLogin(t *testing.T) {
	root := repoRoot(t)
	gated := gatedPaths(t, root)

	var bad []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		for _, lit := range rawStrings(string(b)) {
			if !signInLink.MatchString(lit) {
				continue
			}
			for _, m := range anyHref.FindAllStringSubmatch(lit, -1) {
				if gated[m[1]] {
					bad = append(bad, rel+": offers "+m[1]+
						" beside a log in link, and "+m[1]+" refuses without a session")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(bad) > 0 {
		t.Errorf("a link shown to somebody with no account has to open without one. "+
			"%d place(s):\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}

// gatedPaths reads the route table and adds the handful gated in a handler,
// which is the same ledger TestEveryFooterLinkOpensForAStranger keeps.
func gatedPaths(t *testing.T, root string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(root + "/internal/server/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	gated := map[string]bool{}
	for _, m := range routeGate.FindAllStringSubmatch(string(b), -1) {
		if m[2] == "true" {
			gated[m[1]] = true
		}
	}
	for _, p := range gatedInHandler {
		gated[p] = true
	}
	if len(gated) < 5 {
		t.Fatalf("only %d gated paths found — this scan is broken, not the code", len(gated))
	}
	return gated
}
