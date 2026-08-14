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
  mu <service> <method> [--arg value ...]

COMMON COMMANDS
  mu news                        Latest news feed
  mu news search "ai safety"     Search news
  mu blog list                   List blog posts
  mu chat "hello"                Chat with the AI
  mu agent "what is the btc price?"
  mu web search "claude code"    Search the web
  mu weather forecast --lat 51.5 --lon -0.12
  mu apps search "pomodoro"      Search the apps directory
  mu key                         The key on this machine, and what it holds
  mu x402                        This instance's payment config
  mu wallet balance              What your wallet holds on the server

MANAGEMENT
  mu login                       Log in by pasting a token from /token
  mu logout                      Forget the saved token
  mu config get|set|path         Manage the config file
  mu help [tool]                 Full tool list, or help for a tool

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

EXAMPLES
  mu markets list --category stocks
  mu news search --query "bitcoin" --table
  mu blog create --title "Hi" --content "..."
  mu apps build --prompt "an expense tracker"

Tool names are two words: the service, then what to do with it. The
underscore form works too, so mu news list and mu news_list are the same call.
`

// printShortHelp prints the summary help text.
func printShortHelp(w io.Writer) {
	fmt.Fprint(w, shortHelp)
}

// runHelp handles `mu help` and `mu help <tool>`.
func runHelp(args []string, rc *ResolvedConfig) int {
	if len(args) == 0 {
		return runToolList(rc)
	}
	// `mu help news list` names one tool in two words.
	return runToolHelp(strings.Join(args, " "), rc)
}

// commandName is how a tool is typed: the service, a space, then the method.
// news_list is an identifier; "news list" is how a person says it.
func commandName(tool string) string {
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
	fmt.Println("works too: mu news list and mu news_list are the same call.")
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
