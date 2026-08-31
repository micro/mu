// Package dir is where this instance keeps its things: ~/.mu, and the store,
// keys and logs under it.
//
// A leaf, importing nothing but the standard library, because everything that
// needs the answer has to be able to ask. internal/data would have been the
// obvious home and cannot be one: data imports event, event imports service,
// and service resolves the path too — so putting it there would have left the
// one package that most needs it holding its own copy, which is the shape of
// the bug below rather than a fix for it.
package dir

// Where the store lives, decided in one place — and never the real one under
// `go test`.
//
// # The bug
//
// Ten packages read the store from func init(): auth (accounts, sessions,
// tokens), user (profiles), app (usage, the API log), notes (memory), mail
// (the relay log, the sent-id filter), and the credential stores beside them.
// That is deliberate and the comment in auth/sshkey.go argues for it well: a
// credential store that loads only if somebody wired it up is a credential
// store that is empty on the instance where they forgot.
//
// It is also before any test can do anything. Package init runs before
// TestMain, which runs before the first t.Setenv("HOME", t.TempDir()) — so by
// the time a test redirects HOME, ten packages have already read the real
// ~/.mu, and internal/app/apilog.go has started a goroutine that *writes* to
// it every few seconds for the rest of the run.
//
// The evidence was on the machine this was found on: 91 accounts in
// ~/.mu/data/accounts.json, nearly all of them fixtures — act-agentful,
// chatprivate, csnoop, cthem, compose-keeps — beside a scatter of
// .push_subscriptions.json.tmp and .usage.json.tmp files. `go test ./...` was
// writing into the developer's live instance, and had been for a long time.
//
// # Why this and not ten lazy loaders
//
// The other fix is to move each read out of init behind a sync.Once and call
// it from every accessor. That is more code in more packages, it gives up the
// property sshkey.go names, and it is only correct while nobody adds an
// eleventh init — the failure is silent and the thing it corrupts is not in
// this repo.
//
// This is one function. Under a test binary the store is a temp directory
// unless the test has said otherwise, so no arrangement of inits can reach the
// real instance.
//
// # And a test that sets HOME still wins
//
// HOME is read on every call rather than cached, so t.Setenv("HOME", ...) has
// always worked for anything a test does after it starts. Only the value the
// process was *launched* with is replaced: if the current HOME differs from
// the one at startup, somebody set it on purpose and it is used as it is.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Root is ~/.mu, or the throwaway a test gets instead.
func Root() string {
	home := os.Getenv("HOME")
	if underTest && home == startHome {
		return testRoot()
	}
	return filepath.Join(home, ".mu")
}

// Data is Root()/data, which is where the stores go.
func Data() string { return filepath.Join(Root(), "data") }

var (
	startHome = os.Getenv("HOME")

	// underTest is true in a binary built by `go test`.
	//
	// os.Args[0] rather than testing.Testing(), which would mean importing
	// testing into the server and registering its flags on a production
	// binary. The suffix is what the toolchain produces and has for the life
	// of the tool; -test.v in the argument list is the belt to that braces,
	// for a binary somebody renamed.
	underTest = strings.HasSuffix(os.Args[0], ".test") ||
		strings.HasSuffix(os.Args[0], ".test.exe") ||
		hasTestFlag()

	testOnce sync.Once
	testDir  string
)

func hasTestFlag() bool {
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "-test.") {
			return true
		}
	}
	return false
}

// testRoot is one directory for the whole test binary.
//
// One, not one per test: the packages this exists for read at init, which is
// before there is a test to scope it to. It is left behind rather than cleaned
// up — os.MkdirTemp under the OS temp dir, which the OS reclaims — because the
// process that would have to remove it is the one whose inits are still
// holding it open.
//
// If the temp dir cannot be made, the fallback is a path under it that does
// not exist. That reads as an empty store and fails every write, which is the
// right way to fail here: the alternative is falling back to the real ~/.mu,
// which is the thing this exists to prevent.
func testRoot() string {
	testOnce.Do(func() {
		d, err := os.MkdirTemp("", "mu-test-")
		if err != nil {
			testDir = filepath.Join(os.TempDir(), "mu-test-unavailable")
			return
		}
		testDir = d
	})
	return testDir
}
