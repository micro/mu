package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/internal/api"
	"mu/internal/app"
)

// ── Embeds ──

type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type EmbedFooter struct {
	Text string `json:"text"`
}

const (
	ColorBlue   = 0x3498db
	ColorGreen  = 0x2ecc71
	ColorRed    = 0xe74c3c
	ColorGold   = 0xf1c40f
	ColorPurple = 0x9b59b6
	ColorGray   = 0x95a5a6
)

func sendEmbed(channelID string, embed Embed) {
	body, _ := json.Marshal(map[string]any{
		"embeds": []Embed{embed},
	})
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/channels/"+channelID+"/messages", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Send embed error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

type Button struct {
	Type     int    `json:"type"`
	Style    int    `json:"style"`
	Label    string `json:"label"`
	CustomID string `json:"custom_id"`
}

const (
	ButtonPrimary   = 1
	ButtonDanger    = 4
	ButtonSecondary = 2
)

// ── Slash Commands ──

type SlashCommand struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Options     []SlashCommandOption `json:"options,omitempty"`
}

type SlashCommandOption struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Type        int                  `json:"type"`
	Required    bool                 `json:"required,omitempty"`
	Choices     []SlashCommandChoice `json:"choices,omitempty"`
	// Options nests: a subcommand carries its own parameters here, which is how
	// /news list gets the parameters of the news_list tool.
	Options []SlashCommandOption `json:"options,omitempty"`
}

type SlashCommandChoice struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

const (
	OptionString = 3
	OptionNumber = 10
)

// platformCommands are the ones that are not tools: the agent itself, and the
// two account views. Everything else is generated from the tool registry — see
// commands.go.
var platformCommands = []SlashCommand{
	{
		Name:        "agent",
		Description: "Ask the AI agent anything",
		Options: []SlashCommandOption{
			{Name: "prompt", Description: "Your question", Type: OptionString, Required: true},
		},
	},
	{
		Name:        "balance",
		Description: "Check your Base wallet USDC balance",
	},
	{
		Name:        "usage",
		Description: "View your query usage stats",
	},
}

func registerSlashCommands(appID string) {
	// Tools finish registering after the bot connects, and a command set built
	// from a half-filled registry is missing services with no way to tell.
	api.WaitForTools(30 * time.Second)

	commands := toolCommands()
	body, _ := json.Marshal(commands)
	url := fmt.Sprintf("https://discord.com/api/v10/applications/%s/commands", appID)
	req, _ := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Register slash commands error: %v", err)
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		app.Log("discord", "Slash command registration failed (%d): %.200s", resp.StatusCode, string(respBody))
	} else {
		app.Log("discord", "Registered %d slash commands", len(commands))
	}
}

// ── Proactive Notifications ──

// NotifyUser sends a DM to a user's linked Discord account.
func NotifyUser(muAccountID, message string) {
	linkMu.RLock()
	var discordID string
	for did, mid := range links {
		if mid == muAccountID {
			discordID = did
			break
		}
	}
	linkMu.RUnlock()

	if discordID == "" {
		return
	}

	channelID := getDMChannel(discordID)
	if channelID == "" {
		return
	}
	sendMessage(channelID, message)
}

func getDMChannel(userID string) string {
	body, _ := json.Marshal(map[string]string{"recipient_id": userID})
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/users/@me/channels", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID
}

// formatAsEmbed converts an agent response into a rich embed.
func formatAsEmbed(prompt, answer string) Embed {
	color := ColorBlue

	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "news"):
		color = ColorPurple
	case strings.Contains(lower, "weather"):
		color = ColorGold
	case strings.Contains(lower, "market") || strings.Contains(lower, "price") || strings.Contains(lower, "btc") || strings.Contains(lower, "eth"):
		color = ColorGreen
	case strings.Contains(lower, "mail") || strings.Contains(lower, "email"):
		color = ColorRed
	case strings.Contains(lower, "video"):
		color = ColorRed
	case strings.Contains(lower, "app"):
		color = ColorPurple
	case strings.Contains(lower, "social") || strings.Contains(lower, "blog"):
		color = ColorBlue
	case strings.Contains(lower, "search"):
		color = ColorGray
	}

	desc := app.NormalizeAnswerMarkdown(answer)
	if len(desc) > 4096 {
		desc = desc[:4093] + "…"
	}

	return Embed{
		Description: desc,
		Color:       color,
	}
}
