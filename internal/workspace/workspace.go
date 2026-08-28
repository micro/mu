// Package workspace is how something reads a file off an account's machine
// without knowing what a machine is.
//
// # Why this exists at all
//
// apps needs to host a page that was built in a shell. That is one service
// wanting something another one holds, and services here do not import each
// other — TestServicesDoNotImportEachOther enforces it, and its message names
// the remedy: whatever they share goes in internal/.
//
// The shared thing is small and worth stating exactly: given an account and a
// path, the bytes. Not a container, not a volume, not the pool, not the uid
// rules — those are service/shell's, they are entangled with how isolation
// works, and nothing outside that package should be able to reach them.
//
// # Why a function variable rather than a package that does the work
//
// The fuller answer is an internal/box holding the whole machine mechanism,
// with shell reduced to endpoints and policy on top of it. That is probably
// where this ends up. It is not what this is, because that move would carry the
// shared-pool and run-as-uid code with it — the code whose own comment says a
// path that forgets to set the user is "a container with no isolation and no
// symptom" — and a refactor of that belongs in a change about that, where it
// can be looked at properly rather than as a step on the way somewhere else.
//
// So the seam is declared here and filled in by whoever can: the same shape
// apps already uses for QuotaCheck and ChargeQuota, which main.go sets. An
// instance with no shell service has a nil Read and says so, which is the right
// answer on a machine with no container runtime anyway.
package workspace

import (
	"context"
	"fmt"
)

// Read is set by service/shell at registration. Nothing else may set it, and
// nothing here calls it directly — see ReadFile, which is what callers use.
var Read func(ctx context.Context, accountID, path string) ([]byte, error)

// ReadFile reads one file from an account's machine.
//
// The account is named rather than taken from the context because the caller is
// acting on that account's behalf, having already established who they are.
// Passing it makes that explicit, and stops a handler being tempted to read an
// identity out of its arguments.
//
// The path is relative to the account's own working directory, and resolving it
// — including refusing one that climbs out — is the implementation's job, not
// the caller's.
func ReadFile(ctx context.Context, accountID, path string) ([]byte, error) {
	if Read == nil {
		return nil, fmt.Errorf("this instance has no machine to read files from")
	}
	return Read(ctx, accountID, path)
}
