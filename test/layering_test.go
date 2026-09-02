package test

// The direction things are allowed to point.
//
// The top level is the product: home, agent, service, client, admin, account.
//
// client joined it with the package of that name: a service is a name with
// registered handlers and a client is a way to reach one, which is the pair Go
// Micro started with, and both halves belong at the same level. tool left it
// the other way — it derives one list from another and models no noun anybody
// meets, which is machinery.
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
	"slices"
	"strings"
	"testing"
)

// The top-level product packages, in import-path form.
//
// The nested group is not decoration. This used to end each alternative at the
// closing quote — `agent/[a-z]+`, `service/[a-z]+` — which matches "mu/agent"
// and misses "mu/agent/micro". The since-deleted internal/a2a imported the micro agent for a year
// under that regex, and the test that exists to notice said nothing, because a
// package one directory deeper is invisible to a pattern that stops at the
// first level. Anything under a product directory is the product.
var productImport = regexp.MustCompile(`"mu/(home|agent|admin|account|service|client)(/[a-z0-9/]+)?"`)

// The programs. They assemble everything, so they import everything.
var assembly = map[string]bool{
	"internal/server": true,
	"internal/cli":    true,
}

// No service asks the account what money is.
//
// This named the wallet when the wallet was the ledger. The ledger is the
// account's now, and the rule is the same one: a service asks internal/quota
// what an operation costs, and quota deliberately does not know what a balance
// is. A service that could read a balance would start deciding who may afford
// what, which is the gate's job and not fifteen services'.
//
// The wallet is a service today and this rule does not touch it — it holds a
// key, not a balance, and service/wallet importing account/ would be caught
// here like any other.
func TestNoServiceImportsTheAccount(t *testing.T) {
	for pkg, found := range importsFrom(t, "service", regexp.MustCompile(`"mu/account"`)) {
		for _, imp := range found {
			t.Errorf("%s imports the account (%s) — a service asks internal/quota what "+
				"something costs; money is not underneath it", pkg, imp)
		}
	}
}

// And the wider rule the above is one case of.
func TestServicesDoNotReachSideways(t *testing.T) {
	for pkg, found := range importsFrom(t, "service", productImport) {
		for _, imp := range found {
			// One service importing another is caught by
			// TestServicesDoNotImportEachOther, which can say which pair.
			if strings.HasPrefix(imp, `"mu/service/`) {
				continue
			}
			t.Errorf("%s imports %s — anything a service needs from the product is "+
				"wired by the server, not imported", pkg, imp)
		}
	}
}

func TestInternalNeverImportsTheProduct(t *testing.T) {
	found := importsFrom(t, "internal", productImport)
	for pkg, imports := range found {
		if assembly[pkg] {
			continue
		}
		for _, imp := range imports {
			t.Errorf("%s imports %s — internal/ is underneath the product and cannot "+
				"see it; if it needs something up there, the server hands it over", pkg, imp)
		}
	}
	// The exception has to still be in use, or this test is asserting a rule
	// nobody is near breaking.
	if len(found["internal/server"]) == 0 {
		t.Error("internal/server imports no product package, so either the " +
			"assembly moved or this scan is matching nothing")
	}
}

// importsFrom returns package dir -> every distinct offending import, for each
// package under root whose non-test source matches want.
//
// Every, not the first. It used to keep one match per package and stop, which
// meant a package importing both a peer service and the agent showed whichever
// the walk reached first — and the caller that skips peer imports skipped the
// whole package with it.
func importsFrom(t *testing.T, root string, want *regexp.Regexp) map[string][]string {
	t.Helper()
	out := map[string][]string{}
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
		rel, _ := filepath.Rel(at(""), filepath.Dir(path))
		for _, m := range want.FindAll(b, -1) {
			imp := string(m)
			if !slices.Contains(out[rel], imp) {
				out[rel] = append(out[rel], imp)
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

// TestServicesDoNotImportEachOther holds the horizontal rule, the one the
// vertical layering test cannot see.
//
// Product may import internal/; internal/ may never import product. That is
// enforced elsewhere and it says nothing at all about one service importing
// another, which is how flights came to import places for a geocoder and
// whatsapp to import sms for phone-number routing.
//
// A sideways import makes two services one unit: they have to be read together,
// changed together and moved together, and the catalogue stops being a list of
// independent things. Shared functionality belongs underneath both of them, in
// internal/ — and not in a non-service directory under service/, because
// "one directory per service" is only checkable while it is true.
//
// The allowlist is empty, and that is the point. It held nine pairs when this
// test was written — five of them blog composing a digest, which turned out not
// to be an import problem at all but an agent living inside a service. Every one
// was retired by finding what was actually shared and moving it down:
// internal/geo, internal/phone, internal/linkmeta, internal/contacts,
// agent/blog, agent/social, and web absorbing the search directory that was
// never a service.
//
// Adding a line back is not how this test is meant to be passed.
func TestServicesDoNotImportEachOther(t *testing.T) {
	allowed := map[string]string{}

	// Both halves of this used to stop at the first directory, and a package one
	// level deeper slipped between them: the glob was service/<name>/*.go, and
	// the pattern ended at the closing quote so "mu/service/news/digest" did not
	// even look like a service import. service/news/digest imported markets and
	// video for a year under a test whose whole subject is that edge. A rule you
	// can get out of by making a subdirectory is not a rule.
	imports := regexp.MustCompile(`"mu/service/([a-z0-9]+)(?:/[a-z0-9/]+)?"`)
	dirs, err := os.ReadDir(at("service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		from := d.Name()

		var files []string
		if err := filepath.Walk(at("service", from), func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			b, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			for _, m := range imports.FindAllStringSubmatch(string(b), -1) {
				to := m[1]
				if to == from {
					continue
				}
				edge := from + " -> " + to
				if _, ok := allowed[edge]; ok {
					continue
				}
				rel, _ := filepath.Rel(at(""), file)
				t.Errorf("%s imports mu/service/%s (%s) — services must not import "+
					"each other. Whatever they share goes in internal/", edge, to, rel)
			}
		}
	}
}
