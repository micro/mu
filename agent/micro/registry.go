package micro

// The agent this instance provides.
//
// One. There were eleven — News, Markets, Weather, Places, Video, Social, Mail,
// Apps, Faith, Search and this one — each with its own system prompt and its
// own slice of the tool list, reachable at agent+news@ and routed to by
// keyword.
//
// They were ten tools wearing coats. The distinction between "the news agent"
// and "Micro with the news tools" is a distinction the caller cannot use: you
// do not know in advance which specialist your question belongs to, and if you
// did you would have asked the tool. What the split actually bought was a
// router that had to guess, ten prompts to keep in step with each other, and
// ten rows on a page saying almost the same thing.
//
// Micro has every tool and no scope, which is what it always had. The other ten
// were the same tools behind a name, and the name was the only difference.
//
// # Why Code is not an eleventh coat
//
// Because the difference is a job rather than a topic. The ten were sorted by
// what a question was about, which is the thing a caller cannot reliably know
// in advance — you do not know whether "will it rain on my flight" is weather
// or flights, and if you did you would have called the tool.
//
// Code is sorted by what you are doing. You know whether you are building
// something, the way you know whether you are writing an email, and nothing has
// to guess on your behalf. Its prompt changes behaviour rather than subject —
// it writes files and checks them instead of answering — and its scope is a
// real narrowing: given every tool it wanders, given a machine it builds. That
// is a second agent worth having, and the test for a third is the same one.
//
// The machinery stays: Register, Route and Execute are how an agent with a
// scope is run, and an account's own agents are exactly that — a name, an
// instruction, and the services it may use. What is gone is this instance
// shipping ten of them nobody asked for.
func init() {
	Register(&Agent{
		ID:           "micro",
		Name:         "Micro",
		Description:  "General-purpose personal AI — handles any query",
		SystemPrompt: `You are Micro, a personal AI assistant. You have access to all tools and can help with anything — news, markets, weather, mail, search, places, apps, and more. Be concise, direct, and helpful. Use markdown.`,
		Tools:        nil, // nil = all tools
		MemoryScope:  "",
		Examples:     []string{"Give me a morning brief", "What is moving in markets?", "Weather in San Francisco", "Find today's AI news"},
	})

	Register(&Agent{
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
	})
}
