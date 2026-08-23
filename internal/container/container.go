// Package container runs things somewhere they cannot reach this machine.
//
// Docker today, and named for what it provides rather than for who provides it.
// That is the same reason internal/twilio sits under service/sms rather than
// being it: the service holds the rules about who may run what, this holds the
// mechanism, and a second mechanism — podman, gVisor, a Firecracker microVM —
// is then an adapter rather than a rewrite. The service asks for a place to run
// a command; it does not ask for Docker.
//
// # Shelling out
//
// To the docker CLI, rather than to the Engine API over its socket. Two
// reasons, and the second is the real one.
//
// The API would need a client library, and the argument for one is thin: this
// package runs six verbs and none of them stream. The CLI is on every machine
// that has a daemon and its output for these six is stable.
//
// The other is that talking to the socket means holding a path to the socket,
// and a path to the socket is root on the host — a container that can reach
// /var/run/docker.sock can start a container with the host's filesystem mounted
// in it. Nothing here ever passes that path to anything it starts, and the
// shape of this package is what makes that easy to check: the argv is built in
// one place, in run below, and there is no mount anybody can add from outside.
//
// # What it is not
//
// Not a scheduler and not a pool. One container per caller, started when it is
// first needed and left running until it is idle, which is what a person
// working in a directory expects — a shell you come back to, with your files
// still in it. Anything cleverer is a decision for whoever needs it.
package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Available reports whether this machine can run a container.
//
// Both halves, because they fail differently and only one of them is the
// operator's fault: the CLI may be absent, or present with no daemon behind it.
// Asked once — the answer does not change while the process runs, and it costs
// a subprocess.
func Available() bool {
	availableOnce.Do(func() {
		if _, err := exec.LookPath(binary); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), probeWait)
		defer cancel()
		// `info` rather than `version`: version answers from the client alone
		// and succeeds with no daemon running, which is the exact failure this
		// is meant to catch.
		available = exec.CommandContext(ctx, binary, "info", "--format", "{{.ServerVersion}}").Run() == nil
	})
	return available
}

var (
	availableOnce sync.Once
	available     bool
)

const binary = "docker"

// probeWait bounds the availability check. A daemon that has not answered in
// this long is one no caller wants to wait for either.
const probeWait = 10 * time.Second

// HostMemory is how much memory the daemon has to hand out, in bytes, or 0 if
// it will not say.
//
// The daemon's view rather than this process's. Mu may itself be in a container
// with a cgroup limit of its own, and reading /proc/meminfo from in there
// answers a different question than the one being asked — the sandboxes are
// siblings on the host, so what bounds them is what the host has.
//
// Asked once. It does not change while the machine is up, and it costs a
// subprocess.
func HostMemory() int64 {
	hostOnce.Do(hostFacts)
	return hostMemory
}

// hostFacts asks the daemon what the machine has. One call for both, because
// they are one question and it is a subprocess.
func hostFacts() {
	if !Available() {
		return
	}
	out, err := run(context.Background(), probeWait, "info", "--format", "{{.MemTotal}} {{.NCPU}}")
	if err != nil {
		return
	}
	parts := strings.Fields(out)
	if len(parts) != 2 {
		return
	}
	hostMemory, _ = strconv.ParseInt(parts[0], 10, 64)
	hostCPUs, _ = strconv.Atoi(parts[1])
}

// HostCPUs is how many cores the daemon has, or 0 if it will not say.
func HostCPUs() int {
	hostOnce.Do(hostFacts)
	return hostCPUs
}

var (
	hostOnce   sync.Once
	hostMemory int64
	hostCPUs   int
)

// Limits are what one container may have.
//
// Every field has a value by the time it reaches here — the service fills them
// from its own settings. Zero is not "unlimited" in this package, it is a bug,
// and Start says so rather than starting something uncapped: an uncapped
// container is the whole risk this package exists to bound, and the one failure
// mode where a silent default is worse than an error.
type Limits struct {
	Memory  string // docker's own syntax: "2g"
	CPUs    string // "1", "0.5"
	PIDs    int    // process count, against a fork bomb
	Network string // "bridge", or "none" for a container that cannot reach anything
}

// Start makes sure a named container is running, and creates it if it is not.
//
// Idempotent, because the caller's question is "is my sandbox there" and not
// "make me one" — a caller that had to track whether it had already started
// would get it wrong across a restart of this process, which is exactly when it
// matters.
//
// The volume is mounted at /work and outlives the container: an image is
// replaced, a container is thrown away and rebuilt, and neither should lose
// somebody's files. It is created on demand by the daemon.
func Start(ctx context.Context, name, image, volume string, l Limits) error {
	if !Available() {
		return fmt.Errorf("this machine has no container runtime")
	}
	if l.Memory == "" || l.CPUs == "" || l.PIDs <= 0 {
		return fmt.Errorf("refusing to start %s with no limits set", name)
	}

	switch state, err := inspect(ctx, name); {
	case err != nil:
		return err
	case state == "running":
		return nil
	case state != "":
		// There and stopped: an instance that restarted, or an idle reap. The
		// files are in the volume either way, so this is a resume.
		_, err := run(ctx, startWait, "start", name)
		return err
	}

	network := l.Network
	if network == "" {
		network = "none"
	}
	// --rm is deliberately absent. A container that removes itself on exit
	// takes its own logs with it, and the failure worth debugging here is the
	// one where it died on its own.
	_, err := run(ctx, startWait,
		"run", "--detach",
		"--name", name,
		"--memory", l.Memory,
		// The same number again, which is how docker is told "no swap".
		// Without it a container gets swap equal to its memory for free, so a
		// 2g cap is really 2g of RAM plus 2g of the host's swap — and the
		// symptom is not an OOM kill, it is the whole box thrashing while the
		// container stays inside its limit.
		"--memory-swap", l.Memory,
		"--cpus", l.CPUs,
		"--pids-limit", strconv.Itoa(l.PIDs),
		"--network", network,
		// Nothing it does may gain it anything: no new privileges through a
		// setuid binary, and none of the capabilities a root process would
		// normally inherit. It is still root inside its own namespace, which is
		// what lets somebody apk add a compiler.
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--volume", volume+":/work",
		"--workdir", "/work",
		image,
		// Something that does not exit. The container is a place to exec into;
		// its own process is not the work.
		"sleep", "infinity",
	)
	return err
}

// startWait bounds starting a container. Long, because the first one on a
// machine pulls an image over the network.
const startWait = 10 * time.Minute

// inspect is a container's state, or "" if there is no such container.
func inspect(ctx context.Context, name string) (string, error) {
	out, err := run(ctx, probeWait, "inspect", "--format", "{{.State.Status}}", name)
	if err != nil {
		// "there is no such container" is an answer, not a failure — it is how
		// the first call for an account reports that nothing has been made yet.
		// Matched case-insensitively and on both nouns docker uses: it says
		// "no such object" here and "No such container" elsewhere, and matching
		// the capitalised spelling alone made every first start fail.
		if noSuch(err.Error()) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// noSuch reports whether a docker failure is only "that does not exist".
func noSuch(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "no such object") ||
		strings.Contains(msg, "no such container")
}

// Result is what a command did.
//
// The exit code is not an error. A build that fails is a normal outcome the
// caller wants to read and hand back, and turning it into a Go error would put
// the useful half — the compiler's output — on the floor. An error here means
// the command could not be run at all.
type Result struct {
	Out  string
	Code int
}

// Exec runs a command inside a container and waits for it.
//
// A shell, deliberately. The caller is an agent that writes `cd cmd && go build
// ./... 2>&1 | head -50`, and a bare argv cannot express any of that — pipes,
// redirection, chained commands. It is the shell inside a container with no
// capabilities and no route back here, which is the whole reason the container
// is there.
//
// Output is combined. Anything worth reading writes to both, and a caller
// holding two strings has to decide how to interleave them, which it cannot do
// correctly — the ordering is lost the moment they are separated.
func Exec(ctx context.Context, name, command, workdir string, wait time.Duration) (Result, error) {
	if strings.TrimSpace(command) == "" {
		return Result{}, fmt.Errorf("no command")
	}
	if workdir == "" {
		workdir = "/work"
	}
	out, err := run(ctx, wait, "exec", "--workdir", workdir, name, "sh", "-c", command)
	if err == nil {
		return Result{Out: out}, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// The command ran and failed, which is an answer rather than a fault.
		return Result{Out: out, Code: exit.ExitCode()}, nil
	}
	return Result{}, err
}

// WriteFile puts a file inside a container.
//
// Through the shell's own redirection rather than `docker cp`, which wants a
// tar stream and a temporary file on this side. The path arrives as $0 rather
// than inside the script, so a name with a quote or a space in it is a name and
// not an injection — this takes paths an agent chose.
func WriteFile(ctx context.Context, name, path string, content []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no path")
	}
	_, err := runIn(ctx, fileWait, bytes.NewReader(content),
		"exec", "--interactive", name,
		"sh", "-c", `mkdir -p "$(dirname "$0")" && cat > "$0"`, path)
	return err
}

// ReadFile reads a file out of a container.
func ReadFile(ctx context.Context, name, path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("no path")
	}
	out, err := run(ctx, fileWait, "exec", name, "sh", "-c", `cat -- "$0"`, path)
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return nil, err
	}
	return []byte(out), nil
}

// fileWait bounds a single file transfer. Generous for a large file over a
// pipe, short of letting one hold a request open.
const fileWait = 2 * time.Minute

// Running is the names of the running containers whose names start with a
// prefix.
//
// For tidying up after a restart. This process tracks which machines are in use
// in memory, so a container left running by a previous one is invisible to it —
// and invisible is the same as leaked when the thing being held is memory.
func Running(ctx context.Context, prefix string) ([]string, error) {
	out, err := run(ctx, probeWait, "ps", "--filter", "name="+prefix, "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// Stop stops a container, leaving its volume alone.
func Stop(ctx context.Context, name string) error {
	_, err := run(ctx, probeWait, "stop", "--time", "5", name)
	return err
}

// Remove destroys a container. The volume survives it: throwing away the
// machine is how you recover from breaking it, and it is not how you throw away
// your files. Removing those is the caller's own verb.
func Remove(ctx context.Context, name string) error {
	_, err := run(ctx, probeWait, "rm", "--force", name)
	return err
}

// RemoveVolume destroys a caller's files. Separate from Remove, and named for
// what it loses.
func RemoveVolume(ctx context.Context, volume string) error {
	_, err := run(ctx, probeWait, "volume", "rm", "--force", volume)
	return err
}

// run is the one place an argv is built and a subprocess is started.
//
// Everything this package does goes through here on purpose. It is what makes
// "nothing ever mounts the docker socket" a property somebody can check by
// reading one function rather than by auditing a package.
func run(ctx context.Context, wait time.Duration, args ...string) (string, error) {
	return runIn(ctx, wait, nil, args...)
}

// runIn is run with something on the command's standard input.
func runIn(ctx context.Context, wait time.Duration, stdin io.Reader, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = stdin
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := out.String()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return text, fmt.Errorf("timed out after %s", wait)
		}
		// Wrapped, not formatted. The caller has to be able to tell "the
		// command ran and exited non-zero" from "the command could not be run",
		// and those are the same string until one of them is an *exec.ExitError
		// somebody can still reach.
		return text, fmt.Errorf("%w: %s", err, strings.TrimSpace(text))
	}
	return text, nil
}
