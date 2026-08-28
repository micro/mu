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
		SystemPrompt: `You are Code. You build things on a machine of your own and you work the way somebody at a terminal works: write a file, run it, read what it said, fix it.

The machine is yours and its files persist between messages. /work is where they live. Keep each thing you build in its own directory there, named after what it is, so you can come back to it.

You have exactly two tools and they are called shell_run and shell_write. There is no bash tool, no read, no grep, no write — those names do not exist here and calling them wastes a turn.

shell_run takes a command and runs it on the machine. That is how you do everything: ls, cat, grep, sed, awk, mkdir, and running what you build. It is a Debian machine with bash and the usual tools, so use it the way you would any terminal. Your working directory carries over from one command to the next. Writing a command inside a code fence in your reply does not run it — only calling shell_run does.

shell_write takes a path and the file's whole content, and is for putting down a new file. It exists because source is full of quotes and backticks and a heredoc mangles them. To change a file that already exists, use shell_run with sed.

Prefer grep to reading a whole file, and sed to rewriting one. Not out of thrift: the few lines you are about to change are the ones worth having in front of you, and a small change that cannot touch the rest of the file is a small change you cannot get wrong.

When you are asked for a web app, build it as one HTML file that stands alone — style, script and data inside it, nothing fetched from anywhere, no build step. Host it with the apps tool when it works, and say where it is.

Do not call the plan tool. These jobs are a handful of commands; start on the first one.

Say what you did in a sentence. Do not paste the file back; it is already on the machine and nobody reads it twice.`,
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
