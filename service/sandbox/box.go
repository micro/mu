package sandbox

// One machine per account: what it is called, what it may have, and the rules
// about what may be run on it.
//
// The rules are the service. internal/container knows how to start a container
// and nothing about who deserves one; everything about paths, limits,
// timeouts, charging and who is asking is here, in the same way service/sms
// holds the rules about who may be texted and internal/twilio holds the send.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"mu/internal/container"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/settings"
)

// caller resolves the authenticated account from call metadata.
//
// No owner field on any request, for the same reason notes has none: an
// argument is chosen by whoever makes the call and context metadata is not, and
// a machine with your files on it is yours.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use a sandbox")
	}
	if !Configured() {
		return "", fmt.Errorf("this instance has no container runtime, so it cannot " +
			"give you a machine — an admin installs Docker and restarts")
	}
	return id, nil
}

// boxOf is an account's container name, and volumeOf its files.
//
// The account id is in the name because a name somebody can read is worth
// having when they are looking at `docker ps` trying to work out whose process
// is spinning. The hash is what makes it correct: an id may contain characters
// docker will not take, two different ids may clean to the same string, and a
// collision here would hand one account another's files.
func boxOf(accountID string) string    { return namePrefix + slug(accountID) }
func volumeOf(accountID string) string { return "mu-work-" + slug(accountID) }

// namePrefix is what every machine this instance starts is called. One place,
// because the reaper finds them by it — see idle.go.
const namePrefix = "mu-sandbox-"

func slug(accountID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(accountID) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	sum := sha256.Sum256([]byte(accountID))
	clean := b.String()
	if len(clean) > 24 {
		clean = clean[:24]
	}
	if clean == "" {
		clean = "acct"
	}
	return clean + "-" + hex.EncodeToString(sum[:4])
}

// home is where an account's files are, and machineFor which container holds
// them. The two modes differ in exactly these two answers and nowhere else,
// which is what keeps shared mode from being a second copy of the service.
func home(accountID string) string {
	if shared() {
		return sharedHome(accountID)
	}
	return work
}

func machineFor(accountID string) string {
	if shared() {
		return poolOf(accountID)
	}
	return boxOf(accountID)
}

// runAs is the Unix user a command runs as: nobody in particular on a machine
// of your own, and the account's own uid on a shared one.
//
// The one place that decides. A shared container where one path sets the user
// and another forgets is a container with no isolation and no symptom — see
// container.Run.argv, which is the matching single place on the other side.
func runAs(accountID string) string {
	if !shared() {
		return ""
	}
	uid := strconv.Itoa(uidOf(accountID))
	return uid + ":" + uid
}

// ready makes sure the caller's machine is up.
//
// Called by every method rather than once at sign-in, because there is no
// sign-in here — a tool call arrives from an agent that may be the first thing
// this account has ever done, and the machine has to exist by the time the
// command runs. Starting one that is already running costs one inspect.
func ready(ctx context.Context, accountID string) error {
	if shared() {
		// No eviction here: the pool is sized to fit and its members serve
		// everybody, so there is never a machine to take away to make room.
		return readyShared(ctx, accountID)
	}
	name := boxOf(accountID)
	// Somewhere to put it, before starting anything. On a small box the
	// machines do not all fit, and the honest answer is to stop the one nobody
	// is using rather than to refuse the person who is here.
	room(ctx, name)
	if err := container.Start(ctx, name, image(), volumeOf(accountID), limits()); err != nil {
		return err
	}
	// Every path in goes through here, which is what makes this the one place
	// that has to record use. A machine touched only by Run would be reaped out
	// from under somebody reading and writing files all afternoon.
	touched(name)
	return nil
}

// exec runs the caller's own command on their machine.
//
// It does not charge. The tool door charges what the Endpoint declares, so a
// charge here would bill a tool call twice — which is what
// TestNoOperationIsChargedTwice exists to catch, and it caught this. The page
// does not go through the door, so the page charges itself: see paidRun, and
// /browser, which is split the same way for the same reason.
func exec(ctx context.Context, accountID, command, dir string, wait time.Duration) (container.Result, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return container.Result{}, fmt.Errorf("a command is required")
	}
	where, err := under(accountID, dir)
	if err != nil {
		return container.Result{}, err
	}
	if err := ready(ctx, accountID); err != nil {
		return container.Result{}, err
	}
	return container.Exec(ctx, container.Run{
		Name:    machineFor(accountID),
		Command: command,
		Dir:     where,
		User:    runAs(accountID),
		Wait:    allowed(wait),
	})
}

// paidRun is exec with the gate and the charge the door would have applied.
//
// For the page only. The page is not a way round the price — it does the work
// the tool does, so it costs what the tool costs.
func paidRun(ctx context.Context, accountID, command, dir string) (container.Result, error) {
	ok, _, cost, qerr := quota.CheckQuota(accountID, quota.OpSandboxRun)
	if qerr != nil {
		return container.Result{}, qerr
	}
	if !ok {
		return container.Result{}, fmt.Errorf("that would cost %d credits and this "+
			"account cannot cover it", cost)
	}
	res, err := exec(ctx, accountID, command, dir, 0)
	if err != nil {
		// Nothing ran, so nothing is owed. The failures that reach here are this
		// instance's — a daemon that went away, a container that would not start
		// — and charging for them would bill somebody for our outage.
		return container.Result{}, err
	}
	quota.Charge(accountID, quota.OpSandboxRun, map[string]interface{}{ //nolint:errcheck
		"command": trimTo(command, 200),
	})
	return res, nil
}

// under is a path inside the caller's own directory, or why it is not one.
//
// It took no account once, because there was one place files could be. On a
// shared pool there is one per caller, and a path check that still resolved
// against /work would let anybody name /work/<somebody-else> — which the file
// permissions would then refuse, with an error about permissions rather than
// about whose files those are. The permissions are the boundary; this is what
// makes the paths mean what they look like.
//
// Relative paths are resolved against the caller's home and absolute ones are
// taken as given, so /work/x still works on a private machine and is refused on
// a shared one, where the caller's home is /work/<slug>.
func under(accountID, p string) (string, error) {
	base := home(accountID)
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return base, nil
	}
	if !strings.HasPrefix(p, "/") {
		p = base + "/" + p
	}
	clean := path.Clean(p)
	if clean != base && !strings.HasPrefix(clean, base+"/") {
		return "", fmt.Errorf("%s is outside %s, which is where your files live", p, base)
	}
	return clean, nil
}

const work = "/work"

// allowed is how long a command may have: what was asked for, bounded by what
// this instance will give, and a sensible number when nothing was asked.
func allowed(asked time.Duration) time.Duration {
	max := time.Duration(number(settings.Get("SANDBOX_MAX_SECONDS"), 600)) * time.Second
	if asked <= 0 {
		asked = defaultWait
	}
	if asked > max {
		return max
	}
	return asked
}

// defaultWait is what a command gets when the caller did not say. Long enough
// for a test run, short enough that a mistake is not a hung request.
const defaultWait = 120 * time.Second

// image is what a machine is made of.
//
// A small default rather than a useful one, deliberately. Somebody who wants Go
// and git on it says so — SANDBOX_IMAGE — and an operator who has not thought
// about it does not silently get a gigabyte pulled onto their disk the first
// time an agent tries something.
func image() string {
	if set := strings.TrimSpace(settings.Get("SANDBOX_IMAGE")); set != "" {
		return set
	}
	return "alpine:3.20"
}

// limits are what one machine may have. Every one is an operator's decision,
// and none of them may be absent — see container.Limits.
func limits() container.Limits {
	network := strings.TrimSpace(settings.Get("SANDBOX_NETWORK"))
	if network == "" {
		network = "bridge"
	}
	return container.Limits{
		Memory:  text(settings.Get("SANDBOX_MEMORY"), defaultMemory()),
		CPUs:    text(settings.Get("SANDBOX_CPUS"), defaultCPUs()),
		PIDs:    number(settings.Get("SANDBOX_PIDS"), 512),
		Network: network,
	}
}

// defaultMemory is a share of what the host has, rather than a number.
//
// It was a flat 2g, which is the bug this is fixing. On a 2GB VM that is the
// whole machine: the container stays inside its own cgroup while taking every
// free page, and the host's OOM killer then picks the largest process — which
// is the Mu server, not the sandbox. A default that can kill the thing serving
// it is not a default.
//
// A quarter, so the server, the daemon and a second machine all still fit.
// Floored at 256m because nothing useful builds in less, and capped at the old
// 2g because past that the constraint stops being memory. An operator who wants
// a different answer sets SANDBOX_MEMORY and gets exactly it — this only decides
// what to do when nobody has said.
func defaultMemory() string {
	host := container.HostMemory()
	if host <= 0 {
		// The daemon would not say. The old flat default is the wrong guess on
		// a small box, so guess small: a machine that is too small says so the
		// first time somebody builds in it, and one that is too big takes the
		// server down.
		return "512m"
	}
	share := host / 4
	if share < 256*megabyte {
		share = 256 * megabyte
	}
	if share > 2048*megabyte {
		share = 2048 * megabyte
	}
	return strconv.FormatInt(share/megabyte, 10) + "m"
}

const megabyte = 1024 * 1024

// defaultCPUs leaves one core for everything that is not a sandbox.
//
// Same argument as the memory: a single-core box that hands its only core to a
// build has nothing left to answer requests with. On anything larger this is 1,
// which is what it always was.
func defaultCPUs() string {
	if container.HostCPUs() <= 1 {
		return "0.5"
	}
	return "1"
}

// machineBudget is how many machines may run at once.
//
// Not a queue and not a refusal: see room, which stops the idlest machine to
// make space. What this bounds is total memory, which is the cost the price on
// a command deliberately does not cover — a command is CPU, and an idle
// container is memory somebody stopped paying for the moment they walked away.
func machineBudget() int {
	if n := number(settings.Get("SANDBOX_MAX_MACHINES"), 0); n > 0 {
		return n
	}
	host := container.HostMemory()
	if host <= 0 {
		return 2
	}
	// Half the box for sandboxes, over what one of them takes. The other half
	// is the server, the daemon and the page cache.
	per := parseSize(limits().Memory)
	if per <= 0 {
		return 2
	}
	if n := int((host / 2) / per); n > 1 {
		return n
	}
	return 1
}

// parseSize reads docker's own size syntax back into bytes. Only the suffixes
// docker takes, because it is reading a value docker was given.
func parseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "g"):
		mult, s = 1024*megabyte, strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		mult, s = megabyte, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult, s = 1024, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "b"):
		s = strings.TrimSuffix(s, "b")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n * mult
}

// text and number take a setting's value rather than its key, so every key in
// this package is a literal in the source. docs/config_test.go scans for that
// literal to hold the configuration page honest in both directions, and a key
// reaching settings.Get as a variable is invisible to it — fairly, since it is
// invisible to a person with grep too.
func text(value, fallback string) string {
	if set := strings.TrimSpace(value); set != "" {
		return set
	}
	return fallback
}

func number(value string, fallback int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && n > 0 {
		return n
	}
	return fallback
}

// trimTo shortens a line for the usage record.
func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// quoted is a string a shell will take as one word, whatever is in it.
//
// Single quotes, with the only character they cannot hold spliced in. Used for
// paths an agent chose, which is the whole reason it is here.
func quoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
