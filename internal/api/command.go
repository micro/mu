package api

// Tools as commands.
//
// A tool is named service_method, which reads as an identifier and types like
// one. People do not talk that way: they say "news list", "markets list
// stocks". Every human surface — the CLI, Telegram, Discord — has to turn words
// into a tool name and a set of arguments, and each of them was doing something
// different, or in Discord's case doing nothing at all and asking the model to
// guess.
//
// This is that translation, once: which words name a tool, and where the rest
// of them go.

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Commands returns every callable tool, ordered by name. This is the list the
// human surfaces build their command sets from, so a tool added to the server
// reaches all of them.
func Commands() []Tool {
	return mcpTools()
}

// Lookup finds a tool by name or alias.
func Lookup(name string) (Tool, bool) {
	for i := range tools {
		if toolMatches(tools[i], name) && !tools[i].RESTOnly {
			return tools[i], true
		}
	}
	return Tool{}, false
}

// Service is the part of a tool name before the underscore — the service it
// belongs to. A tool with no underscore is its own service.
func Service(tool string) string {
	if before, _, ok := strings.Cut(tool, "_"); ok {
		return before
	}
	return tool
}

// Method is the part after the underscore, empty for a tool that is one word.
func Method(tool string) string {
	if _, after, ok := strings.Cut(tool, "_"); ok {
		return after
	}
	return ""
}

// Services returns the services that have tools, each with its tools, ordered.
// Discord builds one command per service from this; anything without an
// underscore is its own single-method service.
func Services() map[string][]Tool {
	out := map[string][]Tool{}
	for _, t := range Commands() {
		s := Service(t.Name)
		out[s] = append(out[s], t)
	}
	for s := range out {
		list := out[s]
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		out[s] = list
	}
	return out
}

// Resolve reads a tool name off the front of a command line. It accepts both
// spellings — "news list" and "news_list" — and returns the tool with the words
// that are left.
func Resolve(words []string) (Tool, []string, bool) {
	if len(words) == 0 {
		return Tool{}, nil, false
	}
	// Two words first: "news list" is more specific than "news", and a service
	// name on its own is usually an alias for its list method.
	if len(words) > 1 {
		if t, ok := Lookup(words[0] + "_" + words[1]); ok {
			return t, words[2:], true
		}
	}
	if t, ok := Lookup(words[0]); ok {
		return t, words[1:], true
	}
	return Tool{}, nil, false
}

// Args turns the words after a tool name into its arguments.
//
// Two forms, both of which people type without being told to:
//
//	--flag value        explicit, for anything with more than one parameter
//	the rest of it      for a tool with one obvious parameter to put it in
//
// ok is false when there are words left that have nowhere to go. The caller
// decides what that means — the chat bots hand the whole message to the agent
// instead, which is usually what someone typing prose wanted anyway.
func Args(t Tool, words []string) (map[string]any, bool) {
	args := map[string]any{}
	var bare []string

	for i := 0; i < len(words); i++ {
		w := words[i]
		if !strings.HasPrefix(w, "--") {
			bare = append(bare, w)
			continue
		}
		flag := strings.TrimPrefix(w, "--")
		if name, value, ok := strings.Cut(flag, "="); ok {
			args[name] = value
			continue
		}
		if i+1 < len(words) && !strings.HasPrefix(words[i+1], "--") {
			args[flag] = words[i+1]
			i++
			continue
		}
		args[flag] = true // a bare flag is a boolean
	}

	if len(bare) == 0 {
		return args, true
	}

	// Everything left over goes to the tool's one obvious parameter, if it has
	// one: a search takes a query, chat takes a prompt. Joined, not split, so
	// "markets list us stocks" is one phrase rather than two arguments.
	if key, ok := freeTextParam(t); ok {
		if _, taken := args[key]; !taken {
			args[key] = strings.Join(bare, " ")
			return args, true
		}
	}
	return args, false
}

// Ready reports whether a tool has everything it needs to run. A surface that
// can fall back to the agent uses this to decide: "/weather" names a real tool
// that wants a latitude and a longitude, and the agent — which can turn a place
// name into both — is the better answer to a bare word.
func Ready(t Tool, args map[string]any) bool {
	return missingRequired(t, args) == ""
}

// freeTextParam is the parameter loose words belong in: the single required
// one, or the only one there is. A tool with two required parameters — weather
// wants a latitude and a longitude — has no such place, and saying so is what
// stops "weather london" being answered with nonsense.
func freeTextParam(t Tool) (string, bool) {
	var required []ToolParam
	for _, p := range t.Params {
		if p.Required {
			required = append(required, p)
		}
	}
	switch {
	case len(required) == 1 && required[0].Type == "string":
		return required[0].Name, true
	case len(required) == 0 && len(t.Params) == 1 && t.Params[0].Type == "string":
		return t.Params[0].Name, true
	}
	return "", false
}

// Tools arrive in two waves: a static set in this package, then the rest from
// main() as it wires each service up. Anything that builds a command set from
// the registry — the Discord slash commands, the Telegram menu — has to wait
// for the second wave, or it publishes half a product and the operator finds
// out when a command is missing.
var (
	toolsReady = make(chan struct{})
	readyOnce  sync.Once
)

// ToolsRegistered is called by main() once every tool is registered.
func ToolsRegistered() { readyOnce.Do(func() { close(toolsReady) }) }

// WaitForTools blocks until the registry is complete, or until the timeout —
// which is a backstop, not a design: a caller that waits forever on a signal
// nobody sends is a bot that never gets its commands.
func WaitForTools(timeout time.Duration) {
	select {
	case <-toolsReady:
	case <-time.After(timeout):
	}
}

// ToolCount is how many tools an agent connecting here would be offered.
//
// The landing said "67 real tools" as a literal, which was wrong the moment
// anyone added one — it was 72 by the time somebody counted. A number that
// describes the registry should come from the registry.
//
// It then came from the wrong part of it: len(tools) is every registered
// entry, including the RESTOnly ones that are HTTP endpoints and not tools, so
// the landing advertised 84 while tools/list served 82. The number on the front
// page is the first claim anybody checks, and it is checkable in one curl.
// mcpTools is the list an agent actually gets, so it is the list to count.
func ToolCount() int { return len(mcpTools()) }
