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
	// service/*, plus the one package that registers a Spec from the top level:
	// wallet is a staple and a service at the same time, and it is the one
	// holding money, so dropping out of this scan is the last thing it should do.
	dirs := [][]string{{"wallet"}}
	entries, err := os.ReadDir(at("service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, []string{"service", e.Name()})
		}
	}
	for _, dir := range dirs {
		files, err := os.ReadDir(at(dir...))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") ||
				strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			src, err := os.ReadFile(at(append(append([]string{}, dir...), f.Name())...))
			if err != nil {
				continue
			}
			if regexp.MustCompile(`Scoped:\s*true`).Match(src) {
				out = append(out, dir[len(dir)-1])
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

	// A service's name is its package name, which is what makes this test
	// indifferent to what the hook is actually called — mail's is DeleteInbox,
	// wallet registers two.
	for _, svc := range scopedServices(t) {
		if !strings.Contains(block, svc+".") {
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
