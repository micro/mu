package shell

// Legacy shared-workspace names are retained for locating existing data.
// Execution is blocked in ready: shared processes, ports and hash-derived Unix
// identities do not provide the account isolation required by a hosted instance.
// Do not re-enable this path without migrating to isolated account containers.

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
)

// shared reports whether this instance pools machines rather than giving one
// per account.
func shared() bool {
	switch strings.ToLower(setting("SHELL_SHARED")) {
	case "on", "true", "1", "yes", "shared":
		return true
	}
	return false
}

// sharedVolume is the one volume every pool container mounts. Its name has no
// account in it, which is the point.
const sharedVolume = "mu-work-shared"

// poolOf is which pool container serves an account.
//
// By hash rather than round-robin, so the same account lands in the same place
// every time. Any container would answer correctly — they all see the same
// volume — but a background process somebody started stays reachable, and the
// page cache stays warm for the files they keep touching.
func poolOf(accountID string) string {
	n := machineBudget()
	if n < 1 {
		n = 1
	}
	return fmt.Sprintf("%spool-%d", namePrefix, int(hash32(accountID)%uint32(n)))
}

// uidOf preserves the legacy mapping for locating data; it is not safe isolation.
func uidOf(accountID string) int {
	return uidBase + int(hash32(accountID)%uidRange)
}

const (
	uidBase  = 20000
	uidRange = 40000
)

func hash32(s string) uint32 {
	sum := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint32(sum[:4])
}

// sharedHome is where an account's files live inside the pool.
func sharedHome(accountID string) string { return work + "/" + slug(accountID) }
