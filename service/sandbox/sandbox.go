// Package sandbox is a machine of your own: somewhere an agent can run things.
//
// Every other service answers a question. This one hands over a computer — a
// filesystem that persists, a shell, and whatever the image has in it — and
// lets the caller decide what to do with it. That is a different kind of thing
// to put in a catalogue, and it is worth being explicit about why it belongs
// here rather than being a property of how agents run.
//
// # Why a service and not a container around the agent
//
// The obvious reading of "sandbox the agents" is to put each agent run inside a
// container. That buys nothing today: the agent is a goroutine that calls tools
// over MCP and has no filesystem to contain, so the blast radius is the tool
// surface and not the disk. The moment it gets a shell, containment starts to
// matter — and if the only route to a filesystem is this service, the container
// behind these tools *is* the containment. The agent stays where it is and the
// dangerous half lives on the other side of a tool call.
//
// It also keeps the layering the right way up. An agent consumes tools, tools
// derive from services, a service owns the thing. Putting the agent inside the
// box would mean the thing that reads the catalogue is itself an entry in it.
//
// # What one caller gets
//
// One container, named for them, started when they first ask and left running.
// A volume mounted at /work that outlives the container, because throwing away
// a broken machine is how you recover and it is not how you throw away your
// files. No capabilities, no new privileges, a memory cap, a CPU cap and a
// process cap — see internal/container, where the argv is built in one place so
// that "it never mounts the docker socket" is checkable by reading one function.
//
// # The network is on, and that is a decision
//
// A machine that cannot fetch a dependency or push a branch cannot do the thing
// this was built for. So it has one by default, and SANDBOX_NETWORK=none is
// there for an operator who wants the other trade. Say it plainly rather than
// implying isolation this does not have: what is bounded is the host, not the
// internet.
//
// The sharper edge is not the network, it is what a caller leaves lying in
// /work. An agent given a token to push with is an agent that can push, and
// nothing here can tell a good push from a bad one. That is the caller's
// decision to make deliberately — a scoped token and a protected branch — and
// this package's job is to say so rather than to pretend it has an answer.
//
// # It needs Docker, and says so without it
//
// The same shape as service/browser and Chromium: an instance with no container
// runtime serves a page that explains what is missing, and every method refuses
// with a sentence an operator can act on. Nothing about building or running Mu
// requires it.
package sandbox

import (
	"context"
	"fmt"
	"time"

	"mu/internal/app"
	"mu/internal/container"
	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the service handler.
type Server struct{}

// Configured reports whether this instance can run anything.
//
// Named for the question rather than for Docker, because the runtime is an
// implementation detail the caller should not have to know the name of — the
// same reason browser.Configured() covers three different ways of having a
// browser.
func Configured() bool { return container.Available() }

// ── Run ─────────────────────────────────────────────────────────

type RunRequest struct {
	Command string `json:"command" required:"true" description:"A shell command to run, e.g. 'go test ./... 2>&1 | tail -40'"`
	Dir     string `json:"dir" description:"Directory to run it in, under /work. Defaults to /work"`
	Timeout int    `json:"timeout" description:"Seconds to allow, up to the instance's maximum. Defaults to 120"`
}

type RunResponse struct {
	Output string `json:"output" description:"Everything the command wrote, stdout and stderr interleaved as they happened"`
	Code   int    `json:"code" description:"The exit status. 0 is success; anything else is the command's own failure, not an error from this service"`
}

// Run runs a shell command on the caller's machine.
// @example {"command": "ls -la"}
func (Server) Run(ctx context.Context, req *RunRequest, rsp *RunResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	res, err := exec(ctx, who, req.Command, req.Dir, time.Duration(req.Timeout)*time.Second)
	if err != nil {
		return err
	}
	rsp.Output, rsp.Code = res.Out, res.Code
	return nil
}

// ── Write ───────────────────────────────────────────────────────

type WriteRequest struct {
	Path    string `json:"path" required:"true" description:"Where to put it, under /work. Missing directories are created"`
	Content string `json:"content" required:"true" description:"The whole file. This replaces what was there"`
}

type WriteResponse struct {
	Path  string `json:"path" description:"Where it went"`
	Bytes int    `json:"bytes" description:"How much was written"`
}

// Write puts a file on the caller's machine.
//
// Its own method rather than a shell redirection, because the caller is a model
// and a heredoc containing Go source with backticks and quotes in it is the
// thing models get wrong. The content arrives as a JSON string and reaches the
// file without going through a shell at all.
// @example {"path": "hello.go", "content": "package main\n"}
func (Server) Write(ctx context.Context, req *WriteRequest, rsp *WriteResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	path, err := under(req.Path)
	if err != nil {
		return err
	}
	if len(req.Content) > maxFile {
		return fmt.Errorf("that file is larger than this instance will take (%d bytes)", maxFile)
	}
	if err := ready(ctx, who); err != nil {
		return err
	}
	if err := container.WriteFile(ctx, boxOf(who), path, []byte(req.Content)); err != nil {
		return err
	}
	rsp.Path, rsp.Bytes = path, len(req.Content)
	return nil
}

// ── Read ────────────────────────────────────────────────────────

type ReadRequest struct {
	Path string `json:"path" required:"true" description:"The file to read, under /work"`
}

type ReadResponse struct {
	Path    string `json:"path" description:"What was read"`
	Content string `json:"content" description:"The file"`
}

// Read reads a file from the caller's machine.
// @example {"path": "hello.go"}
func (Server) Read(ctx context.Context, req *ReadRequest, rsp *ReadResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	path, err := under(req.Path)
	if err != nil {
		return err
	}
	if err := ready(ctx, who); err != nil {
		return err
	}
	b, err := container.ReadFile(ctx, boxOf(who), path)
	if err != nil {
		return err
	}
	if len(b) > maxFile {
		b = append(b[:maxFile], []byte("\n\n[…truncated]")...)
	}
	rsp.Path, rsp.Content = path, string(b)
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Dir string `json:"dir" description:"The directory to list, under /work. Defaults to /work"`
}

type ListResponse struct {
	Dir     string `json:"dir" description:"What was listed"`
	Entries string `json:"entries" description:"One line per entry, as ls -la writes it"`
}

// List is what is on the caller's machine, in a directory.
// @example {"dir": "."}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	dir, err := under(req.Dir)
	if err != nil {
		return err
	}
	if err := ready(ctx, who); err != nil {
		return err
	}
	// Not charged and not through Run, so it does not go through the meter: a
	// listing costs this instance nothing and a caller finding its way around
	// should not be billed for looking.
	res, err := container.Exec(ctx, boxOf(who), "ls -la -- "+quoted(dir), work, quickWait)
	if err != nil {
		return err
	}
	rsp.Dir, rsp.Entries = dir, res.Out
	return nil
}

// quickWait bounds the operations that are not the caller's own command.
const quickWait = 30 * time.Second

// maxFile bounds one file in either direction. A model reads what comes back
// and writes what goes in; neither has any business being megabytes.
const maxFile = 256 * 1024

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("sandbox", "service register failed: %v", err)
	}
	// Machines nobody is using cost memory and nothing else. See idle.go.
	//
	// The goroutine is started unconditionally and asks about the runtime from
	// inside it. Configured() shells out to the daemon and waits up to ten
	// seconds for an answer, so asking here would put that wait in the boot
	// path of every instance — including the ones with no daemon, which are the
	// ones that would wait the longest.
	go reap()
}

var Spec = service.Spec{
	Name:        "sandbox",
	Handler:     new(Server),
	Description: "A machine of your own: run commands, keep files, build things",
	Page:        "/sandbox",
	Icon:        "sandbox.svg",
	// One container and one volume per account, so there is no such thing as
	// an unattributed call here — and a caller with no account has nowhere for
	// its files to live.
	Scoped: true,
	Endpoints: map[string]service.Endpoint{
		"Run": {
			Cost:   quota.OpSandboxRun,
			Writes: true,
			Doc: "Run a shell command on your own machine and get back what it wrote. " +
				"A real shell in a container: pipes, redirection and && all work, the " +
				"working directory is /work and files there persist between calls. " +
				"Use it to build, test, run scripts and move files about. A non-zero " +
				"exit code comes back in the response rather than as an error, so read " +
				"the output — a failing build is an answer",
		},
		"Write": {
			Writes: true,
			Doc: "Put a file on your machine, creating any missing directories. Use " +
				"this rather than shell redirection for anything with quotes or " +
				"backticks in it, which is most source code",
		},
		"Read": {
			Doc: "Read a file from your machine",
		},
		"List": {
			Doc: "List a directory on your machine",
		},
	},
}
