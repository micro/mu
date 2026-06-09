// Package discord connects Mu to Discord as a bot. Users DM the bot
// or mention it in a channel, and it runs the AI agent on their behalf.
//
// Setup:
//  1. Create a bot at https://discord.com/developers/applications
//  2. Enable Message Content Intent under Bot settings
//  3. Set DISCORD_BOT_TOKEN env var
//  4. Invite the bot to your server with the Messages scope
//
// Users link their Discord account to their Mu account by sending
// "link <username>" as their first message.
package discord

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"

	"github.com/gorilla/websocket"
)

var (
	botToken string
	botID    string
	botAppID string

	linkMu   sync.RWMutex
	links    = map[string]string{} // discord user ID → mu account ID

	historyMu sync.RWMutex
	histories = map[string][]agent.QueryMessage{} // discord user ID → recent messages
)

const maxHistory = 10

func getHistory(discordID string) []agent.QueryMessage {
	historyMu.RLock()
	defer historyMu.RUnlock()
	return histories[discordID]
}

func addHistory(discordID string, role, text string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	histories[discordID] = append(histories[discordID], agent.QueryMessage{Role: role, Text: text})
	if len(histories[discordID]) > maxHistory {
		histories[discordID] = histories[discordID][len(histories[discordID])-maxHistory:]
	}
}

func Load() {
	data.LoadJSON("discord_links.json", &links)
	go run()
}

func Enabled() bool {
	return settings.Get("DISCORD_BOT_TOKEN") != ""
}

// LinkAccount maps a Discord user ID to a Mu account.
func LinkAccount(discordID, muAccount string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	links[discordID] = muAccount
	data.SaveJSON("discord_links.json", links)
}

// GetLinkedAccount returns the Mu account for a Discord user, or "".
func GetLinkedAccount(discordID string) string {
	linkMu.RLock()
	defer linkMu.RUnlock()
	return links[discordID]
}

// DeleteLinks removes all links for a Mu account (account deletion).
func DeleteLinks(muAccount string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	for k, v := range links {
		if v == muAccount {
			delete(links, k)
		}
	}
	data.SaveJSON("discord_links.json", links)
}

// autoCreateAccount creates a Mu account from a Discord user and links it.
func autoCreateAccount(discordID, username string) string {
	// Sanitise Discord username for Mu (lowercase, alphanumeric, 4-24 chars)
	id := strings.ToLower(username)
	id = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, id)
	if len(id) < 4 {
		id = id + discordID[len(discordID)-4:]
	}
	if len(id) > 24 {
		id = id[:24]
	}

	// Check if username is taken — append digits if so
	baseID := id
	for i := 0; i < 100; i++ {
		if _, err := auth.GetAccount(id); err != nil {
			break // not taken
		}
		id = fmt.Sprintf("%s%d", baseID, i+1)
		if len(id) > 24 {
			id = id[:24]
		}
	}

	// Generate a random password (user doesn't need it — they auth via Discord)
	passBytes := make([]byte, 16)
	rand.Read(passBytes)
	pass := hex.EncodeToString(passBytes)

	acc := &auth.Account{
		ID:      id,
		Name:    username,
		Secret:  pass,
		Created: time.Now(),
	}
	if err := auth.Create(acc); err != nil {
		app.Log("discord", "Auto-create account failed for %s: %v", username, err)
		return ""
	}

	LinkAccount(discordID, id)
	app.Log("discord", "Auto-created account %s for Discord user %s", id, username)
	return id
}

// ── Discord Gateway ──

func run() {
	for {
		token := settings.Get("DISCORD_BOT_TOKEN")
		if token == "" {
			time.Sleep(30 * time.Second)
			continue
		}
		if err := connect(token); err != nil {
			app.Log("discord", "Connection error: %v — reconnecting in 10s", err)
			time.Sleep(10 * time.Second)
		}
	}
}

func connect(token string) error {
	botToken = token

	gatewayURL, err := getGatewayURL()
	if err != nil {
		return fmt.Errorf("get gateway: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(gatewayURL+"?v=10&encoding=json", nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Read Hello
	var hello struct {
		Op int `json:"op"`
		D  struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		} `json:"d"`
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return fmt.Errorf("read hello: %w", err)
	}
	if hello.Op != 10 {
		return fmt.Errorf("expected op 10, got %d", hello.Op)
	}

	// Send Identify
	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   botToken,
			"intents": 1<<9 | 1<<12 | 1<<15, // GUILD_MESSAGES | DIRECT_MESSAGES | MESSAGE_CONTENT
			"properties": map[string]string{
				"os":      "linux",
				"browser": "mu",
				"device":  "mu",
			},
		},
	}
	if err := conn.WriteJSON(identify); err != nil {
		return fmt.Errorf("send identify: %w", err)
	}

	// Start heartbeat
	ticker := time.NewTicker(time.Duration(hello.D.HeartbeatInterval) * time.Millisecond)
	defer ticker.Stop()
	var lastSeq *int

	go func() {
		for range ticker.C {
			hb := map[string]any{"op": 1, "d": lastSeq}
			conn.WriteJSON(hb)
		}
	}()

	// Read events
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var event struct {
			Op int              `json:"op"`
			T  string           `json:"t"`
			S  *int             `json:"s"`
			D  json.RawMessage  `json:"d"`
		}
		json.Unmarshal(msg, &event)

		if event.S != nil {
			lastSeq = event.S
		}

		switch event.T {
		case "READY":
			var ready struct {
				User struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				} `json:"user"`
				Application struct {
					ID string `json:"id"`
				} `json:"application"`
			}
			json.Unmarshal(event.D, &ready)
			botID = ready.User.ID
			botAppID = ready.Application.ID
			app.Log("discord", "Connected as %s (%s)", ready.User.Username, botID)
			go registerSlashCommands(botAppID)

		case "MESSAGE_CREATE":
			var m discordMessage
			json.Unmarshal(event.D, &m)
			go handleMessage(m)

		case "INTERACTION_CREATE":
			go handleInteraction(event.D)
		}
	}
}

type discordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"author"`
	GuildID     string `json:"guild_id"`
	MentionEveryone bool `json:"mention_everyone"`
	Mentions    []struct {
		ID string `json:"id"`
	} `json:"mentions"`
}

func handleMessage(m discordMessage) {
	// Ignore own messages
	if m.Author.Bot || m.Author.ID == botID {
		return
	}

	// Respond to DMs, mentions, or any message in a channel with the bot
	isDM := m.GuildID == ""
	isMention := false
	for _, mention := range m.Mentions {
		if mention.ID == botID {
			isMention = true
			break
		}
	}
	if !isDM && !isMention {
		return
	}

	app.Log("discord", "Received message (DM=%v mention=%v guild=%s): %.100s", isDM, isMention, m.GuildID, m.Content)

	// Strip bot mention from content
	content := m.Content
	content = strings.ReplaceAll(content, "<@"+botID+">", "")
	content = strings.ReplaceAll(content, "<@!"+botID+">", "")
	content = strings.TrimSpace(content)

	if content == "" {
		sendMessage(m.ChannelID, "Ask me anything — I'm your personal AI.")
		return
	}

	// Handle link command — links an existing Mu account via one-time code
	if strings.HasPrefix(strings.ToLower(content), "link ") {
		code := strings.TrimSpace(content[5:])
		accountID, ok := redeemCode(code)
		if !ok {
			sendMessage(m.ChannelID, "Invalid or expired code. Generate a new one at `/account` on Mu.")
			return
		}
		LinkAccount(m.Author.ID, accountID)
		sendMessage(m.ChannelID, fmt.Sprintf("Linked to **%s**. I'll run as your account from now on.", accountID))
		return
	}

	if strings.ToLower(content) == "unlink" {
		linkMu.Lock()
		delete(links, m.Author.ID)
		data.SaveJSON("discord_links.json", links)
		linkMu.Unlock()
		sendMessage(m.ChannelID, "Unlinked.")
		return
	}

	// Look up or auto-create account
	accountID := GetLinkedAccount(m.Author.ID)
	if accountID == "" {
		accountID = autoCreateAccount(m.Author.ID, m.Author.Username)
		if accountID == "" {
			sendMessage(m.ChannelID, "Couldn't create your account. Try again later.")
			return
		}
		sendMessage(m.ChannelID, fmt.Sprintf("Welcome! I've created your account **%s**. Ask me anything.", accountID))
	}

	app.Log("discord", "Message from %s (%s): %s", m.Author.Username, accountID, content)

	// Show typing indicator
	showTyping(m.ChannelID)

	// Get conversation history and run agent with context
	history := getHistory(m.Author.ID)
	answer, err := agent.Query(accountID, content, history...)
	if err != nil {
		app.Log("discord", "Agent error for %s: %v", accountID, err)
		sendMessage(m.ChannelID, "Sorry, something went wrong: "+err.Error())
		return
	}

	if strings.TrimSpace(answer) == "" {
		sendMessage(m.ChannelID, "I couldn't generate a response. Try rephrasing your question.")
		return
	}

	// Save conversation history
	addHistory(m.Author.ID, "user", content)
	addHistory(m.Author.ID, "assistant", answer)

	app.Log("discord", "Reply to %s: %.100s", m.Author.Username, answer)

	embed := formatAsEmbed(content, answer)
	sendEmbed(m.ChannelID, embed)
}

// ── Discord HTTP API ──

func getGatewayURL() (string, error) {
	req, _ := http.NewRequest("GET", "https://discord.com/api/v10/gateway", nil)
	req.Header.Set("Authorization", "Bot "+botToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		URL string `json:"url"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.URL, nil
}

func sendMessage(channelID, content string) {
	body, _ := json.Marshal(map[string]string{"content": content})
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/channels/"+channelID+"/messages", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Send message error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func showTyping(channelID string) {
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/channels/"+channelID+"/typing", nil)
	req.Header.Set("Authorization", "Bot "+botToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
