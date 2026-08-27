package shell

// A real container, from the real daemon, through the real code path.
// Skipped where there is no Docker.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/container"
)

func TestLiveBoxIsEquipped(t *testing.T) {
	if !container.Available() {
		t.Skip("no docker: " + container.Reason())
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SANDBOX_MEMORY", "256m")
	t.Setenv("SANDBOX_CPUS", "0.5")
	t.Setenv("SANDBOX_NETWORK", "bridge")

	const who = "livetester"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := ready(ctx, who); err != nil {
		t.Fatalf("ready: %v", err)
	}
	t.Cleanup(func() { DeleteMachine(who) })

	// The binary is in the box and runs there — which is a different claim
	// from being there, and the one that was wrong first time. A Go binary
	// built the ordinary way is dynamically linked, Alpine has no glibc
	// loader, and the shell reports "not found" about a file it has just
	// listed. No pipe here: a pipeline's exit status is the last command's,
	// so `mu --help | head` returns 0 however badly mu failed.
	equipped := len(equipment()) > 0
	res, err := container.Exec(ctx, container.Run{
		Name: machineFor(who), Dir: home(who), User: runAs(who),
		Command: "ls -l " + muPath + " 2>&1; " + muPath + " --help > /dev/null 2>&1; echo code=$?",
		Wait:    60 * time.Second,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	t.Logf("equipped=%v, in the box:\n%s", equipped, res.Out)

	switch {
	case equipped && !strings.Contains(res.Out, "code=0"):
		t.Errorf("the CLI was mounted but will not run in the box:\n%s", res.Out)
	case !equipped && !strings.Contains(res.Out, "No such file"):
		t.Errorf("nothing was mounted, so the box should not have %s:\n%s", muPath, res.Out)
	}

	// Read-only, checked at the mount rather than by trying to write: a write
	// to a running executable fails with "text file busy" whether or not the
	// mount is ro, which would make the check pass for the wrong reason.
	if equipped {
		mnt, err := container.Exec(ctx, container.Run{
			Name: machineFor(who), Dir: home(who), User: runAs(who),
			Command: "grep ' " + muPath + " ' /proc/mounts", Wait: 30 * time.Second,
		})
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		t.Logf("mount: %s", strings.TrimSpace(mnt.Out))
		if !strings.Contains(mnt.Out, "ro,") && !strings.Contains(mnt.Out, " ro ") {
			t.Errorf("the CLI is not mounted read-only: %s", mnt.Out)
		}
	}

	// The credential reaches one exec...
	with, err := container.Exec(ctx, container.Run{
		Name: machineFor(who), Dir: home(who), User: runAs(who),
		Command: "echo url=$MU_URL token=${MU_TOKEN:+set}",
		Env:     map[string]string{"MU_URL": "https://example.test", "MU_TOKEN": "secret-value"},
		Wait:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(with.Out, "url=https://example.test") || !strings.Contains(with.Out, "token=set") {
		t.Errorf("the environment did not reach the exec: %q", with.Out)
	}

	// ...and to no other exec in the same container. This is the property the
	// whole design rests on.
	without, err := container.Exec(ctx, container.Run{
		Name: machineFor(who), Dir: home(who), User: runAs(who),
		Command: "echo url=[$MU_URL] token=[$MU_TOKEN]",
		Wait:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(without.Out, "url=[] token=[]") {
		t.Errorf("a later exec inherited the session's credential: %q", without.Out)
	}
	t.Logf("second exec sees: %s", strings.TrimSpace(without.Out))

	// Nor is it on the container itself, where `docker inspect` would show it
	// to anything that can read the daemon.
	insp, err := container.Exec(ctx, container.Run{
		Name: machineFor(who), Dir: home(who), User: runAs(who),
		Command: "cat /proc/1/environ | tr '\\0' '\\n' | grep -c MU_TOKEN || true",
		Wait:    30 * time.Second,
	})
	if err == nil && strings.TrimSpace(insp.Out) != "0" {
		t.Errorf("the container's own process carries the token: %q", insp.Out)
	}

	if os.Getenv("KEEP") == "" {
		t.Log("cleaning up")
	}
}
