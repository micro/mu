package test

// Deleting an account deletes what the account stored.
//
// Six account-scoped services had no cleanup at all: files, contacts, tasks,
// events, images and db. Deleting somebody left their uploaded files, their
// address book, their calendar, their task list, their records and their
// generated images on disk, owned by an account that no longer existed. The
// scheduler would go on firing a deleted person's standing instructions.
//
// Nothing noticed because nothing could ask. userdb had no delete-by-owner —
// Delete takes a single id — so there was no function for a hook to call, and
// the absence looked like a decision rather than a gap.
//
// This is a source check rather than a behavioural one because the hooks are
// wired at startup and the services are separate packages. What it holds is
// the rule: a service that stores per-caller data cleans up after itself, and
// the next one added has to say so here.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// scopedServices are the ones whose Spec says the data belongs to a caller.
// Scoped: true is the declaration that this service holds somebody's things.
func scopedServices(t *testing.T) []string {
	t.Helper()
	var out []string
	entries, err := os.ReadDir(at("service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := os.ReadDir(at("service", e.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") ||
				strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			src, err := os.ReadFile(at("service", e.Name(), f.Name()))
			if err != nil {
				continue
			}
			if regexp.MustCompile(`Scoped:\s*true`).Match(src) {
				out = append(out, e.Name())
				break
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no scoped services found — this test would pass vacuously")
	}
	return out
}

func TestEveryScopedServiceCleansUpWhenAnAccountIsDeleted(t *testing.T) {
	hooks, err := os.ReadFile(at("internal/server/hooks.go"))
	if err != nil {
		t.Fatal(err)
	}
	// The block that registers them, so a mention elsewhere in the file does
	// not count as being wired.
	src := string(hooks)
	i := strings.Index(src, "auth.AccountDeleteHooks = append(")
	if i < 0 {
		t.Fatal("nothing registers account deletion hooks any more")
	}
	j := strings.Index(src[i:], "\n\t)")
	if j < 0 {
		t.Fatal("could not find the end of the deletion hook block")
	}
	block := src[i : i+j]

	// A service's name is usually its package name, and matching on the package
	// is what makes this test indifferent to what the hook is called — mail's is
	// DeleteInbox, wallet's are two. Where the two names diverge, say so here
	// rather than loosening the match: the wallet service is a page and three
	// tools over the ledger in billing, so billing is what has records to drop.
	storedBy := map[string]string{"wallet": "billing"}

	for _, svc := range scopedServices(t) {
		pkg := svc
		if alt, ok := storedBy[svc]; ok {
			pkg = alt
		}
		if !strings.Contains(block, pkg+".") {
			t.Errorf("%s stores per-caller data and nothing deletes it when the "+
				"account goes — its records outlive their owner", svc)
		}
	}
}

// The primitive the services need. Without it each would have had to walk its
// own collections, which is why none of them did.
func TestTheStoreCanDeleteEverythingOneOwnerHas(t *testing.T) {
	src, err := os.ReadFile(at("internal/userdb/userdb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "func DeleteOwner(") {
		t.Error("userdb cannot delete everything one owner has, so a service " +
			"built on it has nothing to call from a deletion hook")
	}
}
