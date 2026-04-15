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
  mu <tool>    [--arg value ...]

COMMON COMMANDS
  mu news                        Latest news feed
  mu news_search "ai safety"     Search news
  mu blog_list                   List blog posts
  mu chat "hello"                Chat with the AI
  mu agent "what is the btc price?"
  mu web_search "claude code"    Search the web
  mu weather_forecast --lat 51.5 --lon -0.12
  mu apps_search "pomodoro"      Search the apps directory
  mu me                          Show your account

MANAGEMENT
  mu login                       Log in by pasting a token from /token
  mu logout                      Forget the saved token
  mu config get|set|path         Manage the config file
  mu help [tool]                 Full tool list, or help for a tool

FLAGS (any command)
  --url URL        Mu instance URL (env: MU_URL, default: https://mu.xyz)
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
  mu news | jq '.feed[0]'
  mu news_search --query "bitcoin" --table
  mu blog_create --title "Hi" --content "..."
  mu apps_build --prompt "a pomodoro timer with lap counter"
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
	return runToolHelp(args[0], rc)
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
			fmt.Printf("  %-28s  %s\n", t.Name, desc)
		}
		fmt.Println()
	}
	fmt.Println("Run `mu help <tool>` for parameter details.")
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
	var tool *Tool
	for i := range tools {
		if tools[i].Name == name {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		fmt.Fprintf(os.Stderr, "unknown tool: %s\n", name)
		return 1
	}

	fmt.Printf("%s — %s\n\n", tool.Name, tool.Description)

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
	fmt.Printf("EXAMPLE\n  mu %s", tool.Name)
	for _, n := range names {
		if required[n] {
			fmt.Printf(` --%s "..."`, n)
		}
	}
	fmt.Println()
	return 0
}
