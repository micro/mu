package shell

// A pool of machines everybody shares, instead of one machine each.
//
// One container per account is the right shape and it does not fit on a small
// box. Each holds its memory cap whether or not anybody is using it, so a 2GB
// VM running the server, the daemon and one 512MB sandbox has room for exactly
// one caller — and the second one evicts the first. That is a personal dev tool,
// not a service.
//
// # What makes a pool possible
//
// A shared volume. Every pool container mounts the same /work, so any of them
// can serve any caller — there is nothing to move and nothing to remount, which
// is the thing that makes "hand out a warm container" work at all. Without it a
// pooled container would have the wrong files in it, and docker cannot swap a
// volume on a running container.
//
// Each account gets /work/<slug>, owned by a uid of its own and mode 0700, and
// every command runs as that uid. That is ordinary POSIX file isolation and it
// genuinely holds — tested rather than assumed, see TestOneAccountCannotReadAnother.
//
// # What is given up, said plainly
//
// This is a shared machine and the isolation is weaker than a container each.
// Same kernel namespaces, so:
//
//   - Processes are visible. `ps` shows another account's build, including its
//     command line, and a command line is a place people put secrets.
//   - One cgroup per pool container, so a caller running a heavy build slows
//     everybody sharing that container rather than only themselves.
//   - One process limit per pool container, so a fork bomb takes out whoever is
//     on it — the cap contains the host, not the neighbours.
//   - One network namespace, so a port one caller binds is reachable by another
//     on the same container.
//
// None of that matters on an instance with one user, which is why it is worth
// having. All of it matters on an instance with strangers on it, which is why it
// is off by default and why SHELL_SHARED is a decision an operator makes
// rather than something that happens quietly when memory is short. A security
// model that depends on how much RAM the box has is not a security model.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"

	"mu/internal/container"
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

// uidOf is the Unix user an account's commands run as.
//
// Derived from the account rather than allocated, so it survives a restart, a
// rebuilt container and a rebuilt volume with no state to keep in step. The
// range starts well above the system users every image has and is wide enough
// that a collision needs tens of thousands of accounts — and a collision is two
// accounts sharing a directory's permissions, not a crash, which is why it is
// worth saying out loud rather than hiding behind a hash.
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

// readyShared makes sure a pool container is up and the caller has a home in it
// that only they can reach.
//
// # Why there is no chown here
//
// The obvious way to give somebody a directory is to make it and hand it over:
// mkdir as root, chown to their uid. That does not work, and the reason is this
// service's own hardening — a container started with --cap-drop ALL has no
// CAP_CHOWN, so root cannot change the owner of anything. It fails with
// "Operation not permitted" from a uid 0 shell, which reads like nonsense until
// you remember the capability is gone.
//
// So /work is made sticky-writable once, exactly like /tmp, and each account
// makes its own directory as itself. A directory belongs to whoever created it,
// so no ownership has to be transferred and no capability is needed. The sticky
// bit is what makes it safe: on a world-writable directory it means only the
// owner of an entry may remove or rename it, which is precisely the worry —
// somebody deleting somebody else's work — and the kernel refuses it rather
// than us.
//
// Every command after this runs as the account's own uid. See runAs in box.go,
// which is the one place that decides.
func readyShared(ctx context.Context, accountID string) error {
	name := poolOf(accountID)
	if err := container.Start(ctx, name, image(), sharedVolume, limits(), equipment()...); err != nil {
		return err
	}
	touched(name)

	// As root, and only this. 1777 is /tmp's mode: anybody may create an entry,
	// nobody may touch an entry that is not theirs.
	if err := must(ctx, container.Run{
		Name: name, Dir: "/", Wait: quickWait,
		Command: "mkdir -p " + quoted(work) + " && chmod 1777 " + quoted(work),
	}); err != nil {
		return err
	}

	// As the account, so the directory is theirs from the moment it exists.
	// Idempotent, and it re-states the mode every time on purpose: a home that
	// somehow ends up readable by the rest of the pool is the one failure this
	// mode has, and re-stating it costs one exec against a warm container.
	home := sharedHome(accountID)
	return must(ctx, container.Run{
		Name: name, Dir: work, User: runAs(accountID), Wait: quickWait,
		Command: "mkdir -p " + quoted(home) + " && chmod 700 " + quoted(home),
	})
}

// must runs a command this service needs to have worked.
//
// container.Exec returns a failed command as a Result with a non-zero Code and
// a nil error, deliberately — a build that does not compile is an answer and
// not a fault. That is right for the caller's own command and wrong for ours:
// readyShared checked only the error, so a chmod that was refused looked like
// success and the first symptom was a permission denied several calls later,
// blaming the wrong thing. Anything this package runs on its own behalf goes
// through here.
func must(ctx context.Context, r container.Run) error {
	res, err := container.Exec(ctx, r)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("preparing your machine failed: %s", strings.TrimSpace(res.Out))
	}
	return nil
}
