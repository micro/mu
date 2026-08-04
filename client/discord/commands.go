package discord

// Discord commands, derived from the tool registry.
//
// The bot had twelve hand-written slash commands, each of which turned into a
// sentence for the agent to interpret: /news meant "latest news", and a model
// call decided what that was. Sixty-odd tools were unreachable, and the twelve
// that were reachable took the long way round.
//
// Discord's own shape fits Mu's: a command with subcommands is exactly
// service plus method, so /news list is the tool news_list and its options are
// that tool's parameters. Registering them from the registry means a tool added
// to the server appears in Discord with nothing to edit here.

import (
	"regexp"
	"sort"
	"strings"

	"mu/internal/api"
)

// Discord's limits on an application command, from its documentation. Breaking
// one rejects the whole registration, which would leave the bot with no
// commands at all, so they are enforced here rather than discovered in a 400.
const (
	maxCommands     = 100
	maxSubcommands  = 25
	maxOptions      = 25
	maxDescription  = 100
	maxCommandName  = 32
	optionSubcmd    = 1
	optionString    = OptionString
	optionNumberInt = OptionNumber
)

var discordName = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// toolCommands builds the slash command set: one command per service, one
// subcommand per method, plus the platform commands that are not tools.
func toolCommands() []SlashCommand {
	out := append([]SlashCommand{}, platformCommands...)
	taken := map[string]bool{}
	for _, c := range out {
		taken[c.Name] = true
	}

	services := api.Services()
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, service := range names {
		if taken[service] || !discordName.MatchString(service) {
			continue
		}
		if cmd, ok := serviceCommand(service, services[service]); ok {
			out = append(out, cmd)
			taken[service] = true
		}
		if len(out) >= maxCommands {
			break
		}
	}
	return out
}

// serviceCommand turns one service's tools into a command with a subcommand
// each. A service whose only tool is a single word — chat, agent — becomes a
// plain command with that tool's options instead, since /chat chat would be
// silly.
func serviceCommand(service string, tools []api.Tool) (SlashCommand, bool) {
	var subs []SlashCommandOption
	for _, t := range tools {
		method := api.Method(t.Name)
		if method == "" {
			// One-word tool: the command is the tool.
			if len(tools) == 1 {
				return SlashCommand{
					Name:        service,
					Description: describe(t.Description, service),
					Options:     options(t),
				}, true
			}
			continue
		}
		if !discordName.MatchString(method) {
			continue
		}
		subs = append(subs, SlashCommandOption{
			Name:        method,
			Description: describe(t.Description, method),
			Type:        optionSubcmd,
			Options:     options(t),
		})
		if len(subs) >= maxSubcommands {
			break
		}
	}
	if len(subs) == 0 {
		return SlashCommand{}, false
	}
	return SlashCommand{
		Name:        service,
		Description: describe(serviceSummary(tools), service),
		Options:     subs,
	}, true
}

// options are a tool's parameters as Discord options, required ones first —
// Discord rejects a required option listed after an optional one.
func options(t api.Tool) []SlashCommandOption {
	var required, optional []SlashCommandOption
	for _, p := range t.Params {
		if !discordName.MatchString(p.Name) {
			continue
		}
		opt := SlashCommandOption{
			Name:        p.Name,
			Description: describe(p.Description, p.Name),
			Type:        optionString,
			Required:    p.Required,
		}
		if p.Type == "number" || p.Type == "integer" {
			opt.Type = optionNumberInt
		}
		if p.Required {
			required = append(required, opt)
		} else {
			optional = append(optional, opt)
		}
	}
	all := append(required, optional...)
	if len(all) > maxOptions {
		all = all[:maxOptions]
	}
	return all
}

// serviceSummary is the line under a service command: what it can do.
func serviceSummary(tools []api.Tool) string {
	var methods []string
	for _, t := range tools {
		if m := api.Method(t.Name); m != "" {
			methods = append(methods, m)
		}
	}
	return strings.Join(methods, ", ")
}

// describe fits a description into Discord's limit, falling back to the name
// when there is nothing to say — an empty description is rejected.
func describe(text, fallback string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		text = fallback
	}
	if len(text) > maxDescription {
		cut := text[:maxDescription-1]
		// Prefer a word boundary, so the line does not end mid-word.
		if i := strings.LastIndex(cut, " "); i > maxDescription/2 {
			cut = cut[:i]
		}
		text = cut + "…"
	}
	return text
}
