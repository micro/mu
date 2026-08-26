// Command mu is the whole product in one binary: a server, and a CLI that
// talks to one.
//
// `mu --serve` runs the instance — every service, the web app, /mcp, the SMTP
// server. Anything else is a CLI command, handed to internal/cli, which speaks
// to a running instance over HTTP and never touches server state.
//
// This file used to be 2,973 lines, almost all of it inside main(): service
// loads, cross-package wiring, 46 tool registrations, 120 routes and the
// middleware chain. It is internal/server now, split by concern. What belongs
// in a command is here: the flags, and which of the two programs to run.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"mu/internal/cli"
	"mu/internal/quota"
	"mu/internal/server"
)

// quota.json is what this instance charges for and how much: one entry per
// operation, with the label the cost tables show and the environment variable
// that overrides it.
//
// It is embedded here, in the command, rather than inside the package that
// reads it. internal/quota answers what something costs; where the answer comes
// from is a decision about how this program is configured, and configuration is
// what a command is for. An operator can also drop a quota.json in the data
// directory to change entries without rebuilding — see internal/quota/pricing.go.
//
//go:embed quota.json
var quotaConfig []byte

var EnvFlag = flag.String("env", "dev", "Set the environment")
var ServeFlag = flag.Bool("serve", false, "Run the server")
var AddressFlag = flag.String("address", ":8080", "Address for server")

func main() {
	// Before anything can be charged for. An operation with no price falls back
	// to a flat credit, so this failing quietly would mean an instance that
	// bills a cent for everything.
	if err := quota.Load(quotaConfig); err != nil {
		fmt.Fprintln(os.Stderr, "quota.json is not valid JSON:", err)
		os.Exit(1)
	}

	// Server vs CLI dispatch — any invocation that includes `--serve`
	// (or `-serve`) runs the full server. Anything else is treated as a CLI
	// command and handed to the cli package, which talks to /mcp over HTTP.
	if !isServerMode(os.Args[1:]) {
		os.Exit(cli.Run(os.Args[1:]))
	}

	flag.Parse()

	if !*ServeFlag {
		fmt.Println("--serve not set")
		return
	}

	server.Env = *EnvFlag
	server.Run(*AddressFlag)
}

// isServerMode returns true when the argument list contains the `--serve`
// flag. This is the single signal that switches between the server and CLI
// entry points — kept deliberately simple so it can't accidentally divert the
// production deployment.
func isServerMode(args []string) bool {
	for _, a := range args {
		if a == "--serve" || a == "-serve" {
			return true
		}
		// Allow `--serve=true` / `--serve=false` for completeness.
		if strings.HasPrefix(a, "--serve=") || strings.HasPrefix(a, "-serve=") {
			return true
		}
	}
	return false
}
