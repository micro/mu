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
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"
)

var (
	linkMu   sync.RWMutex
	links    = map[string]string{} // telegram user ID → mu account ID

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

func poll(token string) {
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
					Text string `json:"text"`
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
			if update.Message != nil && update.Message.Text != "" {
				go handleMessage(token, update.Message.From.ID, update.Message.From.Username, update.Message.From.FirstName, update.Message.Chat.ID, update.Message.Chat.Type, update.Message.Text)
			}
		}
	}
}

func handleMessage(token string, userID int64, username, firstName string, chatID int64, chatType, text string) {
	telegramID := fmt.Sprintf("%d", userID)
	isDM := chatType == "private"

	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	// Strip /start command
	if text == "/start" {
		sendTelegram(token, chatID, "Hi! I'm Micro — your personal AI. Ask me anything.")
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

func autoCreateAccount(telegramID, username, displayName string) string {
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

	baseID := id
	for i := 0; i < 100; i++ {
		if _, err := auth.GetAccount(id); err != nil {
			break
		}
		id = fmt.Sprintf("%s%d", baseID, i+1)
		if len(id) > 24 {
			id = id[:24]
		}
	}

	passBytes := make([]byte, 16)
	rand.Read(passBytes)
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
	return histories[telegramID]
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
