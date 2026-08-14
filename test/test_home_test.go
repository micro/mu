package test

// No test run edits somebody's real balance.
//
// internal/data resolves every path under $HOME/.mu/data, read fresh on each
// call. So a test that credits an account, transfers between two, or settles a
// payment writes into whatever ledger the machine happens to have — a
// developer's own balances on their own box, and a shared one on anything that
// runs tests as a real user.
//
// It is not only untidy. It makes the tests lie in both directions: a top-up
// left behind by the last run is still there for the next one, so a test that
// asserts "before + 500" passes or fails depending on history rather than on
// the code. Two of them started failing the moment settlement was deduped
// against the ledger instead of a map in memory, and the tests were right —
// the payment really had already settled, in a previous run.
//
// The fix is three lines of TestMain per package: a temp directory, $HOME
// pointed at it, and remove it after. This says which packages need one.
//
// It cannot catch everything — a package could reach the ledger through some
// indirection this does not name — so it checks the direct case, which is the
// one that has actually happened.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Any package that can reach the ledger, and has tests, isolates its home.
//
// Checked by import rather than by call site, and that is deliberate: the first
// version of this looked for AddCredits and friends in the test files, and it
// passed while admin/ was writing real balances. Admin's tests do not call
// AddCredits — they post a form to a handler that does. Whatever the path, the
// package had to import the money to get there, so the import is the thing to
// ask about.
//
// The account package itself counts. So does a read: CreditsOf creates a record
// when there is none and saves it, which means asking for a balance writes the
// file.
func TestPackagesThatCanReachTheLedgerGetAScratchHome(t *testing.T) {
	root := at("")

	reaches := map[string]bool{} // package dir -> imports the ledger
	hasTests := map[string]bool{}
	isolates := map[string]bool{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "vendor"+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		body := string(b)
		dir := filepath.Dir(rel)

		if strings.Contains(body, `"mu/account"`) || dir == "account" {
			reaches[dir] = true
		}
		if strings.HasSuffix(path, "_test.go") {
			hasTests[dir] = true
			// Isolation is what the TestMain does, not what its file is
			// called.
			if strings.Contains(body, "func TestMain(") && strings.Contains(body, `os.Setenv("HOME"`) {
				isolates[dir] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(reaches) < 3 {
		t.Fatalf("only %d packages reach the ledger — this scan is broken, not the code", len(reaches))
	}

	for dir := range reaches {
		if !hasTests[dir] || isolates[dir] {
			continue
		}
		t.Errorf("%s imports mu/account and has tests, but no TestMain points $HOME "+
			"at a scratch directory — so `go test ./%s/` writes real balances and real "+
			"transactions into ~/.mu/data", dir, dir)
	}
}
