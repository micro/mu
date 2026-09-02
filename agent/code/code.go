package code

// The Code agent: the one that builds things.
//
// # Why it has a package and Micro does not
//
// Micro is a name, an instruction and every tool, and there is nothing else to
// say about it — a struct literal in a registry is the whole of what it is.
// This one is not that. Ask it for a tip calculator and what comes back is not
// a paragraph: it is a file on a Debian machine that keeps existing after the
// conversation ends, and an app hosted at an address somebody else can open.
// Those are things, they belong to an account, and something has to compose
// them — the machine from service/shell, the hosting from service/apps and the
// conversation from agent/ — into one place a person can look at.
//
// A composer has to sit above the things it composes, which is why this is not
// in either service. apps.AuthoredBy says so in its own comment and was
// exported for this page, which had not been written: /code was linked from two
// places on /apps as the way to build an app, was gated in the route table, and
// no handler ever claimed it — so the primary call to action of the apps
// section fell through to the catch-all and rendered the front page.
//
// # What was here before
//
// A struct literal in agent/micro/registry.go beside Micro's, and nothing else.
// That is the version of this agent that could be described as a prompt with a
// name, and listing it at /agent/code while it had no more substance than that
// is the thing this package answers.
//
// The definition stays the same shape and is still registered into the same
// registry, because how an agent is run is agent/micro's job and this is not a
// second mechanism. What is added is what the definition could never carry: a
// front door, a view of the workspace, and the eval that says whether any of it
// works — see ./eval, which measures this agent and now lives beside it.

import "mu/agent/micro"

// ID is what the registry files it under, and Path is where it answers.
//
// Written down because three other packages spell them: the route that
// redirects /code, the links on /apps, and the eval. A string literal in four
// files is three chances to rename one of them.
const (
	ID   = "code"
	Path = "/agent/" + ID
)

// Agent is this instance's Code agent.
//
// A var rather than an anonymous literal inside init, so the page below and the
// eval can read the prompt and the tool scope they are testing rather than
// asserting against a copy.
var Agent = &micro.Agent{

	ID:          "code",
	Name:        "Code",
	Description: "Builds things on a machine of your own — writes the files, runs them, hosts the result",
	// Short, and ordered by what actually costs time.
	//
	// This was four hundred words and its first instruction was "you work
	// the way somebody at a terminal works: write a file, run it, read what
	// it said, fix it" — a four-round-trip loop, stated as the job. Then
	// three hundred words of ls/grep/sed craft, with "a web app is one HTML
	// file" one sentence in the middle of it. A model that read that and
	// then ran ls to see where it was had followed the instruction.
	//
	// Every step is a model round trip of three to fifteen seconds, so the
	// number of them is the whole of how long a build takes. Writing one
	// file is one call. The budget goes first now, and the craft advice is
	// scoped to changing a file that already exists, which is the only job
	// it was ever about.
	SystemPrompt: `You are Code. You build things on a machine of your own: a Debian box where /work holds the files, and they stay there between messages.

Build first, and build in one call. Something new is one shell_write with the whole file in it — do not look around first, do not mkdir, do not call the plan tool, do not read the file back to check. You already know what you were asked for. Write it.

Changing a file you can hold in your head — a page, a script, anything of a few hundred lines — is also two: read it once with shell_run, then shell_write the whole thing back with the change in it. Do not sed a page. A substitution against markup full of quotes and slashes silently changes nothing, sed says it succeeded, and you find out a call later; three of those is a minute of somebody's time to edit one line.

Reach for grep and sed when the file is too big to write back, and not before.

A web app is one HTML file that stands alone — style, script and data inside it, nothing fetched from anywhere, no build step. Host it with the apps tool and say where it is.

shell_write takes a path under /work and a file's whole content. shell_run runs a command, for running what you built and for the jobs that are genuinely commands; its working directory carries over.

Say what you did in one sentence. Do not paste the file back; it is on the machine and nobody reads it twice.`,
	// A machine and somewhere to put what it makes. Not the whole tool list:
	// given all of them a run spends its attention deciding which of a
	// hundred things it does not want, and this one only ever needs two.
	Tools: []string{"shell", "apps"},
	// Its own, because what it learns is about somebody's projects — which
	// directory a thing lives in, what they call it, how they like it built
	// — and none of that belongs in the pool Micro answers from.
	MemoryScope: "code",
	Examples: []string{
		"Build me a tip calculator",
		"Make the background white and the text dark",
		"Write a script that renames these files and run it",
	},
}

func init() { micro.Register(Agent) }
