package service

import (
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Command is a deliberately small prompt grammar: an exact phrase, or a phrase
// ending in one {argument}. Endpoints opt in; writes cannot be prompt commands.
type Command struct {
	Pattern  string
	Defaults map[string]any
}

type CommandCall struct {
	Service string
	Method  string
	Args    map[string]any
}

// MatchCommand resolves only declared, read-only commands in the caller's scope.
// Conflicting declarations fail closed rather than depending on map order.
func MatchCommand(input string, allowed []string) (CommandCall, bool) {
	return MatchCommandFor(input, allowed, false)
}

// MatchCommandFor admits account reads only for authenticated command surfaces.
// Dispatch still enforces the caller identity, scopes and endpoint permissions.
func MatchCommandFor(input string, allowed []string, private bool) (CommandCall, bool) {
	input = strings.TrimSpace(input)
	for _, pair := range [][2]string{{"\"", "\""}, {"'", "'"}, {"“", "”"}, {"‘", "’"}} {
		if len(input) > len(pair[0])+len(pair[1]) && strings.HasPrefix(input, pair[0]) && strings.HasSuffix(input, pair[1]) {
			input = strings.TrimSuffix(strings.TrimPrefix(input, pair[0]), pair[1])
			break
		}
	}

	var found CommandCall
	matched := false
	best := -1
	ambiguous := false
	for _, spec := range Specs() {
		inScope := false
		for _, name := range allowed {
			if name == spec.Name {
				inScope = true
				break
			}
		}
		if !inScope || (spec.Scoped && !private) {
			continue
		}
		for method, ep := range spec.Endpoints {
			if ep.Writes || ep.Destructive || ep.Needs == Operator || (ep.Needs != Open && !private) {
				continue
			}
			for _, command := range commandsFor(spec, method, ep) {
				args, ok := command.match(input)
				if !ok {
					continue
				}
				score := len(command.Pattern)
				if i := strings.Index(command.Pattern, "{"); i >= 0 {
					score = i
				} else {
					score += 10000
				}
				if score < best {
					continue
				}
				if score == best {
					ambiguous = true
					continue
				}
				found = CommandCall{spec.Name, method, args}
				matched = true
				best = score
				ambiguous = false
			}
		}
	}
	if !matched || ambiguous {
		return CommandCall{}, false
	}
	return found, true
}

func (c Command) match(input string) (map[string]any, bool) {
	input = strings.TrimSpace(input)
	if input == "" || len(input) > 8000 || !utf8.ValidString(input) {
		return nil, false
	}
	for _, r := range input {
		if unicode.IsControl(r) && r != '\t' {
			return nil, false
		}
	}
	words := strings.Fields(c.Pattern)
	if len(words) == 0 {
		return nil, false
	}
	args := map[string]any{}
	for k, v := range c.Defaults {
		args[k] = v
	}
	last := words[len(words)-1]
	if !strings.HasPrefix(last, "{") {
		if strings.ContainsAny(c.Pattern, "{}") {
			return nil, false
		}
		return args, strings.EqualFold(strings.Join(strings.Fields(strings.TrimRight(input, "?!.")), " "), strings.Join(words, " "))
	}
	if !strings.HasSuffix(last, "}") || len(words) < 2 {
		return nil, false
	}
	key := strings.TrimSuffix(strings.TrimPrefix(last, "{"), "}")
	if key == "" {
		return nil, false
	}
	for _, r := range key {
		if !unicode.IsLetter(r) && r != '_' {
			return nil, false
		}
	}
	rest := input
	for _, word := range words[:len(words)-1] {
		if strings.ContainsAny(word, "{}") {
			return nil, false
		}
		end := strings.IndexFunc(rest, unicode.IsSpace)
		if end < 0 || !strings.EqualFold(rest[:end], word) {
			return nil, false
		}
		rest = strings.TrimLeftFunc(rest[end:], unicode.IsSpace)
	}
	value := strings.TrimSpace(rest)
	if value == "" {
		return nil, false
	}
	for _, join := range []string{" then ", " and send ", " and email ", " and summarise ", " and summarize ", " and compare ", " and weather ", " and news "} {
		if strings.Contains(strings.ToLower(strings.Join(strings.Fields(value), " ")), join) {
			return nil, false
		}
	}
	if key == "place" {
		value = strings.TrimRight(value, "?!. ")
		if value == "" || strings.EqualFold(value, "in") || strings.EqualFold(value, "at") || strings.EqualFold(value, "for") {
			return nil, false
		}
		for _, word := range strings.Fields(strings.ToLower(value)) {
			word = strings.Trim(word, ",;:?!.")
			switch word {
			case "and", "then", "tomorrow", "today", "next", "yesterday", "not", "don't", "don’t", "like", "here", "there", "me", "my", "this", "week", "tonight":
				return nil, false
			}
		}
		for _, r := range value {
			if !unicode.IsLetter(r) && !unicode.IsSpace(r) && !strings.ContainsRune("-'’.,", r) {
				return nil, false
			}
		}
	}
	args[key] = value
	return args, true
}

// commandsFor derives bare service names and ordinary list phrases from List.
// A required input prevents inference: "maps" cannot invent a destination.
func commandsFor(spec Spec, method string, ep Endpoint) []Command {
	out := append([]Command(nil), ep.Commands...)
	if method != "List" || spec.Handler == nil {
		return out
	}
	m, ok := reflect.TypeOf(spec.Handler).MethodByName(method)
	if !ok || m.Type.NumIn() != 4 {
		return out
	}
	req := m.Type.In(2)
	if req.Kind() != reflect.Ptr || req.Elem().Kind() != reflect.Struct {
		return out
	}
	req = req.Elem()
	defaults := map[string]any{}
	for i := 0; i < req.NumField(); i++ {
		f := req.Field(i)
		if f.Tag.Get("required") == "true" || f.Anonymous {
			return out
		}
		if strings.Split(f.Tag.Get("json"), ",")[0] == "limit" {
			defaults["limit"] = 5
		}
	}
	names := append([]string{spec.Name, strings.ToLower(spec.NavLabel())}, ep.Aliases...)
	seen := map[string]bool{}
	for _, c := range out {
		seen[strings.ToLower(c.Pattern)] = true
	}
	for _, name := range names {
		for _, prefix := range []string{"", "latest ", "show ", "show me "} {
			pattern := strings.ToLower(prefix + name)
			if !seen[pattern] {
				out = append(out, Command{Pattern: pattern, Defaults: defaults})
				seen[pattern] = true
			}
		}
	}
	return out
}

// CommandExamples is one executable example per eligible endpoint, derived
// from the same declarations and permissions as matching, never a UI list.
func CommandExamples(allowed []string, private bool) []string {
	var out []string
	for _, spec := range Specs() {
		for method, ep := range spec.Endpoints {
			if call, ok := MatchCommandFor(spec.Name, allowed, private); ok && call.Service == spec.Name && call.Method == method {
				out = append(out, spec.Name)
				continue
			}
			for _, c := range commandsFor(spec, method, ep) {
				if strings.Contains(c.Pattern, "{") {
					continue
				}
				call, ok := MatchCommandFor(c.Pattern, allowed, private)
				if ok && call.Service == spec.Name && call.Method == method {
					out = append(out, c.Pattern)
					break
				}
			}
		}
	}
	sort.Strings(out)
	return out
}
