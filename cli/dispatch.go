// CLI dispatcher. Parses the command line, builds the MCP argument
// map, calls the tool, and formats the result.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Run is the entry point called from main.go. It receives argv[1:]
// (the program name has already been stripped) and returns an exit code.
//
// Dispatch rules:
//
//	mu                         → short help
//	mu help                    → full help (includes live tool list)
//	mu help <tool>             → per-tool help
//	mu login                   → interactive login
//	mu logout                  → clear token
//	mu config ...              → config management
//	mu <tool> [--flag value]   → call an MCP tool
func Run(args []string) int {
	if len(args) == 0 {
		printShortHelp(os.Stdout)
		return 0
	}

	// Pull out --url / --token / --pretty / --raw / --table / --verbose
	// that may appear anywhere and apply them to the resolved config.
	var rc ResolvedConfig
	positional := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--url":
			if i+1 < len(args) {
				rc.URL = args[i+1]
				i += 2
				continue
			}
		case "--token":
			if i+1 < len(args) {
				rc.Token = args[i+1]
				i += 2
				continue
			}
		case "--pretty":
			rc.Pretty = true
			i++
			continue
		case "--raw":
			rc.Raw = true
			i++
			continue
		case "--table":
			rc.Table = true
			i++
			continue
		case "-v", "--verbose":
			rc.Verbose = true
			i++
			continue
		}
		if strings.HasPrefix(a, "--url=") {
			rc.URL = strings.TrimPrefix(a, "--url=")
			i++
			continue
		}
		if strings.HasPrefix(a, "--token=") {
			rc.Token = strings.TrimPrefix(a, "--token=")
			i++
			continue
		}
		positional = append(positional, a)
		i++
	}

	file, _ := LoadConfig()
	rc.Apply(file)

	if len(positional) == 0 {
		printShortHelp(os.Stdout)
		return 0
	}

	command := positional[0]
	rest := positional[1:]

	switch command {
	case "help", "--help", "-h":
		return runHelp(rest, &rc)
	case "login":
		return runLogin(rest, &rc)
	case "logout":
		return runLogout(rest, &rc)
	case "config":
		return runConfig(rest, &rc)
	case "version", "--version":
		fmt.Println("mu cli (registry-driven, talks to /mcp)")
		return 0
	}

	// Anything else is treated as an MCP tool name.
	return runTool(command, rest, &rc)
}

// runTool dispatches a tool call. It parses remaining --flag value
// pairs into a JSON args map, infers types, and invokes the tool.
func runTool(name string, rest []string, rc *ResolvedConfig) int {
	if err := rc.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	args, trailing, err := parseToolFlags(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 2
	}

	// If a single trailing positional arg is provided and there is no
	// flag for it, try to use it as the most obvious required param
	// for common tools: prompt/query/q. This enables `mu chat "hello"`
	// and `mu news_search "ai safety"` without needing --prompt / --query.
	if len(trailing) == 1 && len(args) == 0 {
		if v, ok := defaultArgKey(name); ok {
			args[v] = trailing[0]
			trailing = nil
		}
	}
	if len(trailing) > 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments:", strings.Join(trailing, " "))
		return 2
	}

	client := NewClient(rc)
	text, err := client.CallTool(name, args)
	if text != "" {
		if ferr := Format(os.Stdout, text, rc); ferr != nil {
			fmt.Fprintln(os.Stderr, "format error:", ferr)
		}
	}
	if err != nil {
		if text == "" {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		return 1
	}
	return 0
}

// parseToolFlags walks remaining args and extracts --name value /
// --name=value pairs. Bare positional arguments are returned separately
// so the caller can decide how to interpret them.
func parseToolFlags(args []string) (map[string]any, []string, error) {
	out := map[string]any{}
	var trailing []string
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "--") && !strings.HasPrefix(a, "-") {
			trailing = append(trailing, a)
			i++
			continue
		}
		// Strip leading dashes.
		flag := strings.TrimLeft(a, "-")
		var value string
		if eq := strings.Index(flag, "="); eq >= 0 {
			value = flag[eq+1:]
			flag = flag[:eq]
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			value = args[i+1]
			i++
		} else {
			// Bool flag with no value (e.g. --public).
			out[flag] = true
			i++
			continue
		}
		if flag == "" {
			return nil, nil, fmt.Errorf("empty flag name")
		}
		out[flag] = coerce(value)
		i++
	}
	return out, trailing, nil
}

// coerce converts a string value to the most plausible JSON type.
// Numbers and booleans become their typed form; everything else stays
// a string. Edge cases (a string that happens to look numeric) can be
// worked around by the caller quoting the value.
func coerce(s string) any {
	// Bool.
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	// Integer.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	// Float.
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// defaultArgKey returns the parameter a single positional argument
// should be assigned to for a small set of well-known tools. Returns
// ("", false) when there is no default.
func defaultArgKey(tool string) (string, bool) {
	switch tool {
	case "chat", "agent", "apps_build":
		return "prompt", true
	case "news_search", "video_search", "social_search", "quran_search":
		return "query", true
	case "web_search", "search", "places_search":
		return "q", true
	case "web_fetch":
		return "url", true
	case "blog_read", "apps_read":
		return "id", true
	}
	return "", false
}
