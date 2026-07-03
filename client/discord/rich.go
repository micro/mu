package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

func sendMessageWithButtons(channelID, content string, buttons []Button) {
	msg := map[string]any{
		"content": content,
		"components": []map[string]any{
			{
				"type":       1, // ACTION_ROW
				"components": buttons,
			},
		},
	}
	body, _ := json.Marshal(msg)
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/channels/"+channelID+"/messages", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Send buttons error: %v", err)
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

func NewButton(label, customID string, style int) Button {
	return Button{Type: 2, Style: style, Label: label, CustomID: customID}
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
}

type SlashCommandChoice struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

const (
	OptionString = 3
	OptionNumber = 10
)

var slashCommands = []SlashCommand{
	{
		Name:        "agent",
		Description: "Ask the AI agent anything",
		Options: []SlashCommandOption{
			{Name: "prompt", Description: "Your question", Type: OptionString, Required: true},
		},
	},
	{
		Name:        "news",
		Description: "Get the latest news headlines",
	},
	{
		Name:        "markets",
		Description: "Get live market prices",
		Options: []SlashCommandOption{
			{Name: "category", Description: "Market category", Type: OptionString, Choices: []SlashCommandChoice{
				{Name: "Crypto", Value: "crypto"},
				{Name: "Futures", Value: "futures"},
				{Name: "Commodities", Value: "commodities"},
			}},
		},
	},
	{
		Name:        "weather",
		Description: "Get the weather forecast",
		Options: []SlashCommandOption{
			{Name: "location", Description: "City or place name", Type: OptionString},
		},
	},
	{
		Name:        "mail",
		Description: "Check your inbox",
	},
	{
		Name:        "balance",
		Description: "Check your Base wallet USDC balance",
	},
	{
		Name:        "apps",
		Description: "Search or browse apps",
		Options: []SlashCommandOption{
			{Name: "query", Description: "Search term", Type: OptionString},
		},
	},
	{
		Name:        "social",
		Description: "View the social feed",
	},
	{
		Name:        "video",
		Description: "Search for videos",
		Options: []SlashCommandOption{
			{Name: "query", Description: "Search term", Type: OptionString, Required: true},
		},
	},
	{
		Name:        "blog",
		Description: "View latest blog posts",
	},
	{
		Name:        "search",
		Description: "Search across all content",
		Options: []SlashCommandOption{
			{Name: "query", Description: "Search term", Type: OptionString, Required: true},
		},
	},
	{
		Name:        "usage",
		Description: "View your query usage stats",
	},
	{
		Name:        "setup",
		Description: "Configure the bot for this server (admin only)",
		Options: []SlashCommandOption{
			{Name: "briefing_channel", Description: "Channel for morning briefings", Type: 7, Required: true}, // type 7 = CHANNEL
		},
	},
}

func registerSlashCommands(appID string) {
	body, _ := json.Marshal(slashCommands)
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
		app.Log("discord", "Registered %d slash commands", len(slashCommands))
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

// NotifyEmbed sends a rich embed DM to a user's linked Discord account.
func NotifyEmbed(muAccountID string, embed Embed) {
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
	sendEmbed(channelID, embed)
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
	case strings.Contains(lower, "swap") || strings.Contains(lower, "trade"):
		color = ColorGold
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
