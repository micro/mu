// CLI dispatcher. Parses the command line, builds the MCP argument
// map, calls the tool, and formats the result.
package cli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"mu/internal/version"
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
		// --url is the instance to talk to, but web_fetch has a url parameter,
		// so once a tool has been named the flag belongs to the tool. Without
		// this, `mu web fetch --url https://example.com` quietly pointed the
		// whole CLI at example.com and tried to speak MCP to it.
		named := len(positional) > 0
		switch a {
		case "--url":
			if i+1 < len(args) && !named {
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
		if strings.HasPrefix(a, "--url=") && !named {
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
	case "setup":
		return runSetup(rest)
	case "wallet":
		return runWallet(rest)
	case "x402":
		return runX402(rest)
	case "version", "--version":
		fmt.Printf("mu %s\n", version.String())
		return 0
	}

	// Anything else is treated as an MCP tool name.
	return runTool(command, rest, &rc)
}

// toolWord matches the second word of a two-word command: "mu news list" is
// the tool news_list. Anything with a space, a dash or punctuation is an
// argument, not half of a name.
var toolWord = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// splitCommand turns "news list" into the tool "news_list", because that reads
// like a sentence and "news_list" reads like an identifier. Returns ok=false
// when the words cannot be a tool name.
//
// The underscore form still works — it is what /tools, the docs and every
// existing script use, and both spellings arriving at the same tool is the
// point.
func splitCommand(command string, rest []string) (string, []string, bool) {
	if len(rest) == 0 || !toolWord.MatchString(command) || !toolWord.MatchString(rest[0]) {
		return "", nil, false
	}
	return command + "_" + rest[0], rest[1:], true
}

// runTool dispatches a tool call. It parses remaining --flag value
// pairs into a JSON args map, infers types, and invokes the tool.
func runTool(name string, rest []string, rc *ResolvedConfig) int {
	if err := rc.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	// "mu news list" and "mu news_list" are the same call, and so are
	// "mu chat hello" and "mu chat --prompt hello". Both readings are
	// plausible from the words alone, so try the likelier one and fall back to
	// the other if the server says there is no such tool. Nothing has happened
	// at that point — an unknown tool runs nothing — so the retry is free of
	// side effects.
	first, firstRest := name, rest
	var second string
	var secondRest []string

	if joined, remainder, ok := splitCommand(name, rest); ok {
		if takesOnePositional(name, rest) {
			// The word after the tool is its argument: mu chat "hello".
			second, secondRest = joined, remainder
		} else {
			first, firstRest = joined, remainder
			second, secondRest = name, rest
		}
	}

	// A fallback is only worth trying if the leftover words could be arguments
	// at all. Without this, `mu news nope` reports "unexpected arguments: nope"
	// when what the person actually did was name a tool that does not exist.
	if second != "" && !canTakeArgs(second, secondRest) {
		second = ""
	}

	code, unknown := callTool(first, firstRest, rc)
	if !unknown {
		return code
	}
	if second != "" {
		if code, unknown = callTool(second, secondRest, rc); !unknown {
			return code
		}
	}

	// No reading of the words is a tool this server has.
	typed := name
	if len(rest) > 0 && toolWord.MatchString(name) && toolWord.MatchString(rest[0]) {
		typed = name + " " + rest[0]
	}
	fmt.Fprintf(os.Stderr, "unknown tool: %s — run `mu help` for the list\n", typed)
	return 1
}

// canTakeArgs reports whether a tool could accept these words: either they are
// all flags, or the tool has a known positional argument to put one in.
func canTakeArgs(name string, rest []string) bool {
	// Ask the flag parser rather than counting dashes: in "--limit 5" the 5 is
	// a value, not a bare word, and only the parser knows that.
	_, trailing, err := parseToolFlags(rest)
	if err != nil {
		return false
	}
	switch len(trailing) {
	case 0:
		return true
	case 1:
		_, ok := defaultArgKey(name)
		return ok
	}
	return false
}

// takesOnePositional reports whether this looks like a tool being handed its
// one obvious argument — mu chat "hello", mu agent "why" — rather than a
// two-word tool name.
func takesOnePositional(name string, rest []string) bool {
	if len(rest) != 1 {
		return false
	}
	_, ok := defaultArgKey(name)
	return ok
}

// callTool performs one interpretation of the command line. unknown reports
// that the server has no tool by that name, so the caller may try another.
func callTool(name string, rest []string, rc *ResolvedConfig) (code int, unknown bool) {
	args, trailing, err := parseToolFlags(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 2, false
	}

	// If a single trailing positional arg is provided and there is no
	// flag for it, try to use it as the most obvious required param
	// for common tools: prompt/query/q. This enables `mu chat "hello"`
	// and `mu news search "ai safety"` without needing --prompt / --query.
	if len(trailing) == 1 && len(args) == 0 {
		if v, ok := defaultArgKey(name); ok {
			args[v] = trailing[0]
			trailing = nil
		}
	}
	if len(trailing) > 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments:", strings.Join(trailing, " "))
		return 2, false
	}

	client := NewClient(rc)
	text, err := client.CallTool(name, args)

	var unknownTool *UnknownToolError
	if errors.As(err, &unknownTool) {
		return 1, true // say nothing: the caller may have another reading
	}

	if text != "" {
		if ferr := Format(os.Stdout, text, rc); ferr != nil {
			fmt.Fprintln(os.Stderr, "format error:", ferr)
		}
	}
	if err != nil {
		if text == "" {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		return 1, false
	}
	return 0, false
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
	case "news_search", "video_search", "social_search", "quran_search", "apps_search":
		return "query", true
	case "web_search", "search", "places_search":
		return "q", true
	case "web_fetch":
		return "url", true
	case "blog_read":
		return "id", true
	case "apps_read":
		// An app is addressed by slug, not id. Mapped to "id" this failed with
		// "apps_read requires slug" for anyone following the documented
		// `mu apps_read <slug>`.
		return "slug", true
	}
	return "", false
}
