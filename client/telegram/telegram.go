// Package telegram connects Mu to Telegram as a bot. Users message the
// bot directly, and it runs the AI agent on their behalf.
//
// Setup:
//  1. Message @BotFather on Telegram, create a bot, get the token
//  2. Set TELEGRAM_BOT_TOKEN via /admin/env or env var
//
// Users are auto-created on first message (like Discord). Existing
// users can link with "link <username> <password>".
package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"
)

var (
	linkMu sync.RWMutex
	links  = map[string]string{} // telegram user ID → mu account ID

	historyMu sync.RWMutex
	histories = map[string][]agent.QueryMessage{} // telegram user ID → recent messages
)

const maxHistory = 10

func Load() {
	data.LoadJSON("telegram_links.json", &links)
	go run()
}

func Enabled() bool {
	return settings.Get("TELEGRAM_BOT_TOKEN") != ""
}

func run() {
	for {
		token := settings.Get("TELEGRAM_BOT_TOKEN")
		if token == "" {
			time.Sleep(30 * time.Second)
			continue
		}
		app.Log("telegram", "Bot starting with long polling")
		poll(token)
		app.Log("telegram", "Polling stopped, restarting in 5s")
		time.Sleep(5 * time.Second)
	}
}

var httpClient = &http.Client{Timeout: 35 * time.Second}

// registerCommands fills Telegram's command menu from the tool registry, so a
// tool added to the server shows up in the bot without anyone editing a list.
// One entry per service — the method is the first word after it, "/news list" —
// because Telegram commands are single words and twenty is a menu while seventy
// is a wall.
func registerCommands(token string) {
	// Wait for the registry: tools finish registering after the bot starts, and
	// a menu built too early is missing services.
	api.WaitForTools(30 * time.Second)

	commands := []map[string]string{
		{"command": "ask", "description": "Ask the AI agent anything"},
		{"command": "usage", "description": "Your usage stats"},
	}

	services := api.Services()
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !telegramCommand.MatchString(name) {
			continue
		}
		commands = append(commands, map[string]string{
			"command":     name,
			"description": serviceMenuText(name, services[name]),
		})
	}
	body, _ := json.Marshal(map[string]any{"commands": commands})
	url := "https://api.telegram.org/bot" + token + "/setMyCommands"
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	app.Log("telegram", "Registered bot commands")
}

func poll(token string) {
	go registerCommands(token)
	baseURL := "https://api.telegram.org/bot" + token
	offset := 0

	for {
		url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", baseURL, offset)
		resp, err := httpClient.Get(url)
		if err != nil {
			app.Log("telegram", "Poll error: %v", err)
			return
		}

		var result struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int `json:"update_id"`
				Message  *struct {
					MessageID int `json:"message_id"`
					From      struct {
						ID        int64  `json:"id"`
						Username  string `json:"username"`
						FirstName string `json:"first_name"`
					} `json:"from"`
					Chat struct {
						ID   int64  `json:"id"`
						Type string `json:"type"`
					} `json:"chat"`
					Text     string `json:"text"`
					Entities []struct {
						Type   string `json:"type"`
						Offset int    `json:"offset"`
						Length int    `json:"length"`
					} `json:"entities"`
				} `json:"message"`
			} `json:"result"`
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(body, &result)

		if !result.OK {
			app.Log("telegram", "API returned not OK")
			return
		}

		for _, update := range result.Result {
			offset = update.UpdateID + 1
			if update.Message == nil || update.Message.Text == "" {
				continue
			}
			m := update.Message

			// Check if this is a bot command or mention
			isBotCommand := false
			for _, e := range m.Entities {
				if e.Type == "bot_command" || e.Type == "mention" {
					isBotCommand = true
					break
				}
			}

			isDM := m.Chat.Type == "private"
			if !isDM && !isBotCommand {
				continue
			}

			go handleMessage(token, m.From.ID, m.From.Username, m.From.FirstName, m.Chat.ID, m.Chat.Type, m.Text)
		}
	}
}

func handleMessage(token string, userID int64, username, firstName string, chatID int64, chatType, text string) {
	telegramID := fmt.Sprintf("%d", userID)
	isDM := chatType == "private"

	// A tool named as a command runs as a command. Every tool is reachable
	// this way — "/news list", "/markets list --category stocks",
	// "/islam prayer" — with no model call and no guessing, which is the
	// difference between a bot with five commands and a bot with all of them.
	var tool *api.Tool
	var toolArgs map[string]any

	// Strip bot commands: /ask@botname query → query
	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text, " ", 2)
		cmd := strings.Split(parts[0], "@")[0] // remove @botname
		if t, args, ok := resolveTool(cmd, parts); ok {
			tool, toolArgs = t, args
		}
		switch cmd {
		case "/start":
			sendTelegram(token, chatID, "Hi! I'm Micro — your agent across news, mail, markets, weather, search and more. Ask me anything.\n\nIn groups, use /ask followed by your question.")
			return
		case "/ask", "/mu", "/agent":
			if len(parts) > 1 {
				text = parts[1]
			} else {
				sendTelegram(token, chatID, "Usage: `"+cmd+" your question here`")
				return
			}
		case "/news":
			text = "latest news"
		case "/markets":
			text = "crypto market prices"
		case "/weather":
			if len(parts) > 1 {
				text = "weather in " + parts[1]
			} else {
				text = "weather forecast"
			}
		case "/usage":
			text = "" // handled below
		default:
			if len(parts) > 1 {
				text = parts[1]
			} else {
				text = ""
			}
		}
	}

	// Strip @mentions
	text = strings.TrimSpace(text)
	// Remove @botname from text
	words := strings.Fields(text)
	var cleaned []string
	for _, w := range words {
		if !strings.HasPrefix(w, "@") {
			cleaned = append(cleaned, w)
		}
	}
	text = strings.Join(cleaned, " ")

	if text == "" {
		sendTelegram(token, chatID, "Ask me anything! In groups use `/ask your question`.")
		return
	}

	// Handle link command (DM only)
	if strings.HasPrefix(strings.ToLower(text), "link ") && isDM {
		parts := strings.Fields(text[5:])
		if len(parts) >= 2 {
			uname := parts[0]
			pass := strings.Join(parts[1:], " ")
			if _, err := auth.Login(uname, pass); err != nil {
				sendTelegram(token, chatID, "Invalid username or password.")
				return
			}
			linkAccount(telegramID, uname)
			sendTelegram(token, chatID, fmt.Sprintf("Linked to *%s*.", uname))
			return
		}
		sendTelegram(token, chatID, "Usage: `link <username> <password>`")
		return
	}

	if strings.ToLower(text) == "unlink" {
		linkMu.Lock()
		delete(links, telegramID)
		data.SaveJSON("telegram_links.json", links)
		linkMu.Unlock()
		sendTelegram(token, chatID, "Unlinked.")
		return
	}

	// Look up or auto-create account
	accountID := getLinkedAccount(telegramID)
	if accountID == "" {
		name := firstName
		if name == "" {
			name = username
		}
		accountID = autoCreateAccount(telegramID, username, name)
		if accountID == "" {
			sendTelegram(token, chatID, "Couldn't create your account. Try again later.")
			return
		}
		sendTelegram(token, chatID, fmt.Sprintf("Welcome! I've created your account *%s*. Ask me anything.", accountID))
	}

	app.Log("telegram", "Message from %s (%s): %s", username, accountID, text)

	// Send typing indicator
	sendAction(token, chatID, "typing")

	// A tool command answers directly.
	if tool != nil {
		out, isErr, err := api.ExecuteToolAs(accountID, tool.Name, toolArgs)
		if err != nil && out == "" {
			out = "That didn't work: " + err.Error()
		}
		if strings.TrimSpace(out) == "" {
			out = "No result."
		}
		if isErr {
			app.Log("telegram", "Tool %s failed for %s: %v", tool.Name, accountID, err)
		}
		sendTelegram(token, chatID, out)
		return
	}

	// Run agent with conversation context
	// Channel messages are public — no private data
	history := getHistory(telegramID)
	answer, err := agent.QueryWithOpts(accountID, text, agent.QueryOpts{
		History: history,
		Public:  !isDM,
	})
	if err != nil {
		app.Log("telegram", "Agent error for %s: %v", accountID, err)
		sendTelegram(token, chatID, "Sorry, something went wrong.")
		return
	}

	if strings.TrimSpace(answer) == "" {
		sendTelegram(token, chatID, "I couldn't generate a response. Try rephrasing.")
		return
	}

	addHistory(telegramID, "user", text)
	addHistory(telegramID, "assistant", answer)

	// Telegram has a 4096 char limit
	if len(answer) > 4000 {
		answer = answer[:4000] + "\n…"
	}

	sendTelegram(token, chatID, answer)
}

func sendTelegram(token string, chatID int64, text string) {
	body, _ := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		app.Log("telegram", "Send error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func sendAction(token string, chatID int64, action string) {
	body, _ := json.Marshal(map[string]any{
		"chat_id": chatID,
		"action":  action,
	})
	url := "https://api.telegram.org/bot" + token + "/sendChatAction"
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// NotifyUser sends a message to a user's linked Telegram account.
func NotifyUser(muAccountID, message string) {
	if !Enabled() {
		return
	}
	linkMu.RLock()
	var telegramID string
	for tid, mid := range links {
		if mid == muAccountID {
			telegramID = tid
			break
		}
	}
	linkMu.RUnlock()

	if telegramID == "" {
		return
	}

	token := settings.Get("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return
	}

	// Parse telegramID back to int64 for chat_id
	var chatID int64
	fmt.Sscanf(telegramID, "%d", &chatID)
	if chatID == 0 {
		return
	}
	sendTelegram(token, chatID, message)
}

// ── Account management ──

func linkAccount(telegramID, muAccount string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	links[telegramID] = muAccount
	data.SaveJSON("telegram_links.json", links)
}

func getLinkedAccount(telegramID string) string {
	linkMu.RLock()
	defer linkMu.RUnlock()
	return links[telegramID]
}

func DeleteLinks(muAccount string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	for k, v := range links {
		if v == muAccount {
			delete(links, k)
		}
	}
	data.SaveJSON("telegram_links.json", links)
}

func sanitizeAccountID(telegramID, username string) string {
	id := strings.ToLower(username)
	id = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, id)
	if len(id) < 4 {
		id = id + telegramID[len(telegramID)-min(4, len(telegramID)):]
	}
	if len(id) > 24 {
		id = id[:24]
	}
	return id
}

func uniqueAccountID(baseID string, exists func(string) bool) string {
	if !exists(baseID) {
		return baseID
	}
	for i := 1; i < 100; i++ {
		suffix := fmt.Sprintf("%d", i)
		prefixLen := 24 - len(suffix)
		if prefixLen <= 0 {
			return ""
		}
		candidate := baseID
		if len(candidate) > prefixLen {
			candidate = candidate[:prefixLen]
		}
		candidate += suffix
		if !exists(candidate) {
			return candidate
		}
	}
	return ""
}

func autoCreateAccount(telegramID, username, displayName string) string {
	id := uniqueAccountID(sanitizeAccountID(telegramID, username), func(id string) bool {
		_, err := auth.GetAccount(id)
		return err == nil
	})
	if id == "" {
		app.Log("telegram", "Auto-create failed for %s: no available account ID", username)
		return ""
	}

	passBytes := make([]byte, 16)
	if _, err := rand.Read(passBytes); err != nil {
		app.Log("telegram", "Auto-create failed for %s: %v", username, err)
		return ""
	}
	pass := hex.EncodeToString(passBytes)

	acc := &auth.Account{
		ID:      id,
		Name:    displayName,
		Secret:  pass,
		Created: time.Now(),
	}
	if err := auth.Create(acc); err != nil {
		app.Log("telegram", "Auto-create failed for %s: %v", username, err)
		return ""
	}

	linkAccount(telegramID, id)
	app.Log("telegram", "Auto-created account %s for Telegram user %s", id, username)
	return id
}

func getHistory(telegramID string) []agent.QueryMessage {
	historyMu.RLock()
	defer historyMu.RUnlock()
	return append([]agent.QueryMessage(nil), histories[telegramID]...)
}

func addHistory(telegramID string, role, text string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	histories[telegramID] = append(histories[telegramID], agent.QueryMessage{Role: role, Text: text})
	if len(histories[telegramID]) > maxHistory {
		histories[telegramID] = histories[telegramID][len(histories[telegramID])-maxHistory:]
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// resolveTool reads a slash command as a tool call: "/news list --limit 3".
//
// It answers false for anything the agent should handle instead — a command
// that is not a tool, words with nowhere to go, or a tool missing a required
// argument. "/weather" is the case that matters: it names a real tool that
// wants a latitude and a longitude, and someone typing it wants a forecast,
// not an error.
func resolveTool(cmd string, parts []string) (*api.Tool, map[string]any, bool) {
	words := []string{strings.TrimPrefix(cmd, "/")}
	if len(parts) > 1 {
		words = append(words, strings.Fields(parts[1])...)
	}

	t, rest, ok := api.Resolve(words)
	if !ok {
		return nil, nil, false
	}
	args, ok := api.Args(t, rest)
	if !ok || !api.Ready(t, args) {
		return nil, nil, false
	}
	return &t, args, true
}

// telegramCommand is what Telegram accepts as a command name: lowercase
// letters, digits and underscores, up to 32 characters.
var telegramCommand = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// serviceMenuText is the one line Telegram shows beside a command. It lists the
// methods, since that is what the person has to type next, and Telegram cuts
// anything past 256 characters.
func serviceMenuText(service string, tools []api.Tool) string {
	var methods []string
	for _, t := range tools {
		if m := api.Method(t.Name); m != "" {
			methods = append(methods, m)
		}
	}
	if len(methods) == 0 {
		// A one-word tool: its own description says it better than a list.
		if len(tools) > 0 {
			return truncateText(tools[0].Description, 250)
		}
		return service
	}
	return truncateText(strings.Join(methods, ", "), 250)
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
