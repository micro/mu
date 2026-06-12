// Package whatsapp connects Mu to WhatsApp via the Business Cloud API.
// Users message the bot number, and it runs the AI agent on their behalf.
//
// Setup:
//  1. Create a Meta Business account and app at developers.facebook.com
//  2. Add WhatsApp to your app, get a phone number ID and access token
//  3. Set WHATSAPP_TOKEN, WHATSAPP_PHONE_ID, WHATSAPP_VERIFY_TOKEN
//     via /admin/env
//  4. Configure the webhook URL in Meta Developer Portal:
//     https://your-domain.com/whatsapp/webhook
//
// Users are auto-created on first message. Existing users can link
// with "link <username> <password>".
package whatsapp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

const apiBase = "https://graph.facebook.com/v21.0"

var (
	linkMu    sync.RWMutex
	links     = map[string]string{} // whatsapp phone number → mu account ID

	historyMu sync.RWMutex
	histories = map[string][]agent.QueryMessage{}
)

const maxHistory = 10

func Load() {
	data.LoadJSON("whatsapp_links.json", &links)
}

func Enabled() bool {
	return settings.Get("WHATSAPP_TOKEN") != "" && settings.Get("WHATSAPP_PHONE_ID") != ""
}

// Handler handles the WhatsApp webhook at /whatsapp/webhook.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleVerify(w, r)
	case "POST":
		handleWebhook(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleVerify handles the webhook verification challenge from Meta.
func handleVerify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	verifyToken := settings.Get("WHATSAPP_VERIFY_TOKEN")
	if mode == "subscribe" && token == verifyToken {
		app.Log("whatsapp", "Webhook verified")
		w.WriteHeader(200)
		w.Write([]byte(challenge))
		return
	}
	app.Log("whatsapp", "Webhook verification failed")
	w.WriteHeader(403)
}

// handleWebhook processes incoming messages from WhatsApp.
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Always return 200 quickly to acknowledge receipt
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(200)
		return
	}

	// Verify signature if app secret is set
	if secret := settings.Get("WHATSAPP_APP_SECRET"); secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, sig, secret) {
			app.Log("whatsapp", "Invalid webhook signature")
			w.WriteHeader(200)
			return
		}
	}

	w.WriteHeader(200)

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" || msg.Text.Body == "" {
					continue
				}
				isGroup := msg.GroupID != ""
				replyTo := ""
				if msg.Context != nil {
					replyTo = msg.Context.From
				}
				go handleMessage(msg.From, msg.Text.Body, isGroup, replyTo)
			}
		}
	}
}

type webhookPayload struct {
	Entry []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Messages []struct {
					From    string `json:"from"`
					Type    string `json:"type"`
					Text    struct {
						Body string `json:"body"`
					} `json:"text"`
					Context *struct {
						From string `json:"from"`
					} `json:"context"`
					GroupID string `json:"group_id"`
				} `json:"messages"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func handleMessage(from, text string, isGroup bool, replyTo string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	// In groups, only respond to:
	// 1. Messages starting with "mu " or "/ask "
	// 2. Replies to the bot's messages
	if isGroup {
		lower := strings.ToLower(text)
		botPhoneID := settings.Get("WHATSAPP_PHONE_ID")
		isReplyToBot := replyTo == botPhoneID
		hasTrigger := strings.HasPrefix(lower, "mu ") ||
			strings.HasPrefix(lower, "/ask ") ||
			strings.HasPrefix(lower, "@mu ") ||
			lower == "mu" || lower == "/ask"

		if !hasTrigger && !isReplyToBot {
			return
		}

		// Strip prefix
		if strings.HasPrefix(lower, "mu ") {
			text = strings.TrimSpace(text[3:])
		} else if strings.HasPrefix(lower, "/ask ") {
			text = strings.TrimSpace(text[5:])
		} else if strings.HasPrefix(lower, "@mu ") {
			text = strings.TrimSpace(text[4:])
		}
		if text == "" {
			sendMessage(from, "Ask me anything! Start with *mu* followed by your question.")
			return
		}
	}

	// Handle link command (DMs only)
	if !isGroup && strings.HasPrefix(strings.ToLower(text), "link ") {
		parts := strings.Fields(text[5:])
		if len(parts) >= 2 {
			username := parts[0]
			password := strings.Join(parts[1:], " ")
			if _, err := auth.Login(username, password); err != nil {
				sendMessage(from, "Invalid username or password.")
				return
			}
			linkAccount(from, username)
			sendMessage(from, fmt.Sprintf("Linked to *%s*.", username))
			return
		}
		sendMessage(from, "Usage: link <username> <password>")
		return
	}

	if strings.ToLower(text) == "unlink" {
		linkMu.Lock()
		delete(links, from)
		data.SaveJSON("whatsapp_links.json", links)
		linkMu.Unlock()
		sendMessage(from, "Unlinked.")
		return
	}

	// Look up or auto-create account
	accountID := getLinkedAccount(from)
	if accountID == "" {
		accountID = autoCreateAccount(from)
		if accountID == "" {
			sendMessage(from, "Couldn't create your account. Try again later.")
			return
		}
		sendMessage(from, fmt.Sprintf("Welcome! I've created your account *%s*. Ask me anything.", accountID))
	}

	app.Log("whatsapp", "Message from %s (%s, group=%v): %s", from, accountID, isGroup, text)

	// Groups are public context, DMs are private
	history := getHistory(from)
	answer, err := agent.QueryWithOpts(accountID, text, agent.QueryOpts{
		History: history,
		Public:  isGroup,
	})
	if err != nil {
		app.Log("whatsapp", "Agent error for %s: %v", accountID, err)
		sendMessage(from, "Sorry, something went wrong.")
		return
	}

	if strings.TrimSpace(answer) == "" {
		sendMessage(from, "I couldn't generate a response. Try rephrasing.")
		return
	}

	addHistory(from, "user", text)
	addHistory(from, "assistant", answer)

	// WhatsApp has a 4096 char limit
	if len(answer) > 4000 {
		answer = answer[:4000] + "\n…"
	}

	sendMessage(from, answer)
}

func sendMessage(to, text string) {
	token := settings.Get("WHATSAPP_TOKEN")
	phoneID := settings.Get("WHATSAPP_PHONE_ID")
	if token == "" || phoneID == "" {
		return
	}

	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})

	url := fmt.Sprintf("%s/%s/messages", apiBase, phoneID)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("whatsapp", "Send error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		app.Log("whatsapp", "Send failed with status %d", resp.StatusCode)
	}
}

// NotifyUser sends a message to a user's linked WhatsApp number.
func NotifyUser(muAccountID, message string) {
	if !Enabled() {
		return
	}
	linkMu.RLock()
	var phone string
	for p, mid := range links {
		if mid == muAccountID {
			phone = p
			break
		}
	}
	linkMu.RUnlock()

	if phone == "" {
		return
	}
	sendMessage(phone, message)
}

// ── Account management ──

func linkAccount(phone, muAccount string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	links[phone] = muAccount
	data.SaveJSON("whatsapp_links.json", links)
}

func getLinkedAccount(phone string) string {
	linkMu.RLock()
	defer linkMu.RUnlock()
	return links[phone]
}

func DeleteLinks(muAccount string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	for k, v := range links {
		if v == muAccount {
			delete(links, k)
		}
	}
	data.SaveJSON("whatsapp_links.json", links)
}

func autoCreateAccount(phone string) string {
	// Use last 6 digits of phone as username base
	id := "wa" + phone
	if len(id) > 10 {
		id = "wa" + phone[len(phone)-6:]
	}

	baseID := id
	for i := 0; i < 100; i++ {
		if _, err := auth.GetAccount(id); err != nil {
			break
		}
		id = fmt.Sprintf("%s%d", baseID, i+1)
	}

	passBytes := make([]byte, 16)
	rand.Read(passBytes)
	pass := hex.EncodeToString(passBytes)

	acc := &auth.Account{
		ID:      id,
		Name:    "WhatsApp User",
		Secret:  pass,
		Created: time.Now(),
	}
	if err := auth.Create(acc); err != nil {
		app.Log("whatsapp", "Auto-create failed for %s: %v", phone, err)
		return ""
	}

	linkAccount(phone, id)
	app.Log("whatsapp", "Auto-created account %s for WhatsApp %s", id, phone)
	return id
}

func getHistory(phone string) []agent.QueryMessage {
	historyMu.RLock()
	defer historyMu.RUnlock()
	return histories[phone]
}

func addHistory(phone string, role, text string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	histories[phone] = append(histories[phone], agent.QueryMessage{Role: role, Text: text})
	if len(histories[phone]) > maxHistory {
		histories[phone] = histories[phone][len(histories[phone])-maxHistory:]
	}
}

func verifySignature(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}
