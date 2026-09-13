// Help text. The tool list is fetched live from the server so any new
// MCP tool automatically shows up.
package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const shortHelp = `mu — command line for the Mu platform

USAGE
  mu <command> [flags]
  mu <capability> <operation> [--arg value ...]

START HERE
  mu login [url]                 Sign in to an instance (default: https://micro.mu)
  mu tools                       Everything this instance can do
  mu ask "what is on my calendar?"
                                 Ask the agent on the instance — it has your
                                 mail, notes and internal service tools

TALKING TO AN AGENT
  Two commands, and the difference is worth a sentence: "ask" talks to the
  agent you made on an instance; "agent" runs one on this machine that rents
  tools from an instance. Same word in English, opposite directions.

  mu ask "summarise my unread mail"
  mu agent "what is the btc price?"

COMMON COMMANDS
  mu agent_list                  Agents you can ask
  mu work submit --prompt "Research the options"
                                 Delegate a goal to run in the background
  mu work list                   Delegated jobs and their state
  mu work get --id WORK_ID        Read the outcome
  mu inbox list                  Recent conversations and updates
  mu inbox read --id THREAD_ID    Read a conversation

MANAGEMENT
  mu setup                       Configure a model and keys for "mu agent"
  mu logout                      Forget the saved token
  mu config get|set|path         Show which instance is being called, or change it
  mu x402                        Paying per call: config, and this machine's key
  mu tools                       Public operations, grouped by capability
  mu help <tool>                 Parameters for one tool
  mu version                     Which build this is

FLAGS (any command)
  --url URL        Mu instance URL (env: MU_URL, default: https://micro.mu)
  --token TOKEN    Session or PAT token (env: MU_TOKEN)
  --pretty         Force pretty-printed output
  --raw            Force raw JSON output
  --table          Render list results as a text table
  -v, --verbose    Verbose logging

CONFIG
  Loaded from $XDG_CONFIG_HOME/mu/config.json (default: ~/.config/mu/config.json).
  Environment variables MU_URL and MU_TOKEN override the config file.
  Command-line flags override both.

  Calls go to https://micro.mu unless you say otherwise. Running your own
  instance? "mu login https://your.host" points everything at it for good, and
  "mu config get" says which one is in use and what decided that.

EXAMPLES
  mu ask --agent research "What needs my attention?"
  mu work submit --prompt "Compare these options and recommend one"
  mu work get --id WORK_ID
  mu inbox read --id THREAD_ID

Operation names can be two words: mu work list and mu work_list are the
same call. Agent operations use the underscore form, such as mu agent_list,
because mu agent runs a local agent. Service commands require a separate
tools host; they are not exposed by the primary Micro API.

`

// printShortHelp prints the summary help text.
func printShortHelp(w io.Writer) {
	fmt.Fprint(w, shortHelp)
}

// runHelp handles `mu help`, `mu help tools` and `mu help <tool>`.
//
// Bare help is about the program: how to sign in, how to reach the agent, what
// the flags are. It used to be the tool list — two hundred lines of catalogue,
// fetched from the server — and the summary that says how to *log in* was
// printed only when the fetch failed. So the half somebody needs first was
// reachable only when something was already broken, and the working case
// scrolled the answer off the top of the terminal.
//
// The catalogue is a command, because that is what it is: a question about the
// instance rather than about the binary.
func runHelp(args []string, rc *ResolvedConfig) int {
	if len(args) == 0 {
		printShortHelp(os.Stdout)
		return 0
	}
	if len(args) == 1 && (args[0] == "tools" || args[0] == "--all") {
		return runToolList(rc)
	}
	// `mu help news list` names one tool in two words.
	return runToolHelp(strings.Join(args, " "), rc)
}

// commandName is how a tool is typed: the service, a space, then the method.
// news_list is an identifier; "news list" is how a person says it.
func commandName(tool string) string {
	if strings.HasPrefix(tool, "agent_") {
		return tool
	}
	return strings.Replace(tool, "_", " ", 1)
}

// runToolList fetches tools/list and prints a grouped summary.
func runToolList(rc *ResolvedConfig) int {
	if err := rc.Validate(); err != nil {
		printShortHelp(os.Stdout)
		return 0
	}
	client := NewClient(rc)
	tools, err := client.ListTools()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to fetch tools:", err)
		printShortHelp(os.Stdout)
		return 1
	}

	// Group by prefix (chars before first underscore). Tools without
	// an underscore go into a "general" bucket.
	groups := map[string][]Tool{}
	for _, t := range tools {
		prefix := "general"
		if idx := strings.Index(t.Name, "_"); idx > 0 {
			prefix = t.Name[:idx]
		}
		groups[prefix] = append(groups[prefix], t)
	}

	var prefixes []string
	for p := range groups {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	fmt.Println("Available tools on", rc.URL)
	fmt.Println()
	for _, p := range prefixes {
		fmt.Printf("# %s\n", p)
		list := groups[p]
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		for _, t := range list {
			desc := t.Description
			if len(desc) > 72 {
				desc = desc[:69] + "..."
			}
			fmt.Printf("  mu %-25s  %s\n", commandName(t.Name), desc)
		}
		fmt.Println()
	}
	fmt.Println("Run `mu help <tool>` for parameter details. The underscore form")
	fmt.Println("works too: mu work list and mu work_list are the same call.")
	return 0
}

// runToolHelp prints parameter details for a single tool.
func runToolHelp(name string, rc *ResolvedConfig) int {
	if err := rc.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	client := NewClient(rc)
	tools, err := client.ListTools()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to fetch tools:", err)
		return 1
	}
	// "mu help news list" arrives here as "news list" — the same tool as
	// "news_list", so match either spelling.
	wanted := strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
	var tool *Tool
	for i := range tools {
		if tools[i].Name == wanted {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		fmt.Fprintf(os.Stderr, "unknown tool: %s\n", name)
		return 1
	}

	fmt.Printf("mu %s — %s\n\n", commandName(tool.Name), tool.Description)

	required := map[string]bool{}
	for _, k := range tool.InputSchema.Required {
		required[k] = true
	}

	if len(tool.InputSchema.Properties) == 0 {
		fmt.Println("(no parameters)")
		return 0
	}

	var names []string
	for n := range tool.InputSchema.Properties {
		names = append(names, n)
	}
	sort.Strings(names)

	fmt.Println("PARAMETERS")
	for _, n := range names {
		f := tool.InputSchema.Properties[n]
		req := ""
		if required[n] {
			req = " (required)"
		}
		fmt.Printf("  --%-16s %-8s %s%s\n", n, f.Type, f.Description, req)
	}
	fmt.Println()
	fmt.Printf("EXAMPLE\n  mu %s", commandName(tool.Name))
	for _, n := range names {
		if required[n] {
			fmt.Printf(` --%s "..."`, n)
		}
	}
	fmt.Println()
	return 0
}
