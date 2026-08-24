package shell

// Machines nobody is using.
//
// A container is started on first use and left running, because a shell you
// come back to with your files still in it is the whole point. What that costs
// is memory: the cap is per container, so ten accounts that each ran one command
// last week hold ten times it, for nothing.
//
// A daily limit on running commands would not have touched this — the cost is
// the idle container rather than the calls, and an account that has stopped
// calling is exactly the one holding one. So it is reaped rather than capped,
// which is also what let the price stay a price: see TestOnlyOutboundOrFreeIsCapped.
//
// Stopping is not deleting. The volume is untouched, the next command starts it
// again in about a second, and the files are where they were. Nobody is meant to
// be able to tell this happened except by the pause.

import (
	"context"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/container"
)

var (
	usedMu sync.Mutex
	used   = map[string]time.Time{} // container name → when it last did something
)

// touched records that a machine is in use.
func touched(name string) {
	usedMu.Lock()
	used[name] = time.Now()
	usedMu.Unlock()
}

// idleAfter is how long a machine may sit doing nothing. Long enough that
// somebody thinking between commands does not pay the restart, short enough
// that a forgotten machine is not a permanent cost.
func idleAfter() time.Duration {
	return time.Duration(number(setting("SHELL_IDLE_MINUTES"), 30)) * time.Minute
}

// reap stops the machines nothing has used lately. Started by Load.
func reap() {
	// Asked here rather than by the caller: this is a goroutine, so the wait
	// for the daemon to answer costs nobody anything. See Load.
	if !Configured() {
		return
	}
	// Once at boot, before anything is in the map. A container left running by
	// a previous process is not in this one's memory, so it would never be
	// reaped — and a restart is exactly when nobody is using one.
	if err := stopAll(); err != nil {
		app.Log("shell", "could not tidy up machines from a previous run: %v", err)
	}
	for range time.Tick(reapEvery) {
		cutoff := time.Now().Add(-idleAfter())
		usedMu.Lock()
		var cold []string
		for name, at := range used {
			if at.Before(cutoff) {
				cold = append(cold, name)
				delete(used, name)
			}
		}
		usedMu.Unlock()

		for _, name := range cold {
			ctx, cancel := context.WithTimeout(context.Background(), quickWait)
			if err := container.Stop(ctx, name); err != nil {
				app.Log("shell", "could not stop the idle machine %s: %v", name, err)
			}
			cancel()
		}
	}
}

// room makes space for a machine about to start.
//
// The cap is on how many run at once, and what it is really bounding is memory:
// each one holds its cap whether or not anybody is using it, so a box that fits
// two cannot host five however cheap a command is.
//
// Stopping the idlest rather than refusing the caller. A refusal makes the
// person who turned up last pay for the person who wandered off first, and the
// thing being taken away is not their files — the volume is untouched and their
// next command starts it again. Which is the same trade the reaper makes, on a
// shorter fuse.
func room(ctx context.Context, starting string) {
	budget := machineBudget()

	usedMu.Lock()
	if _, live := used[starting]; live || len(used) < budget {
		// Already running, or there is space. Nothing to take.
		usedMu.Unlock()
		return
	}
	// The victim is picked under the lock and removed from the map here, so two
	// callers arriving at once cannot choose the same one.
	var victim string
	var oldest time.Time
	for name, at := range used {
		if name == starting {
			continue
		}
		if victim == "" || at.Before(oldest) {
			victim, oldest = name, at
		}
	}
	if victim != "" {
		delete(used, victim)
	}
	usedMu.Unlock()

	if victim == "" {
		return
	}
	// Outside the lock: stopping takes seconds, and holding it would serialise
	// every caller behind one docker stop.
	if err := container.Stop(ctx, victim); err != nil {
		app.Log("shell", "could not stop %s to make room: %v", victim, err)
	}
}

// reapEvery is how often to look. Coarse: the thing being reclaimed is memory
// held by something nobody is waiting on.
const reapEvery = 5 * time.Minute

// stopAll stops every machine this instance has started, whenever it started it.
func stopAll() error {
	if !Configured() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), startWait)
	defer cancel()
	// Both names. The previous run may have been a build from before this
	// service was renamed, and its machines are called mu-sandbox-* — see
	// oldNamePrefix. A sweep that knows only the current name leaves them
	// running with nothing left to stop them.
	for _, p := range []string{namePrefix, oldNamePrefix} {
		names, err := container.Running(ctx, p)
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := container.Stop(ctx, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// startWait bounds tidying up at boot, which is a stop per machine.
const startWait = 2 * time.Minute

// DeleteMachine throws away an account's machine and its files.
//
// For account deletion, and it is the one verb here that loses something. Every
// scoped service has to have one — a service holding a caller's data with no way
// to delete it leaves that data behind when its owner goes, which
// TestEveryScopedServiceCleansUpWhenAnAccountIsDeleted exists to catch.
//
// The volume as well as the container, unlike the reaper: this is not "stop
// using it for now", it is "this person is gone".
func DeleteMachine(accountID string) {
	if !Configured() || accountID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), startWait)
	defer cancel()

	if shared() {
		// A directory, never the container. The pool member holding this
		// account's files is holding everybody else's too, and removing it
		// because one person left would take the rest with them — which is the
		// bug this branch exists to not have.
		//
		// As the account rather than as root, which is the opposite of the
		// obvious answer and is forced by the same hardening that removed the
		// chown: with CAP_DAC_OVERRIDE dropped, uid 0 is checked against the
		// permission bits like anybody else, so root cannot even enter a 0700
		// directory it does not own. The account can, because everything in
		// there is its own, and the sticky bit on /work lets an owner remove
		// their own entry.
		home := sharedHome(accountID)
		if _, err := container.Exec(ctx, container.Run{
			Name:    poolOf(accountID),
			Dir:     work,
			User:    runAs(accountID),
			Wait:    startWait,
			Command: "rm -rf -- " + quoted(home),
		}); err != nil {
			app.Log("shell", "removing the files of %s: %v", accountID, err)
		}
		return
	}

	name := boxOf(accountID)
	usedMu.Lock()
	delete(used, name)
	usedMu.Unlock()

	// Removing rather than stopping, and errors logged rather than returned:
	// there is no machine when an account never used one, which is the common
	// case and not a failure.
	if err := container.Remove(ctx, name); err != nil {
		app.Log("shell", "removing %s: %v", name, err)
	}
	if err := container.RemoveVolume(ctx, volumeOf(accountID)); err != nil {
		app.Log("shell", "removing the files of %s: %v", accountID, err)
	}
}
