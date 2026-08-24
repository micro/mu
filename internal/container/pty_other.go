//go:build !linux

package container

// No terminal anywhere but Linux.
//
// pty_linux.go opens one with ioctls that only that kernel has — TIOCSPTLCK
// and TIOCGPTN — and its comment said "Linux only, and deliberately not
// abstracted" while nothing enforced it. Saying so in prose compiles
// everywhere; the release does not.
//
// It cross-compiles the one binary for linux/amd64, linux/arm64,
// darwin/amd64 and darwin/arm64, and the two darwin targets failed on those
// two identifiers. `set -e` then took the whole step down, so `gh release
// create` never ran — which is why v1.5.0 has no binaries attached and the
// documented `curl … | sh` dies for anybody without Go.
//
// The same binary is the CLI, so it has to build for a Mac whether or not a
// Mac can ever host a container. This is that: the API exists, and asking for
// a terminal says why there is not one rather than failing to compile.
//
// The file name is the constraint. Go reads _linux and _other from the suffix
// — the same implicit rule that hid a test file called zz_js_test.go from the
// build earlier, because js is a GOOS.

import (
	"fmt"
	"os"
)

// pty is the two ends of one terminal, which this platform does not have.
type pty struct {
	master *os.File
	slave  *os.File
}

// openPTY always fails here, and says what would be needed.
//
// An error rather than a panic: an interactive shell is one door onto the
// sandbox and every other one — the tools, the page — works without a
// terminal. A Mac running this should serve the rest, not refuse to start.
func openPTY() (*pty, error) {
	return nil, fmt.Errorf("an interactive shell needs a Linux host: the " +
		"terminal is opened with ioctls this platform does not have")
}

func (p *pty) resize(rows, cols uint16) {}

func (p *pty) close() {}
