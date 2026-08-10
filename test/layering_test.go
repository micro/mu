package test

// The direction things are allowed to point.
//
// The top level is the product: home, agent, service, client, admin, wallet.
// Underneath it is internal/, which is everything that has no name a user would
// recognise. The rule is one-way — the product may reach down into internal/,
// and internal/ may never reach back up — with one exception, which is the
// assembly: internal/server and internal/cli are the two programs, and a
// program is allowed to import everything it is assembling.
//
// This is not tidiness. Every time the rule was broken the fix was a function
// variable filled in at boot, and internal/server/hooks.go is where they all
// live: seven hundred lines whose only job is to hand one package a pointer to
// another because they could not import each other. A cycle is not free — it is
// paid for there, and the bill is legible.
//
// So two things are asserted, and they are the two that were being paid for:
//
//   - No package under service/ imports the wallet. A service that wanted to
//     know what something cost used to import money to ask, which put a top
//     level package underneath fifteen of its peers. The question is
//     internal/quota now, which knows prices and does not know balances.
//   - Nothing in internal/ imports the product, except the two programs.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The top-level product packages, in import-path form.
var productImport = regexp.MustCompile(`"mu/(home|agent|admin|wallet|client/[a-z]+|service/[a-z]+)"`)

// The programs. They assemble everything, so they import everything.
var assembly = map[string]bool{
	"internal/server": true,
	"internal/cli":    true,
}

func TestNoServiceImportsTheWallet(t *testing.T) {
	offenders := importsFrom(t, "service", regexp.MustCompile(`"mu/wallet"`))
	for pkg, file := range offenders {
		t.Errorf("%s imports the wallet (%s) — a service asks internal/quota what "+
			"something costs; money is not underneath it", pkg, file)
	}
}

// And the wider rule the above is one case of.
func TestServicesDoNotReachSideways(t *testing.T) {
	for pkg, file := range importsFrom(t, "service", productImport) {
		// A service may build on another service — news reads markets, web
		// reads search. That is a peer edge inside one layer, not a reach out
		// of it.
		if strings.Contains(file, `"mu/service/`) {
			continue
		}
		t.Errorf("%s imports %s — anything a service needs from the product is "+
			"wired by the server, not imported", pkg, file)
	}
}

func TestInternalNeverImportsTheProduct(t *testing.T) {
	found := importsFrom(t, "internal", productImport)
	for pkg, file := range found {
		if assembly[pkg] {
			continue
		}
		t.Errorf("%s imports %s — internal/ is underneath the product and cannot "+
			"see it; if it needs something up there, the server hands it over", pkg, file)
	}
	// The exception has to still be in use, or this test is asserting a rule
	// nobody is near breaking.
	if found["internal/server"] == "" {
		t.Error("internal/server imports no product package, so either the " +
			"assembly moved or this scan is matching nothing")
	}
}

// importsFrom returns package dir -> the offending import line, for every
// package under root whose non-test source matches want.
func importsFrom(t *testing.T, root string, want *regexp.Regexp) map[string]string {
	t.Helper()
	out := map[string]string{}
	seen := 0
	err := filepath.Walk(at(root), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		seen++
		if m := want.Find(b); m != nil {
			rel, _ := filepath.Rel(at(""), filepath.Dir(path))
			if _, dup := out[rel]; !dup {
				out[rel] = string(m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if seen < 20 {
		t.Fatalf("only read %d files under %s — the scan is broken, not the code", seen, root)
	}
	return out
}
