package shell

// Reading a caller's file from inside this binary.
//
// Read, the endpoint above, is the door for an agent: it takes a context with
// an identity in it and hands the bytes back through the tool layer. This is
// the same act for something that is already inside the process and already
// knows whose files it wants — apps, publishing a page somebody built in their
// box.
//
// It is not called directly by that caller. Services here do not import each
// other, so the seam is declared in internal/workspace and filled in below —
// apps asks workspace for bytes and never learns that a container was involved,
// and shell never learns that an app exists. A machine that had to know what an
// app was would stop being a machine.

import (
	"context"

	"mu/internal/container"
	"mu/internal/workspace"
)

// Filling in the seam. Registration order does not matter: nothing reads a
// file during init, and a nil Read says so plainly rather than crashing.
func init() { workspace.Read = ReadFile }

// ReadFile reads one file from an account's machine.
//
// The account is named rather than taken from a context, because the caller is
// not answering a request from that account — it is acting on their behalf,
// having already established who they are. Passing the id makes that explicit
// and stops a handler being tempted to read an identity out of arguments.
//
// The path is resolved under the account's own /work and nowhere else, by the
// same function the endpoints use, so ".." leads out of nothing.
func ReadFile(ctx context.Context, accountID, p string) ([]byte, error) {
	path, err := under(accountID, p)
	if err != nil {
		return nil, err
	}
	if err := ready(ctx, accountID); err != nil {
		return nil, err
	}
	b, err := container.ReadFile(ctx, fileRun(accountID), path)
	if err != nil {
		return nil, err
	}
	return b, nil
}
