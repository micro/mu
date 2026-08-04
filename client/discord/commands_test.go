package discord

import (
	"encoding/json"
	"testing"

	"mu/internal/api"
)

// registerProbes stands up tools shaped like the real ones. The generated
// command set is checked against Discord's documented limits rather than
// against Discord, because breaking one of them rejects the *whole*
// registration — the bot would come up with no commands at all, and the only
// signal would be a 400 in the log.
func registerProbes(t *testing.T) {
	t.Helper()
	for _, tool := range []api.Tool{
		{Name: "dnews_list", Description: "Get recent news headlines with short summaries balanced across all topics (not dominated by one topic like crypto). Use for general news and briefing requests."},
		{Name: "dnews_search", Description: "Search news", Params: []api.ToolParam{
			{Name: "query", Type: "string", Description: "Search text", Required: true},
			{Name: "limit", Type: "number", Description: "How many"},
		}},
		{Name: "dsolo", Description: "A tool that is its own service"},
	} {
		if _, ok := api.Lookup(tool.Name); !ok {
			api.RegisterTool(tool)
		}
	}
}

func TestGeneratedCommandsObeyDiscordsLimits(t *testing.T) {
	registerProbes(t)
	cmds := toolCommands()

	if len(cmds) == 0 {
		t.Fatal("no commands were generated")
	}
	if len(cmds) > maxCommands {
		t.Errorf("%d commands, Discord allows %d", len(cmds), maxCommands)
	}

	seen := map[string]bool{}
	for _, c := range cmds {
		if !discordName.MatchString(c.Name) {
			t.Errorf("command %q is not a valid name", c.Name)
		}
		if seen[c.Name] {
			t.Errorf("command %q is registered twice", c.Name)
		}
		seen[c.Name] = true

		checkDescription(t, c.Name, c.Description)
		if len(c.Options) > maxSubcommands {
			t.Errorf("%s has %d options, Discord allows %d", c.Name, len(c.Options), maxSubcommands)
		}

		requiredSeen, optionalSeen := false, false
		for _, o := range c.Options {
			if !discordName.MatchString(o.Name) {
				t.Errorf("%s/%s is not a valid option name", c.Name, o.Name)
			}
			checkDescription(t, c.Name+"/"+o.Name, o.Description)

			if o.Type == optionSubcmd {
				if len(o.Options) > maxOptions {
					t.Errorf("%s %s has %d options", c.Name, o.Name, len(o.Options))
				}
				checkOrder(t, c.Name+" "+o.Name, o.Options)
				continue
			}
			// Discord refuses a required option listed after an optional one.
			if o.Required {
				requiredSeen = true
				if optionalSeen {
					t.Errorf("%s lists a required option after an optional one", c.Name)
				}
			} else {
				optionalSeen = true
			}
			_ = requiredSeen
		}
	}
}

func checkDescription(t *testing.T, what, desc string) {
	t.Helper()
	if desc == "" {
		t.Errorf("%s has an empty description, which Discord rejects", what)
	}
	if len([]rune(desc)) > maxDescription {
		t.Errorf("%s description is %d characters, Discord allows %d", what, len([]rune(desc)), maxDescription)
	}
}

func checkOrder(t *testing.T, what string, opts []SlashCommandOption) {
	t.Helper()
	optional := false
	for _, o := range opts {
		if o.Required && optional {
			t.Errorf("%s lists a required option after an optional one", what)
		}
		if !o.Required {
			optional = true
		}
	}
}

// The point of generating them: a tool exists in Discord without anyone
// editing a list.
func TestEveryServiceBecomesACommand(t *testing.T) {
	registerProbes(t)
	cmds := toolCommands()

	byName := map[string]SlashCommand{}
	for _, c := range cmds {
		byName[c.Name] = c
	}

	news, ok := byName["dnews"]
	if !ok {
		t.Fatal("dnews did not become a command")
	}
	methods := map[string]SlashCommandOption{}
	for _, o := range news.Options {
		methods[o.Name] = o
	}
	if _, ok := methods["list"]; !ok {
		t.Error("dnews list is missing")
	}
	search, ok := methods["search"]
	if !ok {
		t.Fatal("dnews search is missing")
	}
	if len(search.Options) != 2 || search.Options[0].Name != "query" || !search.Options[0].Required {
		t.Errorf("dnews search options are wrong: %+v", search.Options)
	}

	// A one-word tool is a plain command, not /dsolo dsolo.
	solo, ok := byName["dsolo"]
	if !ok {
		t.Fatal("dsolo did not become a command")
	}
	for _, o := range solo.Options {
		if o.Type == optionSubcmd {
			t.Errorf("dsolo should have no subcommands: %+v", solo.Options)
		}
	}

	// The platform commands survive alongside the generated ones.
	for _, name := range []string{"agent", "balance", "usage"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("%s is missing from the command set", name)
		}
	}
}

// What is sent to Discord has to be JSON it will accept.
func TestCommandsMarshalToDiscordShape(t *testing.T) {
	registerProbes(t)
	b, err := json.Marshal(toolCommands())
	if err != nil {
		t.Fatal(err)
	}
	var back []map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	for _, c := range back {
		if c["name"] == nil || c["description"] == nil {
			t.Errorf("a command lost a required field: %v", c)
		}
	}
}
