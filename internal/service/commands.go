package service

import (
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
		if !inScope || spec.Scoped {
			continue
		}
		for method, ep := range spec.Endpoints {
			if ep.Writes || ep.Destructive || ep.Needs != 0 {
				continue
			}
			for _, command := range ep.Commands {
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
