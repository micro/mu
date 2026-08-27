package shell

// Two kinds of test here, and the split is deliberate.
//
// The ones that need no container are the ones about the rules — where a path
// may point, what a machine is called, how long a command may have — and those
// run everywhere, including in CI, which has no daemon. The one that needs a
// real container is skipped unless there is one, because a test that quietly
// passes when the thing it tests is absent is worse than no test.

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"mu/internal/container"
	"mu/internal/service"
)

// A path is inside /work or it is refused.
//
// The container is the real boundary and there is nothing outside /work worth
// having. This is about the paths meaning what they look like: an agent that
// reads /etc/passwd out of its own image gets a confusing answer rather than a
// useful one.
func TestEveryPathIsUnderWork(t *testing.T) {
	for _, in := range []string{"", ".", "a.go", "proj/a.go", "/work", "/work/x", "./x"} {
		got, err := under("anybody", in)
		if err != nil {
			t.Errorf("under(%q) refused a path inside /work: %v", in, err)
			continue
		}
		if got != work && !strings.HasPrefix(got, work+"/") {
			t.Errorf("under(%q) = %q, which is not under %s", in, got, work)
		}
	}
	for _, in := range []string{"/etc/passwd", "../etc", "/work/../etc", "a/../../b", "/"} {
		if got, err := under("anybody", in); err == nil {
			t.Errorf("under(%q) = %q — it should have been refused", in, got)
		}
	}
}

// Two accounts never share a machine.
//
// The readable half of the name is lossy on purpose — it drops the characters
// docker will not take — so on its own it collides, and a collision here hands
// one account another's files. The hash is what makes it safe, and this is the
// test that would notice it being dropped for being ugly.
func TestTwoAccountsNeverShareAMachine(t *testing.T) {
	seen := map[string]string{}
	for _, id := range []string{
		"asim", "Asim", "as.im", "as im", "as/im", "", "..", "-", "u_123", "u.123",
		strings.Repeat("x", 80), strings.Repeat("x", 81),
	} {
		name := boxOf(id)
		if prev, dup := seen[name]; dup {
			t.Errorf("%q and %q both get %q", prev, id, name)
		}
		seen[name] = id

		if !strings.HasPrefix(name, namePrefix) {
			t.Errorf("%q is not named so the reaper can find it: %q", id, name)
		}
		// Docker's own rule for a name, which is what makes this worth doing at
		// all: a name it refuses is an account that cannot have a machine.
		for _, r := range strings.TrimPrefix(name, namePrefix) {
			if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
				t.Errorf("%q produced %q, which docker will not take (%q)", id, name, r)
			}
		}
		if boxOf(id) == volumeOf(id) {
			t.Errorf("%q names its container and its files the same thing", id)
		}
	}
}

// A command gets what it asked for, bounded by what the instance will give.
func TestACommandCannotAskForForever(t *testing.T) {
	if got := allowed(0); got != defaultWait {
		t.Errorf("a command that asked for nothing got %s", got)
	}
	if got := allowed(5 * time.Second); got != 5*time.Second {
		t.Errorf("a short request was overridden: %s", got)
	}
	if got := allowed(9 * time.Hour); got != 10*time.Minute {
		t.Errorf("a command asked for %s and got it", got)
	}
}

// A path with a quote in it is a path, not a second command.
func TestAQuotedPathCannotEndTheCommand(t *testing.T) {
	got := quoted(`a'; rm -rf /; echo '`)
	if strings.Count(got, "'")%2 != 0 {
		t.Errorf("%s leaves an unbalanced quote", got)
	}
	if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
		t.Errorf("%s is not one word", got)
	}
}

// No account, no machine — before anything is started.
//
// Scoped is set on the Spec, so the door refuses an anonymous caller before
// this is reached. It is checked here as well because the page and the tool
// both come through caller(), and a service holding a person's files should not
// depend on somebody else's gate being right.
func TestAMachineNeedsAnAccount(t *testing.T) {
	if _, err := caller(context.Background()); err == nil {
		t.Error("a caller with no account was given a machine")
	}
	if !Spec.Scoped {
		t.Error("the spec is not scoped, so the door would let an anonymous caller in")
	}
}

// A machine may not be the size of the whole box.
//
// This was a flat 2g, which on a 2GB VM is every page the host has. The
// container stays inside its own cgroup while taking them, and the host's OOM
// killer then picks the largest process — the Mu server. A default that can
// kill the thing serving it is not a default.
func TestAMachineIsAShareOfTheHostRatherThanAllOfIt(t *testing.T) {
	host := container.HostMemory()
	if host <= 0 {
		t.Skip("the daemon will not say how much memory this machine has")
	}
	got := parseSize(defaultMemory())
	if got <= 0 {
		t.Fatalf("defaultMemory() = %q, which is not a size", defaultMemory())
	}
	if got > host/2 {
		t.Errorf("one machine gets %d of %d bytes — the server has to fit too", got, host)
	}
	if got < 256*megabyte {
		t.Errorf("one machine gets %d bytes, which is too little to build in", got)
	}
}

// And they do not all fit, so there is a cap on how many run at once.
func TestTheMachinesFitInTheBox(t *testing.T) {
	host := container.HostMemory()
	if host <= 0 {
		t.Skip("the daemon will not say how much memory this machine has")
	}
	n := machineBudget()
	if n < 1 {
		t.Fatalf("machineBudget() = %d, so nobody may have a machine", n)
	}
	if total := int64(n) * parseSize(limits().Memory); total > host {
		t.Errorf("%d machines at %s is %d bytes on a %d byte box", n, limits().Memory, total, host)
	}
}

// docker's size syntax, read back.
func TestASizeIsReadBackAsDockerWroteIt(t *testing.T) {
	for in, want := range map[string]int64{
		"512m":     512 * megabyte,
		"2g":       2048 * megabyte,
		"2G":       2048 * megabyte,
		" 1g ":     1024 * megabyte,
		"1024":     1024,
		"1024b":    1024,
		"nonsense": 0,
		"":         0,
	} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

// The whole thing, against a real container.
func TestAMachineKeepsItsFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !Configured() {
		t.Skip("no container runtime on this machine")
	}

	ctx := service.WithAccount(context.Background(), "sandbox-test-account")
	s := Server{}
	t.Cleanup(func() { DeleteMachine("sandbox-test-account") })

	const src = "package main\n\nfunc main() { println(`it's fine`) }\n"
	var w WriteResponse
	if err := s.Write(ctx, &WriteRequest{Path: "proj/main.go", Content: src}, &w); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read back through a different door than it went in, so a bug in either
	// one shows rather than cancelling out.
	var r RunResponse
	if err := s.Run(ctx, &RunRequest{Command: "cat proj/main.go"}, &r); err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Output != src {
		t.Errorf("what came back is not what went in: %q", r.Output)
	}

	// A failing command is an answer, not an error. This is the property the
	// whole dev loop rests on: a build that does not compile has to come back
	// as output somebody can read.
	if err := s.Run(ctx, &RunRequest{Command: "ls /definitely-not-here"}, &r); err != nil {
		t.Fatalf("a failing command came back as an error: %v", err)
	}
	if r.Code == 0 {
		t.Error("a command that failed reported success")
	}
	if strings.TrimSpace(r.Output) == "" {
		t.Error("a command that failed said nothing about why")
	}

	// Files outlive one call.
	if err := s.Run(ctx, &RunRequest{Command: "echo kept > note"}, &r); err != nil {
		t.Fatalf("run: %v", err)
	}
	var rd ReadResponse
	if err := s.Read(ctx, &ReadRequest{Path: "note"}, &rd); err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.TrimSpace(rd.Content) != "kept" {
		t.Errorf("the file did not survive the call: %q", rd.Content)
	}
}

// On a shared pool, a path is inside the caller's own directory.
//
// The permissions are the real boundary — see TestOneAccountCannotReadAnother —
// but a path check that still resolved against /work would let anybody name
// /work/<somebody-else> and get an error about permissions rather than one
// about whose files those are.
func TestOnASharedPoolAPathIsInsideYourOwnDirectory(t *testing.T) {
	t.Setenv("SANDBOX_SHARED", "on")

	mine := home("alice")
	if mine == work {
		t.Fatalf("alice's home is %s, which is everybody's", mine)
	}
	got, err := under("alice", "proj/main.go")
	if err != nil {
		t.Fatalf("alice cannot reach her own files: %v", err)
	}
	if !strings.HasPrefix(got, mine+"/") {
		t.Errorf("under() = %q, which is not under %q", got, mine)
	}
	// Somebody else's, and the top of the volume, are both outside.
	for _, in := range []string{home("bob"), home("bob") + "/x", work, "/work/../work/bob"} {
		if _, err := under("alice", in); err == nil {
			t.Errorf("alice was allowed to name %q", in)
		}
	}
	// And two accounts never get the same directory or the same user.
	if home("alice") == home("bob") {
		t.Error("two accounts share a directory")
	}
	if uidOf("alice") == uidOf("bob") {
		t.Error("two accounts share a uid, so the permissions separate nothing")
	}
}

// Every path onto a shared machine runs as the caller's own user.
//
// The isolation is POSIX file permissions, so a command that ran as root would
// have none. One path setting the user and another forgetting is a container
// with no isolation and no symptom, which is why runAs is the only place that
// decides and why this checks the callers rather than the function.
func TestEveryPathOntoASharedMachineRunsAsTheCaller(t *testing.T) {
	t.Setenv("SANDBOX_SHARED", "on")
	if runAs("alice") == "" {
		t.Fatal("a shared machine runs commands as nobody in particular, so it is root")
	}
	if runAs("alice") == runAs("bob") {
		t.Error("two accounts run as the same user")
	}
	if fileRun("alice").User != runAs("alice") {
		t.Error("reading and writing files does not go through the caller's user")
	}
	if fileRun("alice").Dir != home("alice") {
		t.Error("reading and writing files does not start in the caller's directory")
	}

	// And a machine of your own has no such user — it is not shared, so there
	// is nobody to be separated from.
	t.Setenv("SANDBOX_SHARED", "")
	if runAs("alice") != "" {
		t.Error("a private machine runs as a synthetic user for no reason")
	}
}

// The pool is bounded and every account lands somewhere in it.
func TestEveryAccountLandsInThePool(t *testing.T) {
	t.Setenv("SANDBOX_SHARED", "on")
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		name := poolOf("account-" + strconv.Itoa(i))
		if !strings.HasPrefix(name, namePrefix) {
			t.Fatalf("%q is not named so the reaper can find it", name)
		}
		seen[name] = true
	}
	if n := machineBudget(); len(seen) > n {
		t.Errorf("200 accounts spread over %d containers, but the budget is %d", len(seen), n)
	}
	// The same account goes to the same place every time, so a process it
	// started stays reachable.
	if poolOf("alice") != poolOf("alice") {
		t.Error("an account moves between containers")
	}
}

// The thing the whole mode rests on: one account cannot read another's files.
func TestOneAccountCannotReadAnother(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !Configured() {
		t.Skip("no container runtime on this machine")
	}
	t.Setenv("SANDBOX_SHARED", "on")

	s := Server{}
	alice := service.WithAccount(context.Background(), "share-alice")
	bob := service.WithAccount(context.Background(), "share-bob")
	t.Cleanup(func() { DeleteMachine("share-alice"); DeleteMachine("share-bob") })

	// A marker that cannot turn up in a path or an error message, because the
	// first version of this test asserted on the word "alice" and a refusal
	// naming /work/share-alice-.../secret contains it — the isolation worked
	// and the test called it a leak.
	const marker = "SQUIRRELPARADE"
	var w WriteResponse
	if err := s.Write(alice, &WriteRequest{Path: "secret", Content: marker}, &w); err != nil {
		t.Fatalf("alice cannot write: %v", err)
	}
	var r RunResponse
	if err := s.Run(bob, &RunRequest{Command: "echo bobs > secret"}, &r); err != nil {
		t.Fatalf("bob cannot write: %v", err)
	}

	// Each reads their own.
	var rd ReadResponse
	if err := s.Read(alice, &ReadRequest{Path: "secret"}, &rd); err != nil || rd.Content != marker {
		t.Errorf("alice got %q (%v) reading her own file", rd.Content, err)
	}

	// And neither reads the other's, by the absolute path, from a shell.
	if err := s.Run(bob, &RunRequest{
		Command: "cat " + quoted(sharedHome("share-alice")+"/secret"),
	}, &r); err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Code == 0 || strings.Contains(r.Output, marker) {
		t.Errorf("bob read alice's file: code=%d output=%q", r.Code, r.Output)
	}
	// Nor list her directory.
	if err := s.Run(bob, &RunRequest{
		Command: "ls -la " + quoted(sharedHome("share-alice")),
	}, &r); err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Code == 0 {
		t.Errorf("bob listed alice's directory: %q", r.Output)
	}

	// And cannot delete them, which is the sharper worry: reading somebody's
	// files is a leak, and rm -rf on them is their afternoon gone. The
	// directory is 0700 and owned by its account, so the kernel refuses both.
	if err := s.Run(bob, &RunRequest{
		Command: "rm -rf " + quoted(sharedHome("share-alice")) + " 2>&1; echo done",
	}, &r); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := s.Read(alice, &ReadRequest{Path: "secret"}, &rd); err != nil || rd.Content != marker {
		t.Errorf("bob deleted alice's files: she now reads %q (%v)", rd.Content, err)
	}

	// Deleting one account leaves the other's files alone, because the thing
	// removed is a directory and not the container they share.
	DeleteMachine("share-alice")
	if err := s.Run(bob, &RunRequest{Command: "cat secret"}, &r); err != nil {
		t.Fatalf("bob lost his machine when alice was deleted: %v", err)
	}
	if !strings.Contains(r.Output, "bobs") {
		t.Errorf("bob lost his files when alice was deleted: %q", r.Output)
	}
}
