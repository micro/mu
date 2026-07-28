// Package version reports the Mu release version.
//
// Version is stamped at build time by the release workflow:
//
//	go build -ldflags "-X mu/internal/version.Version=v1.0.0" .
//
// Builds without that flag (go install, go run, local development) report
// "dev" enriched with the VCS revision from the embedded build info, so a
// binary can always be traced back to a commit.
package version

import (
	"runtime/debug"
	"strings"
	"sync"
)

// Version is the release version, set via -ldflags at build time.
var Version = "dev"

var (
	once     sync.Once
	resolved string
)

// String returns the release version. Tagged builds report the tag (e.g.
// "v1.0.0"); untagged builds report "dev (<short commit>)" so support requests
// and bug reports identify an exact build.
func String() string {
	once.Do(func() {
		resolved = Version
		if Version != "dev" {
			return
		}
		rev, dirty := vcsInfo()
		if rev == "" {
			return
		}
		if len(rev) > 12 {
			rev = rev[:12]
		}
		resolved = "dev (" + rev
		if dirty {
			resolved += ", dirty"
		}
		resolved += ")"
	})
	return resolved
}

// Release reports whether this binary was built from a version tag.
func Release() bool { return Version != "dev" }

func vcsInfo() (revision string, dirty bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(s.Value)
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return revision, dirty
}
