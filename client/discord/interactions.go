package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/service"
	"mu/service/wallet"
)

type interaction struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	Type      int    `json:"type"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
	Member    *struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"member"`
	User *struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Data struct {
		Name    string        `json:"name"`
		Options []optionValue `json:"options"`
	} `json:"data"`
}

// optionValue is one option Discord sends back. A subcommand arrives as an
// option of type 1 whose own Options are the tool's parameters, so this nests.
type optionValue struct {
	Name    string        `json:"name"`
	Type    int           `json:"type"`
	Value   any           `json:"value"`
	Options []optionValue `json:"options"`
}

func (i *interaction) userID() string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func (i *interaction) username() string {
	if i.Member != nil {
		return i.Member.User.Username
	}
	if i.User != nil {
		return i.User.Username
	}
	return ""
}

func (i *interaction) getOption(name string) string {
	for _, opt := range i.Data.Options {
		if opt.Name == name {
			return fmt.Sprintf("%v", opt.Value)
		}
	}
	return ""
}

func handleInteraction(raw json.RawMessage) {
	var inter interaction
	if err := json.Unmarshal(raw, &inter); err != nil {
		return
	}

	// Only handle slash commands (type 2)
	if inter.Type != 2 {
		return
	}

	discordID := inter.userID()
	isChannelCmd := inter.Member != nil

	accountID := LinkedAccount(discordID)

	// Defer the response — tell Discord we're thinking
	deferResponse(inter.ID, inter.Token)

	if accountID == "" {
		editResponse(inter.Token, "Send me a DM with `link <username>` to connect your Mu account first.")
		return
	}

	// Anything scoped is private, and a slash command in a guild channel
	// answers into that channel where everyone can read it.
	//
	// Two commands used to say this for themselves — /mail and /balance each
	// carried their own isChannelCmd guard. That is a hand-written list of the
	// private things somebody remembered, and it did not include /events or
	// /contacts, which are generated from the registry like every other
	// service command. So "/events list" in a shared channel printed the
	// caller's calendar to the channel.
	//
	// Derived from the Spec instead: Scoped already means "holds one person's
	// data", and it is the same flag that makes the MCP tool refuse an
	// unauthenticated call. A service that becomes scoped later is covered
	// without anybody remembering this file exists.
	if channelPrivate(inter.Data.Name, isChannelCmd) {
		editResponse(inter.Token, "That's private — use this command in a DM.")
		return
	}

	app.Log("discord", "Slash /%s from %s (%s)", inter.Data.Name, inter.username(), accountID)
	trackQuery(accountID)

	var prompt string
	switch inter.Data.Name {
	case "agent":
		prompt = inter.getOption("prompt")
	case "news":
		prompt = "latest news"
	case "markets":
		cat := inter.getOption("category")
		if cat != "" {
			prompt = cat + " market prices"
		} else {
			prompt = "crypto market prices"
		}
	case "weather":
		loc := inter.getOption("location")
		if loc != "" {
			prompt = "weather in " + loc
		} else {
			prompt = "weather forecast"
		}
	case "mail":
		// The generic guard above already refused this in a channel; mail is a
		// scoped service. Kept as a case only for its prompt.
		prompt = "read my email"
	case "apps":
		q := inter.getOption("query")
		if q != "" {
			prompt = "search apps for " + q
		} else {
			prompt = "show me available apps"
		}
	case "social":
		prompt = "show the social feed"
	case "video":
		q := inter.getOption("query")
		prompt = "search videos for " + q
	case "blog":
		prompt = "latest blog posts"
	case "search":
		q := inter.getOption("query")
		prompt = "search for " + q
	case "balance":
		if isChannelCmd {
			editResponse(inter.Token, "Wallet balance is private — use this command in a DM.")
			return
		}
		bw, err := wallet.EnsureFor(accountID)
		if err != nil {
			editResponse(inter.Token, "Wallet error: "+err.Error())
			return
		}
		usdc, _ := wallet.USDCBalance(bw.Address)
		embed := Embed{
			Title:  "Your Base Wallet",
			Color:  ColorGreen,
			Fields: []EmbedField{{Name: "USDC", Value: "$" + usdc, Inline: true}},
			Footer: &EmbedFooter{Text: bw.Address},
		}
		editResponseEmbed(inter.Token, embed)
		return
	case "usage":
		u := GetUserUsage(accountID)
		embed := Embed{
			Title: "Your Usage",
			Color: ColorBlue,
			Fields: []EmbedField{
				{Name: "Today", Value: fmt.Sprintf("%d queries", u.DailyCount), Inline: true},
				{Name: "All time", Value: fmt.Sprintf("%d queries", u.Queries), Inline: true},
				{Name: "Last query", Value: func() string {
					if u.LastQuery.IsZero() {
						return "never"
					}
					return u.LastQuery.Format("2 Jan 15:04")
				}(), Inline: true},
			},
			Footer: &EmbedFooter{Text: accountID},
		}
		editResponseEmbed(inter.Token, embed)
		return
	default:
		// Every other command is a tool: /news list is news_list, and its
		// options are that tool's parameters. Run it directly — no model call
		// to work out what the person meant when they already said it.
		if name, args, ok := inter.tool(); ok {
			out, isErr, err := api.ExecuteToolAs(accountID, name, args)
			if err != nil && out == "" {
				out = "That didn't work: " + err.Error()
			}
			if strings.TrimSpace(out) == "" {
				out = "No result."
			}
			if isErr {
				app.Log("discord", "Tool %s failed for %s: %v", name, accountID, err)
			}
			editResponse(inter.Token, out)
			return
		}
		prompt = inter.Data.Name
	}

	if prompt == "" {
		editResponse(inter.Token, "Please provide a prompt.")
		return
	}

	// A slash command is a routing decision the client already made, so the
	// agent it names is passed rather than left for the keyword router to guess
	// from the prompt text.
	res, err := agent.Ask(agent.AskRequest{
		Account: accountID,
		Client:  Client,
		Thread:  inter.ChannelID,
		Text:    prompt,
		Public:  isChannelCmd,
		Agent:   commandAgent(inter.Data.Name),
		Trigger: "discord /" + inter.Data.Name,
	})
	answer := res.Text
	if err != nil {
		editResponse(inter.Token, "Error: "+err.Error())
		return
	}

	if strings.TrimSpace(answer) == "" {
		editResponse(inter.Token, "I couldn't generate a response.")
		return
	}

	embed := formatAsEmbed(prompt, answer)
	editResponseEmbed(inter.Token, embed)
}

func deferResponse(interactionID, interactionToken string) {
	body, _ := json.Marshal(map[string]any{
		"type": 5, // DEFERRED_CHANNEL_MESSAGE_WITH_SOURCE
	})
	url := fmt.Sprintf("https://discord.com/api/v10/interactions/%s/%s/callback", interactionID, interactionToken)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func editResponse(interactionToken, content string) {
	if len(content) > 2000 {
		content = content[:1997] + "…"
	}
	body, _ := json.Marshal(map[string]string{"content": content})
	url := fmt.Sprintf("https://discord.com/api/v10/webhooks/%s/%s/messages/@original", botAppID, interactionToken)
	req, _ := http.NewRequest("PATCH", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Edit response error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func editResponseEmbed(interactionToken string, embed Embed) {
	body, _ := json.Marshal(map[string]any{
		"embeds": []Embed{embed},
	})
	url := fmt.Sprintf("https://discord.com/api/v10/webhooks/%s/%s/messages/@original", botAppID, interactionToken)
	req, _ := http.NewRequest("PATCH", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Edit embed response error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// tool reads an interaction as a tool call: the command is the service, the
// subcommand is the method, and the options are the arguments.
//
// It answers false when the command names no tool, so the caller can fall back
// to the agent — which is the right answer for a command like /weather, where
// turning "London" into a latitude is work the model does and the tool does
// not.
func (i *interaction) tool() (string, map[string]any, bool) {
	name := i.Data.Name
	opts := i.Data.Options

	// A subcommand is an option of type 1 carrying its own options.
	if len(opts) == 1 && opts[0].Type == optionSubcmd {
		name += "_" + opts[0].Name
		opts = opts[0].Options
	}

	t, ok := api.Lookup(name)
	if !ok {
		return "", nil, false
	}

	args := map[string]any{}
	for _, o := range opts {
		if o.Value == nil {
			continue
		}
		args[o.Name] = o.Value
	}
	if !api.Ready(t, args) {
		return "", nil, false
	}
	return t.Name, args, true
}

// channelPrivate reports whether a command must refuse to answer here because
// the answer would be readable by everyone in the channel.
func channelPrivate(commandName string, inChannel bool) bool {
	return inChannel && service.AccountScoped(commandName)
}
