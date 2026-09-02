package server

import (
	"io"
	"time"

	gmlogger "go-micro.dev/v6/logger"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/tool"
)

// The server half of the binary.
//
// main.go was 2,973 lines, nearly all of it one function: every service load,
// every cross-package hook, 46 hand-written tools, 120 routes, and the
// middleware chain, in a single main(). Nothing could be found in it, and the
// metering bugs that cost a day were sitting in the middle where two gates
// disagreed with each other twenty lines apart.
//
// Splitting it is not architecture, it is being able to read one concern at a
// time: boot, hooks, tools, routes, serve. What is left in package main is
// what a command is — flags, and which of the two programs to run.

// Run brings the instance up and serves until interrupted.
//
// The order is the one thing here that is not arbitrary. Services load first so
// they are registered before anything asks for them; hooks are wired next,
// because a service may be handed one before it is asked anything; the
// catalogue is built from the Specs after that; routes come last, since a route
// can serve a page built from the catalogue.
//
// There is no step that registers tools by hand. There was, and it was a
// thousand lines: every tool that could not be derived because its capability
// was not declared on a service. They all are now.
func Run(addr string) {
	// Before anything logs, because the point of it is that the log stops
	// going to the screen — a service that boots first and logs first would
	// otherwise print to the surface this is clearing. See
	// internal/app/logfile.go.
	//
	// The framework's logger is pointed at the same file from here rather than
	// from internal/app, which may not import it: app is the bottom of the
	// product and a dependency added there is a dependency everywhere.
	app.Quieten(func(w io.Writer) {
		if err := gmlogger.Init(gmlogger.WithOutput(w)); err != nil {
			app.Log("main", "could not redirect the framework log: %v", err)
		}
	})
	app.OpenLog()

	// Timed, per phase, because "the restart takes ages" is not answerable
	// without it. Boot is a tenth of a second on an empty data directory and
	// nobody deploys one of those — what scales is whatever reads what is on
	// disk, and until this was here the only way to find out which phase that
	// was is to guess. One log line per phase, at startup only.
	started := time.Now()
	phase := started

	boot()
	app.Log("main", "boot: services in %s", time.Since(phase).Round(time.Millisecond))
	phase = time.Now()

	wireHooks()
	app.Log("main", "boot: hooks in %s", time.Since(phase).Round(time.Millisecond))
	phase = time.Now()

	// The catalogue: everything declared on a Spec that was not written out by
	// hand above. Six endpoints had drifted out of reach before this existed —
	// see tool/derive.go. Hand-written registrations win, so this fills gaps and
	// has to run after all of them.
	//
	// It also announces that the registry is complete. Surfaces that publish a
	// command set built from it — the Discord slash commands, the Telegram menu
	// — are waiting on that; without it they race the wiring and publish a
	// partial one.
	tool.Load(service.Specs())
	app.Log("main", "boot: catalogue in %s", time.Since(phase).Round(time.Millisecond))
	phase = time.Now()

	registerRoutes()
	app.Log("main", "boot: routes in %s, ready in %s",
		time.Since(phase).Round(time.Millisecond), time.Since(started).Round(time.Millisecond))

	serve(addr)
}

// Env is the environment name from the --env flag, set by main before Run.
//
// It relaxes CORS in "dev" and nothing else. A package variable rather than a
// flag read from in here, because internal/server is a library: flags belong to
// the command that owns the process.
var Env = "dev"
