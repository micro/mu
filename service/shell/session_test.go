package shell

import (
	"context"
	"strings"
	"testing"

	"mu/internal/container"
	"mu/internal/service"
)

// cd survives to the next command.
//
// Every call is its own docker exec, so the filesystem persisted and the shell
// session did not: `cd project` then `ls` listed the place you started from.
// The second command works, on the wrong directory, and says nothing about it.
// Nobody who has used a terminal expects that, and an agent working in a
// project directory hits it on its second call.
func TestTheWorkingDirectoryIsWhereYouLeftIt(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !Configured() {
		t.Skip("no container runtime on this machine: " + container.Reason())
	}

	const who = "session-test-account"
	ctx := service.WithAccount(context.Background(), who)
	t.Cleanup(func() { DeleteMachine(who) })
	s := Server{}

	var r RunResponse
	must := func(cmd string) string {
		t.Helper()
		if err := s.Run(ctx, &RunRequest{Command: cmd}, &r); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		return strings.TrimSpace(r.Output)
	}

	must("mkdir -p project/deep && echo inside > project/deep/marker")
	must("cd project/deep")

	// The next call is a different exec, and it starts where the last one
	// finished.
	if got := must("pwd"); !strings.HasSuffix(got, "/project/deep") {
		t.Errorf("pwd is %q, so cd did not survive the call that set it", got)
	}
	if got := must("cat marker"); got != "inside" {
		t.Errorf("a relative path resolved against the wrong directory: %q", got)
	}

	// An explicit dir still wins — the caller has said where they want to be.
	if err := s.Run(ctx, &RunRequest{Command: "pwd", Dir: "."}, &r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.TrimSpace(r.Output), "project") {
		t.Errorf("an explicit dir was overridden by the remembered one: %q", r.Output)
	}

	// And the exit code is the command's own, not the bookkeeping's.
	if err := s.Run(ctx, &RunRequest{Command: "exit 3"}, &r); err != nil {
		t.Fatal(err)
	}
	if r.Code != 3 {
		t.Errorf("exit code came back as %d, want 3 — the wrapper is swallowing it", r.Code)
	}
}

// The machine is one a model recognises.
//
// It was alpine: busybox for /bin/sh and no bash at all. A model trained on
// bash and GNU coreutils writes what it knows, gets something subtly
// different, and the failure looks like the model being bad at the shell
// rather than the shell being unusual — which is the wrong thing to go and
// fix, and this instance spent a while fixing it.
func TestTheMachineHasTheToolsAModelExpects(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !Configured() {
		t.Skip("no container runtime on this machine: " + container.Reason())
	}

	const who = "toolcheck-test-account"
	ctx := service.WithAccount(context.Background(), who)
	t.Cleanup(func() { DeleteMachine(who) })
	s := Server{}

	var r RunResponse
	// One at a time: dash's command -v only looks at its first argument, so
	// asking for several in one call reports on one and passes for the rest.
	//
	// curl and git are on the list because they are promised. The README says a
	// machine is for building things, running tests and cloning a repo, and an
	// agent asked to build against an API reaches for curl before anything
	// else — one asked for a live sports app spent its whole step budget
	// probing endpoints and reported that it had made do with wget.
	for _, want := range []string{"bash", "sed", "grep", "awk", "curl", "git"} {
		if err := s.Run(ctx, &RunRequest{Command: "command -v " + want}, &r); err != nil {
			t.Fatal(err)
		}
		if r.Code != 0 {
			t.Errorf("the machine has no %s, so a model writing ordinary shell will "+
				"fail in ways that look like its own mistake", want)
		}
	}

	// And bash is what runs a command, not merely installed. An image with bash
	// whose sh is dash gives the worst of both: the model's ordinary shell
	// fails and the reason is invisible.
	if err := s.Run(ctx, &RunRequest{Command: `[[ 1 == 1 ]] && echo bashisms-work`}, &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Output, "bashisms-work") {
		t.Errorf("commands are not running under bash: %q (code %d)", r.Output, r.Code)
	}
	// GNU rather than busybox, which is the part that changes behaviour.
	if err := s.Run(ctx, &RunRequest{Command: "sed --version 2>&1 | head -1"}, &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(r.Output), "gnu") {
		t.Errorf("sed is not GNU sed: %q", r.Output)
	}
}

// A command runs where you left off; a written file does not.
//
// The two doors disagree about what a relative path means, on purpose: a shell
// command belongs where the session is, and a file's destination is an argument
// that should mean the same thing whoever reads the call. Both defensible, and
// the combination is a trap — it cost a whole eval run, which reported "no
// file" for pages that existed one directory over, because the harness read
// with a command and a relative path.
//
// So it is pinned here. If somebody makes them agree later that is a decision,
// not a tidy-up, and this test is what makes them make it deliberately.
func TestACommandFollowsTheSessionAndAWriteDoesNot(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !Configured() {
		t.Skip("no container runtime: " + container.Reason())
	}

	const who = "pathrule-test-account"
	ctx := service.WithAccount(context.Background(), who)
	t.Cleanup(func() { DeleteMachine(who) })
	s := Server{}

	var r RunResponse
	if err := s.Run(ctx, &RunRequest{Command: "mkdir -p elsewhere && cd elsewhere"}, &r); err != nil {
		t.Fatal(err)
	}

	// The write ignores that and lands under /work.
	if err := s.Write(ctx, &WriteRequest{Path: "note.txt", Content: "anchored\n"}, &WriteResponse{}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := s.Run(ctx, &RunRequest{Command: "cat /work/note.txt", Dir: "."}, &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Output, "anchored") {
		t.Errorf("a written file did not land under /work: %q", r.Output)
	}

	// The command, meanwhile, is still in elsewhere.
	if err := s.Run(ctx, &RunRequest{Command: "pwd"}, &r); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(r.Output), "/elsewhere") {
		t.Errorf("the session did not stay where it was left: %q", r.Output)
	}
	// And a relative read from there does not find it, which is the trap
	// stated outright rather than left for somebody to rediscover.
	if err := s.Run(ctx, &RunRequest{Command: "cat note.txt"}, &r); err != nil {
		t.Fatal(err)
	}
	if r.Code == 0 {
		t.Errorf("a relative read found the file from another directory, so the "+
			"two path rules have quietly become one: %q", r.Output)
	}
}
