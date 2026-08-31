package dir

// The two halves, and they pull against each other.
//
// A test binary must not be able to reach the instance somebody actually uses,
// and a test that points HOME somewhere on purpose must still get what it
// asked for. Fail the first and `go test ./...` writes fixtures into a
// developer's live accounts.json — which it did, 91 of them. Fail the second
// and every test that sets up its own home silently shares one directory with
// every other.
//
// These live here rather than in test/ because that package's TestMain has
// already redirected HOME by the time anything runs, so from in there the
// "real" home and the redirected one are the same string and neither property
// is observable. startHome is only honest in a package that has not moved it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestATestBinaryGetsAThrowaway(t *testing.T) {
	if !underTest {
		t.Fatal("underTest is false inside a test binary — os.Args[0] is " +
			os.Args[0] + ", and every init read in this repo is pointed at the " +
			"real ~/.mu")
	}

	root := Root()
	if live := filepath.Join(startHome, ".mu"); root == live {
		t.Fatalf("Root() is %s, which is the instance somebody uses — ten packages "+
			"read it from func init(), before any test can redirect HOME", root)
	}
	if !strings.HasPrefix(filepath.Base(root), "mu-test-") &&
		!strings.HasPrefix(filepath.Base(root), "mu-test") {
		t.Errorf("Root() is %s, which is neither the live instance nor the "+
			"throwaway — where is it writing?", root)
	}
	if !strings.HasPrefix(Data(), root) {
		t.Errorf("Data() is %s, which is not under %s", Data(), root)
	}

	// One directory for the binary, not one per call: the packages this exists
	// for read at init, which is before there is a test to scope it to.
	if a, b := Root(), Root(); a != b {
		t.Errorf("Root() moved between calls: %s then %s — anything read at init "+
			"is now somewhere nothing can find it", a, b)
	}
}

// And a test that sets HOME still decides.
//
// HOME is read on every call rather than cached, so this has always worked for
// anything a test does after it starts. Only the value the process was
// launched with is replaced.
func TestSettingHomeStillDecides(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if got, want := Root(), filepath.Join(tmp, ".mu"); got != want {
		t.Fatalf("Root() = %s, want %s — a test that sets HOME is being ignored, "+
			"which breaks every fixture that depends on it", got, want)
	}
	if got, want := Data(), filepath.Join(tmp, ".mu", "data"); got != want {
		t.Errorf("Data() = %s, want %s", got, want)
	}
}

// Outside a test binary it is exactly ~/.mu and nothing clever.
func TestTheServerGetsItsOwnDirectory(t *testing.T) {
	was := underTest
	underTest = false
	t.Cleanup(func() { underTest = was })

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if got, want := Root(), filepath.Join(tmp, ".mu"); got != want {
		t.Errorf("Root() = %s, want %s — the redirect is leaking into the server, "+
			"which would run against a temp directory that is emptied on reboot",
			got, want)
	}
}
