package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mu/agent"
	"mu/internal/app"
	"mu/trade"
)

type interaction struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	Type      int    `json:"type"`
	ChannelID string `json:"channel_id"`
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
		Name    string `json:"name"`
		Options []struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		} `json:"options"`
	} `json:"data"`
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
	accountID := GetLinkedAccount(discordID)
	isChannelCmd := inter.Member != nil

	// Defer the response — tell Discord we're thinking
	deferResponse(inter.ID, inter.Token)

	if accountID == "" {
		editResponse(inter.Token, "Send me a DM with `link <username>` to connect your Mu account first.")
		return
	}

	app.Log("discord", "Slash /%s from %s (%s)", inter.Data.Name, inter.username(), accountID)

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
		if isChannelCmd {
			editResponse(inter.Token, "Mail is private — use this command in a DM.")
			return
		}
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
	case "swap":
		if isChannelCmd {
			editResponse(inter.Token, "Trading is private — use this command in a DM.")
			return
		}
		from := inter.getOption("from")
		to := inter.getOption("to")
		amount := inter.getOption("amount")
		if from != "" && to != "" && amount != "" {
			quote, err := trade.GetQuote(from, to, amount)
			if err != nil {
				editResponse(inter.Token, "Quote failed: "+err.Error())
				return
			}
			embed := Embed{
				Title:       "Swap Quote",
				Description: fmt.Sprintf("**%s** → **%s**", quote.AmountIn, quote.AmountOut),
				Color:       ColorGold,
				Fields: []EmbedField{
					{Name: "Pool Fee", Value: quote.PoolFee, Inline: true},
				},
				Footer: &EmbedFooter{Text: "Use the web UI at /markets?category=trade to execute"},
			}
			editResponseEmbed(inter.Token, embed)
			return
		}
		prompt = "swap " + amount + " " + from + " for " + to
	case "balance":
		if isChannelCmd {
			editResponse(inter.Token, "Wallet balance is private — use this command in a DM.")
			return
		}
		info := trade.GetWalletInfo(accountID)
		if info == nil {
			editResponse(inter.Token, "No trading wallet. Create one at /markets?category=trade")
			return
		}
		balances := trade.GetBalances(info.Address)
		var fields []EmbedField
		for symbol, amount := range balances {
			fields = append(fields, EmbedField{Name: symbol, Value: amount, Inline: true})
		}
		if len(fields) == 0 {
			fields = append(fields, EmbedField{Name: "Balance", Value: "No tokens found"})
		}
		embed := Embed{
			Title:  "Trading Wallet",
			Color:  ColorGreen,
			Fields: fields,
			Footer: &EmbedFooter{Text: info.Address},
		}
		editResponseEmbed(inter.Token, embed)
		return
	default:
		prompt = inter.Data.Name
	}

	if prompt == "" {
		editResponse(inter.Token, "Please provide a prompt.")
		return
	}

	history := getHistory(discordID)
	answer, err := agent.QueryWithOpts(accountID, prompt, agent.QueryOpts{
		History: history,
		Public:  isChannelCmd,
	})
	if err != nil {
		editResponse(inter.Token, "Error: "+err.Error())
		return
	}

	if strings.TrimSpace(answer) == "" {
		editResponse(inter.Token, "I couldn't generate a response.")
		return
	}

	addHistory(discordID, "user", prompt)
	addHistory(discordID, "assistant", answer)

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
