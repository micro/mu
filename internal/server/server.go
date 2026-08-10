package server

import "mu/internal/api"

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
// they are registered before anything asks for them; hooks are wired before
// tools, because a tool may be handed a hook at registration; routes come after
// tools, since a route can serve a page built from the registry.
func Run(addr string) {
	boot()
	wireHooks()
	registerTools()

	// Anything declared on a Spec but not written out by hand becomes a tool
	// here. Six endpoints had drifted out of reach this way — see
	// internal/api/derive.go. Hand-written registrations win, so this only
	// fills gaps and has to run after all of them.
	api.DeriveTools()

	// Every tool is registered. Surfaces that publish a command set built from
	// the registry — the Discord slash commands, the Telegram menu — are
	// waiting on this; without it they race the wiring and publish a partial
	// one.
	api.ToolsRegistered()

	registerRoutes()
	serve(addr)
}

// Env is the environment name from the --env flag, set by main before Run.
//
// It relaxes CORS in "dev" and nothing else. A package variable rather than a
// flag read from in here, because internal/server is a library: flags belong to
// the command that owns the process.
var Env = "dev"
