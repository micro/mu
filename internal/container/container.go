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
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Available reports whether this machine can run a container.
//
// Both halves, because they fail differently and only one of them is the
// operator's fault: the CLI may be absent, or present with no daemon behind it.
//
// A success is cached for the life of the process; a failure is retried. That
// asymmetry is the whole point. This used to be one sync.Once, so the first
// answer stood forever — an instance that started before Docker was installed,
// or before its user was added to the docker group, or before the daemon came
// up, went on saying there was no Docker for as long as it ran. The operator
// fixes the machine, watches nothing change, and has no reason to guess that
// the fix is to restart a web server.
//
// Retried at an interval rather than per call, because the probe forks docker
// and waits, and the pages that ask this are ordinary pages.
func Available() bool {
	probe()
	return available
}

// probe fills in the host facts, retrying while there are none.
func probe() {
	hostMu.Lock()
	defer hostMu.Unlock()
	if available {
		return
	}
	if !lastProbe.IsZero() && time.Since(lastProbe) < retryEvery {
		return
	}
	lastProbe = time.Now()
	hostFacts()
}

// Reason is why there is no container runtime, for a page to show. Empty when
// there is one.
//
// Because the two ways of not having one need different things done about them
// and look identical from here. "Install Docker" is the wrong instruction for a
// machine that has Docker running and a server process that cannot reach its
// socket — which is the common one, since the socket is root-owned and mode 660
// and nothing puts a service account in the docker group for you.
func Reason() string {
	probe()
	return unreachable
}

var (
	hostMu      sync.Mutex
	lastProbe   time.Time
	available   bool
	unreachable string
	hostMemory  int64
	hostCPUs    int
)

// retryEvery is how often a machine with no runtime is asked again.
//
// Long enough that a page load does not fork docker, short enough that an
// operator who has just installed it does not conclude it made no difference.
const retryEvery = 30 * time.Second

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
	probe()
	return hostMemory
}

// hostFacts asks the daemon whether it is there and what the machine has.
//
// One probe for both, because "is there a runtime" and "how big is it" are the
// same question asked of the same subprocess.
//
// # Why the exit code is not the answer
//
// This checked `docker info`'s exit status and nothing else, on the reasoning
// that a client with no daemon behind it fails. Some versions of the CLI do
// exit non-zero; others print the error and exit 0, and the deployed instance
// had one of those. So the probe passed, every method sailed through the gate,
// and the first sign of trouble was a raw socket path in an agent's face.
//
// The daemon's own version is the fact that cannot be faked by a client on its
// own: it renders empty unless something answered. Anything that does not put
// three fields on a line is a failure whatever it exited with, and what it said
// instead becomes Reason.
func hostFacts() {
	if _, err := exec.LookPath(binary); err != nil {
		// What was actually tested, which is not what this used to claim.
		//
		// LookPath searching PATH and finding nothing does not mean Docker is
		// absent — it means this process cannot see it. A service started by
		// systemd gets a short PATH that does not include /snap/bin, so a
		// snap-installed Docker is invisible to it while being perfectly
		// present on the machine. Saying "no docker installed" to an operator
		// who can type `docker ps` sends them to argue with the wrong thing.
		//
		// So the PATH is in the message. It is the fact that distinguishes the
		// two, and the operator has it in front of them either way.
		unreachable = "no docker on this server's PATH (" + os.Getenv("PATH") + ") — " +
			"it may not be installed, or may not be on the PATH this process was " +
			"started with"
		return
	}
	out, err := run(context.Background(), probeWait,
		"info", "--format", "{{.ServerVersion}}|{{.MemTotal}}|{{.NCPU}}")

	if _, mem, cpus, ok := readFacts(out); ok {
		// And the old reason goes. Without this the retry above only half
		// works: an instance that started before Docker was up starts saying
		// yes to Available while Reason still reads "no docker on this
		// server's PATH", so a page can report a runtime that is working and
		// explain why there isn't one, in the same breath.
		hostMemory, hostCPUs, available, unreachable = mem, cpus, true, ""
		return
	}
	unreachable = whyNot(out, err)
}

// missingShell reports whether an exec failed because the shell is not there.
//
// Matched on the runtime's words because that is all there is: docker exec
// returns 126 for several unrelated things, and the distinction being made here
// is narrow enough that a wrong guess would silently rerun a command that had
// really failed.
func missingShell(out string) bool {
	return strings.Contains(out, "executable file not found")
}

// readFacts pulls the daemon's answer out of a probe's output.
//
// Scanned line by line rather than parsed whole, because run combines stdout
// and stderr: a warning, or the error itself, may share the output with the
// answer. A line with three fields and a version in the first is the daemon
// having replied, and nothing else counts however the command exited.
func readFacts(out string) (version string, memory int64, cpus int, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) != 3 || parts[0] == "" {
			continue
		}
		memory, _ = strconv.ParseInt(parts[1], 10, 64)
		cpus, _ = strconv.Atoi(parts[2])
		return parts[0], memory, cpus, true
	}
	return "", 0, 0, false
}

// whyNot turns a failed probe into something an admin can act on.
func whyNot(out string, err error) string {
	text := strings.ToLower(out)
	if err != nil {
		text += " " + strings.ToLower(err.Error())
	}
	switch {
	case strings.Contains(text, "permission denied"):
		return "this server cannot reach the container runtime's socket — it is running " +
			"as a user that is not in the docker group. Adding it and restarting the " +
			"server is the fix; the group is read once when a process starts, so the " +
			"restart is not optional"
	case daemonDown(text):
		return "docker is installed but its daemon is not running"
	case trimLine(out) != "":
		return "the container runtime did not answer: " + trimLine(out)
	}
	return "the container runtime did not answer"
}

// daemonDown is the runtime's several ways of saying the client is fine and
// nothing answered it.
//
// More than one phrasing because the phrasing belongs to the CLI and the CLI
// changes it. This was written against "Cannot connect to the Docker daemon at
// unix:///var/run/docker.sock. Is the docker daemon running?"; a current client
// says "failed to connect to the docker API at unix:///var/run/docker.sock;
// check if the path is correct and if the daemon is running" instead, which
// shares no matched substring with it — not "cannot connect", not "is the
// docker daemon running".
//
// So the client whose words moved fell past this case to the one below, and an
// operator with a stopped daemon was told "the container runtime did not
// answer: |0|0". That is the probe's own template rendering itself empty. It
// names nothing to start and nothing to look at, which is the failure this
// package's messages exist to avoid.
func daemonDown(text string) bool {
	for _, phrase := range []string{
		"cannot connect", "failed to connect", "connection refused",
		"daemon is running", "docker.sock",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// trimLine is the first useful line of a command's output, bounded.
//
// Useful excludes our own probe template coming back. run combines stdout and
// stderr, so a failed probe's output holds both the runtime's complaint and the
// template rendering itself empty — and the template is first, because it is on
// stdout. Quoting that at an operator explains nothing: "|0|0" is this file
// talking to itself.
func trimLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if _, _, _, ours := readFacts(line); ours || strings.Count(line, "|") == 2 {
			continue
		}
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 200 {
				line = line[:200] + "…"
			}
			return line
		}
	}
	return ""
}

// HostCPUs is how many cores the daemon has, or 0 if it will not say.
func HostCPUs() int {
	probe()
	return hostCPUs
}

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
func Start(ctx context.Context, name, image, volume string, l Limits, mounts ...string) error {
	if !Available() {
		return fmt.Errorf("%s", Reason())
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
	argv := []string{
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
		"--volume", volume + ":/work",
		"--workdir", "/work",
	}
	// Anything else the caller wants in the box, in docker's own --volume
	// syntax. Variadic, so every existing call site is unchanged, and a
	// mechanism rather than a policy: this package has no opinion about what
	// belongs in a container, only about how one is made.
	for _, m := range mounts {
		if strings.TrimSpace(m) != "" {
			argv = append(argv, "--volume", m)
		}
	}
	argv = append(argv, image,
		// Something that does not exit. The container is a place to exec into;
		// its own process is not the work.
		"sleep", "infinity",
	)

	_, err := run(ctx, startWait, argv...)
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

// Run is one command to run somewhere.
//
// A struct rather than six positional arguments, and User is why it exists: a
// shared container serves more than one caller, and which Unix user a command
// runs as is the only thing separating them. See service/shell's shared mode.
type Run struct {
	Name    string // the container
	Command string // a shell command
	Dir     string // where to run it
	User    string // uid[:gid]; empty is the image's own user, which is root
	Wait    time.Duration
	// TTY allocates a terminal, for an interactive session. Set by Shell and
	// not by callers of Exec: a command run for its output does not want one,
	// and asking for one merges stderr into stdout at the pty rather than in
	// the code that knows why.
	TTY bool
	// Shell is which shell runs Command. Empty is "sh", which is what a
	// container is guaranteed to have.
	//
	// It matters because sh is not one thing. On Debian it is dash, so an image
	// with bash installed still runs bash-flavoured commands under something
	// that is not bash — and the caller who chose that image did so precisely
	// to get bash. Naming it here keeps that a caller's decision rather than a
	// property of whichever base image somebody happens to be on.
	Shell string
	// Env is set on this exec and on nothing else.
	//
	// The distinction is the whole point and is a security property rather
	// than a convenience: a variable given to the *container* is visible to
	// everything that ever runs in it, including code an agent fetched off the
	// internet and ran. A variable given to one exec belongs to that exec.
	//
	// So a credential may travel this way and must never travel the other. See
	// service/shell's SSH session, which is the only caller that sets one.
	Env map[string]string
}

// argv is the docker invocation up to the command itself.
//
// One place, so that "who is this running as" cannot be set in one path and
// forgotten in another — which is exactly the mistake that turns a shared
// container's isolation off without any visible symptom.
func (r Run) argv() []string {
	argv := []string{"exec", "--workdir", r.Dir}
	if r.User != "" {
		argv = append(argv, "--user", r.User)
	}
	// An interactive session is the same exec with a terminal on it. Through
	// here rather than assembled at the call site, for the reason above: the
	// --user flag is what separates two accounts in a shared container, and a
	// second place that builds a docker exec is a second place to forget it.
	if r.TTY {
		argv = append(argv, "--interactive", "--tty")
	}
	// Sorted, so the same Run produces the same command line. A map's order is
	// random and an argv that shuffles is one no test can assert on.
	if len(r.Env) > 0 {
		keys := make([]string, 0, len(r.Env))
		for k := range r.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			argv = append(argv, "--env", k+"="+r.Env[k])
		}
	}
	return append(argv, r.Name)
}

// Shell attaches a terminal to a container and runs an interactive shell.
//
// It hands back the process with its pipes already wired, rather than waiting
// for it: the caller is a protocol server copying bytes in both directions
// until somebody types exit, which is not a shape Exec can express.
//
// Everything the container was started with still applies — no capabilities,
// no new privileges, the memory, CPU and PID caps, and whatever network it was
// given. A shell is not a way past any of that; it is the same box with a
// person at the keyboard instead of an agent.
func Shell(ctx context.Context, r Run, in io.Reader, out io.Writer) (*Session, error) {
	if !Available() {
		return nil, fmt.Errorf("%s", Reason())
	}
	if r.Dir == "" {
		r.Dir = "/work"
	}
	r.TTY = true

	// A real terminal, because docker checks for one.
	//
	// `docker exec -t` calls isatty() on its own stdin and refuses a pipe:
	// "the input device is not a TTY". The first version of this handed the
	// SSH channel straight through and got exactly that. So the slave end goes
	// to docker, which is satisfied, and the master is what the bytes flow
	// over. See pty.go.
	tty, err := openPTY()
	if err != nil {
		return nil, err
	}

	// The shell is named rather than taken from the caller. A Run carries a
	// Command for Exec's benefit and letting it through here would make the
	// login shell somebody else's choice — which, for a session that is
	// supposed to be a person at a prompt, is a way to run one command as if
	// it were a login.
	cmd := exec.CommandContext(ctx, "docker", append(r.argv(), "sh", "-l")...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty.slave, tty.slave, tty.slave
	if err := cmd.Start(); err != nil {
		tty.close()
		return nil, err
	}
	// The slave is the child's now. Holding our copy open means the master
	// never reports EOF when the shell exits, and the session hangs on a
	// terminal nobody is attached to.
	tty.slave.Close()
	tty.slave = nil

	s := &Session{cmd: cmd, tty: tty}
	go func() { io.Copy(tty.master, in) }()  //nolint:errcheck — ends when the channel closes
	go func() { io.Copy(out, tty.master) }() //nolint:errcheck — ends when the shell exits
	return s, nil
}

// Session is a shell somebody is attached to.
type Session struct {
	cmd *exec.Cmd
	tty *pty
}

// Resize tells the shell how big the window is, so a full-screen program
// redraws when somebody drags the corner.
func (s *Session) Resize(rows, cols uint16) {
	if s != nil && rows > 0 && cols > 0 {
		s.tty.resize(rows, cols)
	}
}

// Wait blocks until the shell exits, then releases the terminal.
func (s *Session) Wait() error {
	err := s.cmd.Wait()
	s.tty.close()
	return err
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
func Exec(ctx context.Context, r Run) (Result, error) {
	if strings.TrimSpace(r.Command) == "" {
		return Result{}, fmt.Errorf("no command")
	}
	if r.Dir == "" {
		r.Dir = "/work"
	}
	sh := r.Shell
	if sh == "" {
		sh = "sh"
	}
	out, err := run(ctx, r.Wait, append(r.argv(), sh, "-c", r.Command)...)
	// A shell the image does not have is not a failed command, it is a failed
	// choice — and the caller cannot know what is in an image an operator set.
	// sh is the one thing every container has, so the ask degrades to it rather
	// than handing back a runtime error about a binary the caller never
	// mentioned. Only for the missing-binary case: a command that genuinely
	// failed must still fail.
	if sh != "sh" && missingShell(out) {
		out, err = run(ctx, r.Wait, append(r.argv(), "sh", "-c", r.Command)...)
	}
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
func WriteFile(ctx context.Context, r Run, path string, content []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no path")
	}
	argv := append(r.argv(), "sh", "-c", `mkdir -p "$(dirname "$0")" && cat > "$0"`, path)
	// --interactive has to reach docker before the container name, so it goes
	// in ahead of the shared argv rather than being appended to it.
	argv = append([]string{argv[0], "--interactive"}, argv[1:]...)
	_, err := runIn(ctx, fileWait, bytes.NewReader(content), argv...)
	return err
}

// ReadFile reads a file out of a container.
func ReadFile(ctx context.Context, r Run, path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("no path")
	}
	out, err := run(ctx, fileWait, append(r.argv(), "sh", "-c", `cat -- "$0"`, path)...)
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
