package chat

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/admin"
	"mu/app"
	"mu/auth"
	"mu/data"
	"mu/wallet"
)

//go:embed *.json
var f embed.FS

type Prompt struct {
	System   string   `json:"system"`   // System prompt override
	Topic    string   `json:"topic"`    // User-selected topic/context
	Rag      []string `json:"rag"`
	Context  History  `json:"context"`
	Question string   `json:"question"`
	Priority int      `json:"priority"` // 0=high (chat), 1=medium, 2=low (background)
}

type History []Message

// message history
type Message struct {
	Prompt string
	Answer string
}

var Template = `
<div id="topic-selector">
  <div class="topic-tabs">%s</div>
</div>
<div id="messages"></div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="topic" name="topic" type="hidden">
<input id="prompt" name="prompt" type="text" placeholder="Ask a question" autocomplete=off>
<button>Send</button>
</form>
<div id="chat-back-link" style="margin-top: 20px; text-align: center; display: none;">
  <a href="#" onclick="window.history.back(); return false;" style="color: #666; text-decoration: none;">← Back</a>
</div>`

var mutex sync.RWMutex

var prompts = map[string]string{}

var summaries = map[string]string{}

var topics = []string{}

var head string

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// Room represents a discussion room for a specific item.
// Room state is ephemeral - messages exist only in memory while the server runs.
// The last 20 messages are kept in memory for new joiners.
// Client-side sessionStorage is used so participants see their conversation until they leave.
type Room struct {
	ID           string                      // e.g., "post_123", "news_456", "video_789"
	Type         string                      // "post", "news", "video"
	Title        string                      // Item title
	Summary      string                      // Item summary/description
	URL          string                      // Original item URL
	Topic        string                      // News topic (e.g., "Dev", "World", etc.)
	LastRefresh  time.Time                   // Last time external content was refreshed
	LastActivity time.Time                   // Last time room had any activity (for cleanup)
	LastAIMsg    time.Time                   // Last time AI sent an auto-message
	Messages     []RoomMessage               // Last 20 messages (in-memory only)
	Clients      map[*websocket.Conn]*Client // Connected clients
	Broadcast    chan RoomMessage            // Broadcast channel
	Register     chan *Client                // Register client
	Unregister   chan *Client                // Unregister client
	Shutdown     chan bool                   // Signal for graceful shutdown
	mutex        sync.RWMutex
}

// RoomMessage represents a message in a chat room
type RoomMessage struct {
	UserID    string    `json:"username"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsLLM     bool      `json:"is_llm"`
}

// Client represents a connected websocket client
type Client struct {
	Conn          *websocket.Conn
	UserID        string
	Room          *Room
	InMicroConvo  bool      // true if user started a conversation with @micro
	LastMicroReply time.Time // when micro last replied to this user
}

var rooms = make(map[string]*Room)
var roomsMutex sync.RWMutex

// saveRoomMessages persists room messages to disk
func saveRoomMessages(roomID string, messages []RoomMessage) {
	filename := "room_" + strings.ReplaceAll(roomID, "/", "_") + ".json"
	b, err := json.Marshal(messages)
	if err != nil {
		app.Log("chat", "Error marshaling room messages: %v", err)
		return
	}
	if err := data.SaveFile(filename, string(b)); err != nil {
		app.Log("chat", "Error saving room messages: %v", err)
	}
}

// loadRoomMessages loads persisted room messages from disk
func loadRoomMessages(roomID string) []RoomMessage {
	filename := "room_" + strings.ReplaceAll(roomID, "/", "_") + ".json"
	b, err := data.LoadFile(filename)
	if err != nil {
		return nil
	}
	var messages []RoomMessage
	if err := json.Unmarshal(b, &messages); err != nil {
		app.Log("chat", "Error unmarshaling room messages: %v", err)
		return nil
	}
	app.Log("chat", "Loaded %d messages for room %s", len(messages), roomID)
	return messages
}

// handlePatternMatch handles predictable queries with direct lookups, skipping LLM
func handlePatternMatch(content string, room *Room) string {
	contentLower := strings.ToLower(strings.TrimSpace(content))
	// Remove @micro mention for pattern matching
	contentLower = strings.ReplaceAll(contentLower, "@micro", "")
	contentLower = strings.TrimSpace(contentLower)

	// Price patterns: "btc price", "price of btc", "eth price", "what is btc", etc.
	pricePatterns := []struct {
		patterns []string
		symbol   string
		name     string
	}{
		{[]string{"btc price", "bitcoin price", "price of btc", "price of bitcoin", "what is btc", "what's btc", "how much is btc", "how much is bitcoin"}, "BTC", "Bitcoin"},
		{[]string{"eth price", "ethereum price", "price of eth", "price of ethereum", "what is eth", "what's eth", "how much is eth", "how much is ethereum"}, "ETH", "Ethereum"},
		{[]string{"gold price", "price of gold", "what is gold", "how much is gold", "xau price"}, "XAU", "Gold"},
		{[]string{"silver price", "price of silver", "what is silver", "how much is silver", "xag price"}, "XAG", "Silver"},
		{[]string{"sol price", "solana price", "price of sol", "price of solana", "what is sol", "how much is sol"}, "SOL", "Solana"},
		{[]string{"doge price", "dogecoin price", "price of doge", "what is doge", "how much is doge"}, "DOGE", "Dogecoin"},
	}

	for _, p := range pricePatterns {
		for _, pattern := range p.patterns {
			if contentLower == pattern || strings.HasPrefix(contentLower, pattern+" ") || strings.HasSuffix(contentLower, " "+pattern) {
				// Look up price from data index
				entry := data.GetByID("market_" + p.symbol)
				if entry != nil {
					if price, ok := entry.Metadata["price"].(float64); ok {
						if price >= 1000 {
							return fmt.Sprintf("%s (%s) is currently **$%.2f**", p.name, p.symbol, price)
						} else if price >= 1 {
							return fmt.Sprintf("%s (%s) is currently **$%.2f**", p.name, p.symbol, price)
						} else {
							return fmt.Sprintf("%s (%s) is currently **$%.4f**", p.name, p.symbol, price)
						}
					}
				}
				return fmt.Sprintf("I don't have current price data for %s", p.name)
			}
		}
	}

	// Generic "X price" pattern - try to match any symbol
	if strings.HasSuffix(contentLower, " price") {
		symbol := strings.ToUpper(strings.TrimSuffix(contentLower, " price"))
		if len(symbol) >= 2 && len(symbol) <= 6 {
			entry := data.GetByID("market_" + symbol)
			if entry != nil {
				if price, ok := entry.Metadata["price"].(float64); ok {
					if price >= 1000 {
						return fmt.Sprintf("%s is currently **$%.2f**", symbol, price)
					} else if price >= 1 {
						return fmt.Sprintf("%s is currently **$%.2f**", symbol, price)
					} else {
						return fmt.Sprintf("%s is currently **$%.4f**", symbol, price)
					}
				}
			}
		}
	}

	// "price of X" pattern
	if strings.HasPrefix(contentLower, "price of ") {
		symbol := strings.ToUpper(strings.TrimPrefix(contentLower, "price of "))
		if len(symbol) >= 2 && len(symbol) <= 6 {
			entry := data.GetByID("market_" + symbol)
			if entry != nil {
				if price, ok := entry.Metadata["price"].(float64); ok {
					if price >= 1000 {
						return fmt.Sprintf("%s is currently **$%.2f**", symbol, price)
					} else if price >= 1 {
						return fmt.Sprintf("%s is currently **$%.2f**", symbol, price)
					} else {
						return fmt.Sprintf("%s is currently **$%.4f**", symbol, price)
					}
				}
			}
		}
	}

	return "" // No pattern match
}

// isMoreInfoRequest checks if user is asking for more details about something
func isMoreInfoRequest(content string) bool {
	contentLower := strings.ToLower(content)
	moreInfoPhrases := []string{
		"more detail", "tell me more", "more info", "full article",
		"read more", "expand on", "elaborate", "further info",
		"what else", "anything else", "more about", "go deeper",
		"full story", "complete story", "more information",
		"can you explain more", "explain further", "dig deeper",
	}
	for _, phrase := range moreInfoPhrases {
		if strings.Contains(contentLower, phrase) {
			return true
		}
	}
	return false
}

// extractURLsFromContext finds URLs from recent RAG context or room messages
// Returns both external URLs (http/https) and internal URLs (/video, /news, etc.)
func extractURLsFromContext(room *Room, ragContext []string) []string {
	var urls []string
	seen := make(map[string]bool)
	
	// Check room URL first
	if room.URL != "" {
		if strings.HasPrefix(room.URL, "http") || strings.HasPrefix(room.URL, "/") {
			urls = append(urls, room.URL)
			seen[room.URL] = true
			app.Log("chat", "extractURLs: found room URL: %s", room.URL)
		}
	}
	
	// Extract URLs from context strings (Source: url)
	for _, ctx := range ragContext {
		if idx := strings.Index(ctx, "(Source: "); idx != -1 {
			end := strings.Index(ctx[idx:], ")")
			if end != -1 {
				url := ctx[idx+9 : idx+end]
				if !seen[url] && (strings.HasPrefix(url, "http") || strings.HasPrefix(url, "/")) {
					urls = append(urls, url)
					seen[url] = true
					app.Log("chat", "extractURLs: found context URL: %s", url)
				}
			}
		}
	}
	
	// Also check recent room messages for URLs (in case user is discussing a specific item)
	room.mutex.RLock()
	for _, msg := range room.Messages {
		// Look for http/https URLs in message content
		words := strings.Fields(msg.Content)
		for _, word := range words {
			if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
				// Clean up punctuation at end
				word = strings.TrimRight(word, ".,;:!?)")
				if !seen[word] {
					urls = append(urls, word)
					seen[word] = true
					app.Log("chat", "extractURLs: found message URL: %s", word)
				}
			}
		}
	}
	room.mutex.RUnlock()
	
	app.Log("chat", "extractURLs: total URLs found: %d", len(urls))
	return urls
}

// getContentFromURL gets content from a URL - uses index for internal URLs, HTTP fetch for external
// getContentFromURL gets content from a URL - uses index for internal URLs, HTTP fetch for external
func getContentFromURL(url string) (string, error) {
	// Handle internal URLs by looking up in index
	if strings.HasPrefix(url, "/") {
		// Parse internal URL to get ID
		// e.g., /video?id=xxx -> video_xxx, /news?id=xxx -> news_xxx
		var entryID string
		if strings.HasPrefix(url, "/video?id=") {
			entryID = "video_" + strings.TrimPrefix(url, "/video?id=")
		} else if strings.HasPrefix(url, "/news?id=") {
			entryID = strings.TrimPrefix(url, "/news?id=")
		} else if strings.HasPrefix(url, "/post?id=") {
			entryID = strings.TrimPrefix(url, "/post?id=")
		}
		
		if entryID != "" {
			entry := data.GetByID(entryID)
			if entry != nil {
				content := entry.Title + "\n\n" + entry.Content
				app.Log("chat", "getContentFromURL: found internal content for %s (%d chars)", url, len(content))
				return content, nil
			}
			app.Log("chat", "getContentFromURL: internal URL %s not found in index (tried ID: %s)", url, entryID)
		}
		return "", fmt.Errorf("internal URL not found: %s", url)
	}
	
	// External URL - fetch via HTTP
	return fetchURLContent(url)
}

// fetchURLContent fetches and extracts text content from a URL
func fetchURLContent(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuBot/1.0)")
	
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	
	// Extract text content (simple approach - strip HTML tags)
	content := string(body)
	
	// Remove script and style tags with content
	for _, tag := range []string{"script", "style", "noscript", "head"} {
		for {
			start := strings.Index(strings.ToLower(content), "<"+tag)
			if start == -1 {
				break
			}
			end := strings.Index(strings.ToLower(content[start:]), "</"+tag+">")
			if end == -1 {
				end = strings.Index(content[start:], ">")
				if end != -1 {
					content = content[:start] + content[start+end+1:]
				} else {
					break
				}
			} else {
				content = content[:start] + content[start+end+len("</"+tag+">"):]
			}
		}
	}
	
	// Remove remaining HTML tags
	var result strings.Builder
	inTag := false
	for _, r := range content {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
			result.WriteRune(' ')
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	
	// Clean up whitespace
	text := result.String()
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 20 { // Skip very short lines (likely nav elements)
			cleanLines = append(cleanLines, line)
		}
	}
	
	text = strings.Join(cleanLines, "\n")
	
	// Limit to reasonable size
	if len(text) > 8000 {
		text = text[:8000] + "..."
	}
	
	return text, nil
}

// getOrCreateRoom gets an existing room or creates a new one
func getOrCreateRoom(id string) *Room {
	start := time.Now()
	app.Log("chat", "[getOrCreateRoom] Start for %s", id)

	// Check if room exists first (fast path with read lock)
	roomsMutex.RLock()
	if room, exists := rooms[id]; exists {
		roomsMutex.RUnlock()
		app.Log("chat", "[getOrCreateRoom] Found existing room %s (took %v)", id, time.Since(start))
		return room
	}
	roomsMutex.RUnlock()
	app.Log("chat", "[getOrCreateRoom] Room %s not found, creating new (took %v so far)", id, time.Since(start))

	// Parse the ID to determine type and fetch item details
	parts := strings.SplitN(id, "_", 2)
	if len(parts) != 2 {
		return nil
	}

	itemType := parts[0]
	itemID := parts[1]

	// Create room structure (outside any locks)
	room := &Room{
		ID:           id,
		Type:         itemType,
		Clients:      make(map[*websocket.Conn]*Client),
		Broadcast:    make(chan RoomMessage, 256),
		Register:     make(chan *Client),
		Unregister:   make(chan *Client),
		Shutdown:     make(chan bool),
		Messages:     make([]RoomMessage, 0, 20),
		LastActivity: time.Now(),
	}

	// Fetch item details based on type (OUTSIDE roomsMutex to avoid deadlocks)
	switch itemType {
	case "post":
		// For posts, lookup by exact ID from index (posts are now indexed)
		app.Log("chat", "Attempting to get post %s from index", itemID)

		// Try with a timeout to avoid blocking during heavy indexing
		entryChan := make(chan *data.IndexEntry, 1)
		go func() {
			entryChan <- data.GetByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up post %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting post %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "Post Discussion"
			room.Summary = "Loading post content..."
			room.URL = "/post?id=" + itemID
			break
		}

		if entry != nil {
			room.Title = entry.Title
			if room.Title == "" {
				room.Title = "Untitled Post"
			}
			room.Summary = entry.Content
			if len(room.Summary) > 2000 {
				room.Summary = room.Summary[:2000] + "..."
			}
			room.URL = "/post?id=" + itemID
			app.Log("chat", "Room context - Title: %s, Summary length: %d, URL: %s", room.Title, len(room.Summary), room.URL)
		} else if room.Title == "" {
			app.Log("chat", "Post %s not found in index", itemID)
			room.Title = "Post Discussion"
			room.URL = "/post?id=" + itemID
		}
	case "news":
		// For news, lookup by exact ID
		app.Log("chat", "Attempting to get news item %s from index", itemID)

		// Try with a timeout to avoid blocking during heavy indexing
		entryChan := make(chan *data.IndexEntry, 1)
		go func() {
			entryChan <- data.GetByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up news item %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting news %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "News Discussion"
			room.Summary = "Loading article content..."
			break
		}

		if entry != nil {
			room.Title = entry.Title
			room.Summary = entry.Content
			if len(room.Summary) > 2000 {
				room.Summary = room.Summary[:2000] + "..."
			}
			if url, ok := entry.Metadata["url"].(string); ok {
				room.URL = url
			}
			app.Log("chat", "Room context - Title: %s, Summary length: %d, URL: %s", room.Title, len(room.Summary), room.URL)
		} else {
			if room.Title == "" {
				app.Log("chat", "News item %s not found in index", itemID)
				room.Title = "News Discussion"
			}
			// If entry not found but we have a title, log it
			app.Log("chat", "News item %s not indexed yet, using title only: %s", itemID, room.Title)
		}
	case "video":
		// For videos, lookup by exact ID
		app.Log("chat", "Attempting to get video item %s from index", itemID)

		// Try with a timeout to avoid blocking during heavy indexing
		entryChan := make(chan *data.IndexEntry, 1)
		go func() {
			entryChan <- data.GetByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up video item %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting video %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "Video Discussion"
			room.Summary = "Loading video content..."
			break
		}

		if entry != nil {
			room.Title = entry.Title
			room.Summary = entry.Content
			if len(room.Summary) > 2000 {
				room.Summary = room.Summary[:2000] + "..."
			}
			if url, ok := entry.Metadata["url"].(string); ok {
				room.URL = url
			}
			app.Log("chat", "Room context - Title: %s, Summary length: %d, URL: %s", room.Title, len(room.Summary), room.URL)
		} else if room.Title == "" {
			app.Log("chat", "Video item %s not found in index", itemID)
			room.Title = "Video Discussion"
		}
	case "chat":
		// For chat topics, use the topic name from summaries
		room.Title = itemID + " Discussion"
		mutex.RLock()
		if summary, exists := summaries[itemID]; exists {
			room.Summary = summary
		} else {
			room.Summary = "General discussion about " + itemID
		}
		mutex.RUnlock()
		room.Topic = itemID
		// Load persisted messages
		if saved := loadRoomMessages(id); saved != nil {
			room.Messages = saved
			// Find last AI message time to prevent duplicate greetings
			for i := len(saved) - 1; i >= 0; i-- {
				if saved[i].IsLLM {
					room.LastAIMsg = saved[i].Timestamp
					break
				}
			}
		}
		app.Log("chat", "Created chat room for topic: %s (lastAI: %v)", itemID, room.LastAIMsg)
	}

	// Now acquire write lock only for the map update
	roomsMutex.Lock()
	// Check again if another goroutine created it while we were fetching data
	if existingRoom, exists := rooms[id]; exists {
		roomsMutex.Unlock()
		app.Log("chat", "[getOrCreateRoom] Race - room %s created by another goroutine (total time %v)", id, time.Since(start))
		return existingRoom
	}
	rooms[id] = room
	roomsMutex.Unlock()

	// Subscribe to index complete events via channel
	go func() {
		sub := data.Subscribe(data.EventIndexComplete)
		defer sub.Close()

		// Wait for either index event or timeout
		timeout := time.After(5 * time.Second)

		for {
			select {
			case event, ok := <-sub.Chan:
				if !ok {
					// Channel closed
					return
				}
				if itemID, ok := event.Data["id"].(string); ok {
					// Check if this is our room's item
					parts := strings.SplitN(room.ID, "_", 2)
					if len(parts) == 2 && parts[1] == itemID {
						// Fetch updated entry
						entry := data.GetByID(itemID)
						if entry != nil {
							room.mutex.Lock()
							room.Title = entry.Title
							room.Summary = entry.Content
							if len(room.Summary) > 2000 {
								room.Summary = room.Summary[:2000] + "..."
							}
							if url, ok := entry.Metadata["url"].(string); ok {
								room.URL = url
							}
							room.mutex.Unlock()
							app.Log("chat", "Updated room %s context from index event", room.ID)
							return // Got content, done
						}
					}
				}
				// Not our item, keep waiting

			case <-timeout:
				// Fallback: Try fetching directly
				room.mutex.RLock()
				hasContent := room.Summary != "" && room.Summary != "Loading article content..." &&
					room.Summary != "Loading post content..." && room.Summary != "Loading video content..."
				room.mutex.RUnlock()

				if !hasContent {
					app.Log("chat", "Room %s still has no content after 5s, attempting direct fetch", room.ID)
					parts := strings.SplitN(room.ID, "_", 2)
					if len(parts) == 2 {
						entry := data.GetByID(parts[1])
						if entry != nil {
							room.mutex.Lock()
							room.Title = entry.Title
							room.Summary = entry.Content
							if len(room.Summary) > 2000 {
								room.Summary = room.Summary[:2000] + "..."
							}
							if url, ok := entry.Metadata["url"].(string); ok {
								room.URL = url
							}
							room.mutex.Unlock()
							app.Log("chat", "Updated room %s context via fallback", room.ID)
						} else {
							app.Log("chat", "Room %s item still not indexed after 5s", room.ID)
						}
					}
				}
				return // Done after timeout
			}
		}
	}()

	go room.run()
	room.startAIAutoResponse()

	app.Log("chat", "[getOrCreateRoom] Created room %s (total time %v)", id, time.Since(start))
	return room
}

// broadcastUserList sends the current list of usernames to all clients
func (room *Room) broadcastUserList() {
	room.mutex.RLock()
	usernames := make([]string, 0, len(room.Clients)+1)
	for _, client := range room.Clients {
		usernames = append(usernames, client.UserID)
	}
	room.mutex.RUnlock()

	// Always include micro in topic chat rooms
	if strings.HasPrefix(room.ID, "chat_") {
		usernames = append(usernames, "micro")
	}

	userListMsg := map[string]interface{}{
		"type":  "user_list",
		"users": usernames,
	}

	room.mutex.RLock()
	for conn := range room.Clients {
		conn.WriteJSON(userListMsg)
	}
	room.mutex.RUnlock()
}

// run handles the chat room message broadcasting
func (room *Room) run() {
	for {
		select {
		case <-room.Shutdown:
			// Graceful shutdown - close all client connections
			room.mutex.Lock()
			for conn := range room.Clients {
				conn.Close()
			}
			room.Clients = make(map[*websocket.Conn]*Client)
			room.mutex.Unlock()
			app.Log("chat", "Room %s shut down", room.ID)
			return

		case client := <-room.Register:
			room.mutex.Lock()
			room.Clients[client.Conn] = client
			room.LastActivity = time.Now()
			room.mutex.Unlock()

			// Broadcast updated user list
			room.broadcastUserList()

		case client := <-room.Unregister:
			room.mutex.Lock()
			if _, ok := room.Clients[client.Conn]; ok {
				delete(room.Clients, client.Conn)
				client.Conn.Close()
			}
			room.LastActivity = time.Now()
			room.mutex.Unlock()

			// Broadcast updated user list
			room.broadcastUserList()

		case message := <-room.Broadcast:
			// Add message to history (keep last 20)
			room.mutex.Lock()
			room.Messages = append(room.Messages, message)
			if len(room.Messages) > 20 {
				room.Messages = room.Messages[len(room.Messages)-20:]
			}
			room.LastActivity = time.Now()
			messagesToSave := make([]RoomMessage, len(room.Messages))
			copy(messagesToSave, room.Messages)
			room.mutex.Unlock()

			// Persist messages for topic chat rooms
			if strings.HasPrefix(room.ID, "chat_") {
				go saveRoomMessages(room.ID, messagesToSave)
			}

			// Broadcast to all clients
			room.mutex.RLock()
			for conn := range room.Clients {
				err := conn.WriteJSON(message)
				if err != nil {
					conn.Close()
					delete(room.Clients, conn)
				}
			}
			room.mutex.RUnlock()
		}
	}
}

// startAIAutoResponse starts a goroutine that sends AI messages when topic rooms are quiet
func (room *Room) startAIAutoResponse() {
	// Only for topic chat rooms
	if !strings.HasPrefix(room.ID, "chat_") {
		return
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-room.Shutdown:
				return
			case <-ticker.C:
				room.mutex.RLock()
				numClients := len(room.Clients)
				lastActivity := room.LastActivity
				lastAI := room.LastAIMsg
				numMessages := len(room.Messages)
				room.mutex.RUnlock()

				// Only trigger if:
				// - There are users in the room
				// - Room has been quiet for 2+ minutes
				// - AI hasn't spoken in last 10 minutes
				// - Room has no messages yet (first greeting only)
				if numClients > 0 &&
					time.Since(lastActivity) > 2*time.Minute &&
					time.Since(lastAI) > 10*time.Minute &&
					numMessages == 0 {

					room.sendAIGreeting()
				}
			}
		}
	}()
}

// sendAIGreeting sends a conversation-starting message from AI
func (room *Room) sendAIGreeting() {
	topicName := strings.TrimPrefix(room.ID, "chat_")

	// Get the topic summary if available
	mutex.RLock()
	summary := summaries[topicName]
	mutex.RUnlock()

	var prompt *Prompt
	if summary != "" {
		prompt = &Prompt{
			System:   "You are a friendly chat participant in a " + topicName + " discussion room. Start a brief, engaging conversation based on the current summary. Ask a thought-provoking question or share an interesting observation. Keep it to 1-2 sentences. Be conversational, not formal.",
			Question: "Current " + topicName + " summary: " + summary + "\n\nStart a conversation:",
			Priority: PriorityLow,
		}
	} else {
		prompt = &Prompt{
			System:   "You are a friendly chat participant in a " + topicName + " discussion room. Start a brief, engaging conversation about " + topicName + ". Ask a thought-provoking question or share an interesting observation. Keep it to 1-2 sentences. Be conversational, not formal.",
			Question: "Start a conversation about " + topicName + ":",
			Priority: PriorityLow,
		}
	}

	resp, err := askLLM(prompt)
	if err != nil || resp == "" {
		app.Log("chat", "AI greeting failed for room %s: %v", room.ID, err)
		return
	}

	msg := RoomMessage{
		UserID:    "micro",
		Content:   resp,
		Timestamp: time.Now(),
		IsLLM:     true,
	}

	room.mutex.Lock()
	room.LastAIMsg = time.Now()
	room.mutex.Unlock()

	room.Broadcast <- msg
	app.Log("chat", "AI greeting sent to room %s", room.ID)
}

// handleWebSocket handles WebSocket connections for chat rooms
func handleWebSocket(w http.ResponseWriter, r *http.Request, room *Room) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.Log("chat", "WebSocket upgrade error: %v", err)
		return
	}

	// Get user session
	sess, err := auth.GetSession(r)
	if err != nil {
		conn.Close()
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if acc == nil || err != nil {
		conn.Close()
		return
	}

	client := &Client{
		Conn:   conn,
		UserID: acc.ID,
		Room:   room,
	}

	room.Register <- client

	// Send room history to new client
	room.mutex.RLock()
	for _, msg := range room.Messages {
		conn.WriteJSON(msg)
	}
	room.mutex.RUnlock()

	// Read messages from client
	go func() {
		defer func() {
			room.Unregister <- client
		}()

		for {
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				break
			}

			if content, ok := msg["content"].(string); ok && len(content) > 0 {
				// Broadcast user message
				userMsg := RoomMessage{
					UserID:    client.UserID,
					Content:   content,
					Timestamp: time.Now(),
					IsLLM:     false,
				}
				room.Broadcast <- userMsg

				// Check if micro should respond:
				// 1. User mentioned @micro - start conversation
				// 2. User is in active micro conversation (within 2 min of last micro reply)
				// 3. User is alone in the room (micro keeps them company)
				contentLower := strings.ToLower(content)
				mentionedMicro := strings.Contains(contentLower, "@micro")
				inActiveConvo := client.InMicroConvo && time.Since(client.LastMicroReply) < 2*time.Minute

				// Check if user is alone in a topic chat room
				room.mutex.RLock()
				isAlone := strings.HasPrefix(room.ID, "chat_") && len(room.Clients) == 1
				room.mutex.RUnlock()

				if mentionedMicro || isAlone {
					client.InMicroConvo = true
				}

				if mentionedMicro || inActiveConvo || isAlone {
					go func() {
						// Pattern matching for predictable queries - skip LLM for direct lookups
						if response := handlePatternMatch(content, room); response != "" {
							app.Log("chat", "Pattern match hit, skipping LLM")
							client.LastMicroReply = time.Now()
							llmMsg := RoomMessage{
								UserID:    "micro",
								Content:   response,
								Timestamp: time.Now(),
								IsLLM:     true,
							}
							room.Broadcast <- llmMsg
							return
						}

						// If this is a Dev (HN) discussion, trigger comment refresh via event
						// But throttle to once per 5 minutes to avoid excessive API calls
						if room.Topic == "Dev" && room.URL != "" {
							room.mutex.RLock()
							lastRefresh := room.LastRefresh
							room.mutex.RUnlock()

							if time.Since(lastRefresh) > 5*time.Minute {
								room.mutex.Lock()
								room.LastRefresh = time.Now()
								room.mutex.Unlock()

								app.Log("chat", "Publishing refresh event for: %s", room.URL)
								data.Publish(data.Event{
									Type: data.EventRefreshHNComments,
									Data: map[string]interface{}{
										"url": room.URL,
									},
								})
							} else {
								app.Log("chat", "Skipping comment refresh for %s (last refresh %v ago)", room.URL, time.Since(lastRefresh).Round(time.Second))
							}
						}

						// Build context from room details
						var ragContext []string

						// Add room context first (most important)
						if room.Title != "" || room.Summary != "" {
							roomContext := ""
							if room.Title != "" {
								roomContext = "Discussion topic: " + room.Title
							}
							if room.Summary != "" {
								if roomContext != "" {
									roomContext += ". "
								}
								roomContext += room.Summary
							} else if room.Title != "" && room.Title != "News Discussion" {
								// If we have title but no content, make it clear this is what we're discussing
								roomContext += ". The article content is not yet available, answer based on related sources about this topic."
							}
							if room.URL != "" {
								roomContext += " (Source: " + room.URL + ")"
							}
							ragContext = append(ragContext, roomContext)
							app.Log("chat", "Added room context: %s", roomContext)
						} else {
							app.Log("chat", "No room context available - Title: '%s', Summary: '%s'", room.Title, room.Summary)
						}

						// Resolve pronouns by extracting entities from recent conversation
						searchContent := content
						pronouns := []string{"him", "her", "them", "they", "it", "this", "that", "he", "she"}
						contentLower := strings.ToLower(content)
						hasPronoun := false
						for _, p := range pronouns {
							if strings.Contains(contentLower, " "+p+" ") || strings.HasSuffix(contentLower, " "+p) || strings.HasPrefix(contentLower, p+" ") {
								hasPronoun = true
								break
							}
						}
						if hasPronoun {
							// Extract likely person names from recent messages (consecutive capitalized words)
							room.mutex.RLock()
							var personNames []string
							skipWords := map[string]bool{
								"the": true, "this": true, "that": true, "what": true, "when": true,
								"where": true, "which": true, "who": true, "how": true, "why": true,
								"yes": true, "sure": true, "here": true, "there": true, "i": true,
								"british": true, "brexit": true, "european": true, "union": true,
								"party": true, "uk": true, "london": true, "reform": true, "over": true,
								"recently": true, "meanwhile": true, "independence": true,
							}
							for i := len(room.Messages) - 1; i >= 0 && len(personNames) < 2; i-- {
								msg := room.Messages[i]
								words := strings.Fields(msg.Content)
								// Look for consecutive capitalized words (likely names)
								for j := 0; j < len(words)-1; j++ {
									w1 := strings.Trim(words[j], ".,!?;:'\"")
									w2 := strings.Trim(words[j+1], ".,!?;:'\"")
									if len(w1) > 2 && len(w2) > 2 &&
										w1[0] >= 'A' && w1[0] <= 'Z' &&
										w2[0] >= 'A' && w2[0] <= 'Z' &&
										!skipWords[strings.ToLower(w1)] && !skipWords[strings.ToLower(w2)] {
										name := w1 + " " + w2
										found := false
										for _, n := range personNames {
											if n == name {
												found = true
												break
											}
										}
										if !found {
											personNames = append(personNames, name)
											if len(personNames) >= 2 {
												break
											}
										}
									}
								}
							}
							room.mutex.RUnlock()
							if len(personNames) > 0 {
								// Append just the person names to search
								searchContent = content + " " + strings.Join(personNames, " ")
								app.Log("chat", "Resolved pronouns, enriched search: %s", searchContent)
							}
						}

						// Stage 1: Retrieve candidate results with snippets
						seenIDs := make(map[string]bool)

						// Exclude the current room's article from search results
						currentRoomID := room.ID[strings.Index(room.ID, "_")+1:]
						seenIDs[currentRoomID] = true

						// Search 1: Question + title context for related content
						searchQuery1 := searchContent
						if room.Title != "" && room.Title != "News Discussion" && room.Title != "Post Discussion" && room.Title != "Video Discussion" {
							searchQuery1 = room.Title + " " + searchContent
						}
						ragEntries1 := data.Search(searchQuery1, 10)
						app.Log("chat", "Search 1 (title+question) for '%s' returned %d results", searchQuery1, len(ragEntries1))

						// Search 2: Just the question to find directly relevant content
						ragEntries2 := data.Search(searchContent, 10)
						app.Log("chat", "Search 2 (question only) for '%s' returned %d results", searchContent, len(ragEntries2))

						// Combine and deduplicate results, create snippets for reranking
						type Candidate struct {
							Entry   *data.IndexEntry
							Snippet string
						}
						var candidates []Candidate
						allEntries := append(ragEntries1, ragEntries2...)

						for _, entry := range allEntries {
							if seenIDs[entry.ID] {
								continue
							}
							seenIDs[entry.ID] = true

							// Create short snippet for reranking (title + first 150 chars)
							snippet := entry.Title + ": "
							if len(entry.Content) > 150 {
								snippet += entry.Content[:150] + "..."
							} else {
								snippet += entry.Content
							}

							candidates = append(candidates, Candidate{
								Entry:   entry,
								Snippet: snippet,
							})
						}

						app.Log("chat", "Stage 1: Found %d unique candidates", len(candidates))

						// Stage 2: Rerank - ask LLM to pick most relevant sources
						var selectedEntries []*data.IndexEntry

						if len(candidates) > 5 {
							// Build reranking prompt
							snippetList := ""
							for i, c := range candidates {
								snippetList += fmt.Sprintf("%d. %s\n", i+1, c.Snippet)
							}

							// Reranking is medium priority - skip if rate limited rather than block chat
							rerankPrompt := &Prompt{
								System:   "You are a search relevance expert. Given a question and a list of document snippets, return ONLY the numbers (comma-separated) of the 3-5 most relevant documents that would help answer the question. Example: 1,3,5",
								Question: fmt.Sprintf("Question: %s\n\nDocuments:\n%s\n\nMost relevant document numbers:", content, snippetList),
								Priority: PriorityMedium, // Medium priority - skip if rate limited
							}

							rerankResp, err := askLLM(rerankPrompt)
							if err == nil && len(rerankResp) > 0 {
								app.Log("chat", "Stage 2: Reranking response: %s", rerankResp)

								// Parse comma-separated numbers
								parts := strings.Split(strings.TrimSpace(rerankResp), ",")
								for _, part := range parts {
									part = strings.TrimSpace(part)
									// Extract first number from the part (handles "1." or "1" format)
									var num int
									if _, err := fmt.Sscanf(part, "%d", &num); err == nil {
										if num > 0 && num <= len(candidates) {
											selectedEntries = append(selectedEntries, candidates[num-1].Entry)
										}
									}
									if len(selectedEntries) >= 5 {
										break
									}
								}
								app.Log("chat", "Stage 2: Selected %d documents after reranking", len(selectedEntries))
							}
						}

						// If reranking failed or we have <=5 candidates, use all
						if len(selectedEntries) == 0 {
							for _, c := range candidates {
								selectedEntries = append(selectedEntries, c.Entry)
								if len(selectedEntries) >= 5 {
									break
								}
							}
						}

						// Stage 3: Build full context from selected entries
						for _, entry := range selectedEntries {
							contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
							if len(contextStr) > 600 {
								contextStr = contextStr[:600] + "..."
							}
							if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
								contextStr += fmt.Sprintf(" (Source: %s)", url)
							}
							ragContext = append(ragContext, contextStr)
						}

						app.Log("chat", "Stage 3: Total RAG context items: %d", len(ragContext))

						// Stage 4: Inject real-time price data for financial queries
						contentLowerForPrices := strings.ToLower(content)
						priceKeywords := []string{"price", "btc", "bitcoin", "eth", "ethereum", "crypto", "stock", "market", "trading", "worth", "value", "gold", "silver", "oil"}
						needsPrices := false
						for _, kw := range priceKeywords {
							if strings.Contains(contentLowerForPrices, kw) {
								needsPrices = true
								break
							}
						}
						// Also check if in Crypto topic
						if room.Topic == "Crypto" || strings.Contains(strings.ToLower(room.Title), "crypto") || strings.Contains(strings.ToLower(room.Title), "bitcoin") {
							needsPrices = true
						}
						
						if needsPrices {
							// Query the data index for market prices (indexed by news package)
							marketEntries := data.GetByType("market", 50)
							if len(marketEntries) > 0 {
								priceInfo := "CURRENT MARKET PRICES (real-time data): "
								priceList := []string{}
								for _, entry := range marketEntries {
									if price, ok := entry.Metadata["price"].(float64); ok {
										priceList = append(priceList, fmt.Sprintf("%s: $%.2f", entry.Title, price))
									}
								}
								if len(priceList) > 0 {
									priceInfo += strings.Join(priceList, ", ")
									// Prepend prices so they're seen as authoritative
									ragContext = append([]string{priceInfo}, ragContext...)
									app.Log("chat", "Injected real-time prices from data index: %d symbols", len(priceList))
								}
							}
						}

						// Build conversation history from recent room messages
						var history History
						var recentTopics []string // Track topics from recent messages for context
						room.mutex.RLock()
						app.Log("chat", "Building history from %d room messages", len(room.Messages))
						for _, m := range room.Messages {
							if m.IsLLM {
								history = append(history, Message{Answer: m.Content})
								// Extract key phrases from AI responses for context
								if len(m.Content) > 50 {
									topicLen := min(200, len(m.Content))
									recentTopics = append(recentTopics, m.Content[:topicLen])
								}
							} else {
								history = append(history, Message{Prompt: m.UserID + ": " + m.Content})
							}
						}
						room.mutex.RUnlock()
						app.Log("chat", "Built %d history items, %d recentTopics", len(history), len(recentTopics))

						// Stage 5: Check if user wants more details - fetch full article
						app.Log("chat", "Stage 5: checking isMoreInfoRequest for: %s", content)
						if isMoreInfoRequest(content) {
							app.Log("chat", "Stage 5: User IS asking for more info")
							
							// First check if we have any URLs in current context
							urls := extractURLsFromContext(room, ragContext)
							
							// If no URLs found but we have recent topics, search based on conversation
							if len(urls) == 0 && len(recentTopics) > 0 {
								// Find the best topic to search - prefer ones with named entities
								// Start from most recent but skip generic responses
								var searchTopic string
								genericPhrases := []string{"at this time", "no additional", "for ongoing", "for continuous", "check established", "to clarify"}
								
								for i := len(recentTopics) - 1; i >= 0; i-- {
									topic := recentTopics[i]
									topicLower := strings.ToLower(topic)
									isGeneric := false
									for _, phrase := range genericPhrases {
										if strings.Contains(topicLower, phrase) {
											isGeneric = true
											break
										}
									}
									if !isGeneric {
										searchTopic = topic
										break
									}
								}
								
								// If all topics seem generic, use the first one (likely has the original info)
								if searchTopic == "" && len(recentTopics) > 0 {
									searchTopic = recentTopics[0]
								}
								
								if searchTopic != "" {
									// Try multiple search strategies
									var topicEntries []*data.IndexEntry
									
									// Strategy 1: Extract likely named entities (capitalized words) for targeted search
									words := strings.Fields(searchTopic)
									var namedEntities []string
									skipWords := map[string]bool{
										"the": true, "this": true, "that": true, "here": true, "there": true,
										"during": true, "however": true, "unfortunately": true, "certainly": true,
										"regarding": true, "absolutely": true, "sure": true, "focusing": true,
										"let": true, "recap": true, "clarify": true, "course": true, "know": true,
										"far": true, "what": true, "beyond": true, "basic": true, "key": true,
										"main": true, "central": true, "currently": true, "recent": true,
										"here's": true, "it's": true, "hasn't": true, "doesn't": true, "isn't": true,
									}
									for _, word := range words {
										// Skip common words, keep likely names/entities
										cleanWord := strings.Trim(word, ".,!?;:'\"")
										if len(cleanWord) > 2 && cleanWord[0] >= 'A' && cleanWord[0] <= 'Z' {
											lowerWord := strings.ToLower(cleanWord)
											if !skipWords[lowerWord] {
												namedEntities = append(namedEntities, cleanWord)
											}
										}
									}
									
									if len(namedEntities) >= 2 {
										// Search with named entities (e.g., "Nigel Farage Nadhim Zahawi")
										entitySearch := strings.Join(namedEntities[:min(4, len(namedEntities))], " ")
										app.Log("chat", "Stage 5: Searching with entities: %s", entitySearch)
										topicEntries = data.Search(entitySearch, 5)
									}
									
									// Strategy 2: If entity search fails, try topic keyword
									if len(topicEntries) == 0 && room.Topic != "" {
										app.Log("chat", "Stage 5: Trying topic search: %s", room.Topic)
										topicEntries = data.Search(room.Topic, 5)
									}
									
									app.Log("chat", "Stage 5: Found %d entries from search", len(topicEntries))
									
									for _, entry := range topicEntries {
										contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
										if len(contextStr) > 600 {
											contextStr = contextStr[:600] + "..."
										}
										if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
											contextStr += fmt.Sprintf(" (Source: %s)", url)
										}
										ragContext = append(ragContext, contextStr)
									}
									
									// Re-extract URLs after adding topic search results
									urls = extractURLsFromContext(room, ragContext)
								}
							}
							
							app.Log("chat", "Stage 5: Found %d URLs in context", len(urls))
							if len(urls) > 0 {
								app.Log("chat", "User asking for more info, getting content from: %s", urls[0])
							
							// Send progress message
							progressMsg := RoomMessage{
								UserID:    "micro",
								Content:   "Let me get the full article for you...",
								Timestamp: time.Now(),
								IsLLM:     true,
							}
							room.Broadcast <- progressMsg
							
							// Get the article content from index (already parsed/summarized)
							articleContent, err := getContentFromURL(urls[0])
							if err == nil && len(articleContent) > 100 {
								// Prepend the full article content
								fullArticle := fmt.Sprintf("FULL ARTICLE CONTENT from %s:\n\n%s", urls[0], articleContent)
								ragContext = append([]string{fullArticle}, ragContext...)
								// Modify the question to tell LLM we already sent a progress message
								content = content + "\n\n[Note: A progress message has already been shown to the user. Do not add any preamble like 'Let me provide...' - just give the information directly.]"
								app.Log("chat", "Got full article content: %d chars", len(articleContent))
							} else {
								app.Log("chat", "Failed to get article: %v", err)
							}
						}
					}

						prompt := &Prompt{
							Rag:      ragContext,
							Context:  history,
							Question: content,
						}

						resp, err := askLLM(prompt)
						if err == nil && len(resp) > 0 {
							// Update client's micro conversation state
							client.LastMicroReply = time.Now()

							llmMsg := RoomMessage{
								UserID:    "micro",
								Content:   resp,
								Timestamp: time.Now(),
								IsLLM:     true,
							}
							room.Broadcast <- llmMsg
						}
					}()
				}
			}
		}
	}()
}

func Load() {
	// load the feeds file
	b, _ := f.ReadFile("prompts.json")
	if err := json.Unmarshal(b, &prompts); err != nil {
		app.Log("chat", "Error parsing topics.json: %v", err)
	}

	for topic, _ := range prompts {
		topics = append(topics, topic)
	}

	sort.Strings(topics)

	// Generate head with topics (rooms will be added dynamically)
	head = app.Head("chat", topics)

	// Register LLM analyzer for content moderation
	admin.SetAnalyzer(&llmAnalyzer{})

	// Load existing summaries from disk
	if b, err := data.LoadFile("chat_summaries.json"); err == nil {
		if err := json.Unmarshal(b, &summaries); err != nil {
			app.Log("chat", "Error loading summaries: %v", err)
		} else {
			app.Log("chat", "Loaded %d summaries from disk", len(summaries))
		}
	}

	// Subscribe to summary generation requests
	summaryRequestSub := data.Subscribe(data.EventGenerateSummary)
	go func() {
		for event := range summaryRequestSub.Chan {
			uri, okUri := event.Data["uri"].(string)
			content, okContent := event.Data["content"].(string)
			eventType, okType := event.Data["type"].(string)

			if okUri && okContent && okType {
				app.Log("chat", "Received summary generation request for %s (%s)", uri, eventType)

				// Generate summary using LLM (low priority - background task)
				prompt := &Prompt{
					System:   "You are a helpful assistant that creates concise summaries. Provide only the summary content itself without any introductory phrases like 'Here is a summary' or 'This article is about'. Just write 2-3 clear, factual sentences that capture the key points.",
					Question: fmt.Sprintf("Summarize this article:\n\n%s", content),
					Priority: PriorityLow, // Low priority for background article summaries
				}

				summary, err := askLLM(prompt)
				if err != nil {
					app.Log("chat", "Error generating summary for %s: %v", uri, err)
					continue
				}

				// Publish the generated summary
				data.Publish(data.Event{
					Type: data.EventSummaryGenerated,
					Data: map[string]interface{}{
						"uri":     uri,
						"summary": summary,
						"type":    eventType,
					},
				})

				app.Log("chat", "Published generated summary for %s", uri)
			}
		}
	}()

	// Subscribe to tag generation requests
	tagRequestSub := data.Subscribe(data.EventGenerateTag)
	go func() {
		for event := range tagRequestSub.Chan {
			postID, okID := event.Data["post_id"].(string)
			title, okTitle := event.Data["title"].(string)
			content, okContent := event.Data["content"].(string)
			eventType, okType := event.Data["type"].(string)

			if okID && okTitle && okContent && okType && eventType == "post" {
				app.Log("chat", "Received tag generation request for post %s", postID)

				// Get valid topics from prompts map
				var topics []string
				for topic := range prompts {
					topics = append(topics, topic)
				}
				if len(topics) == 0 {
					app.Log("chat", "No topics available for tag generation")
					continue
				}

				// Generate tag using LLM with predefined categories (low priority)
				prompt := &Prompt{
					System:   fmt.Sprintf("You are a content categorization assistant. Your task is to categorize posts into ONE of these categories ONLY: %s. If the post does not clearly fit into any of these categories, respond with 'None'. Respond with ONLY the category name or 'None', nothing else.", strings.Join(topics, ", ")),
					Question: fmt.Sprintf("Categorize this post:\n\nTitle: %s\n\nContent: %s\n\nWhich single category best fits this post?", title, content),
					Priority: PriorityLow, // Low priority for background tag generation
				}

				tag, err := askLLM(prompt)
				if err != nil {
					app.Log("chat", "Error generating tag for post %s: %v", postID, err)
					continue
				}

				// Trim and validate the tag
				tag = strings.TrimSpace(tag)

				// Skip if LLM couldn't categorize
				if tag == "None" || tag == "none" || tag == "" {
					app.Log("chat", "Post %s could not be categorized, skipping tag", postID)
					continue
				}

				// Validate against prompts map keys
				validTag := false
				for topic := range prompts {
					if strings.EqualFold(tag, topic) {
						tag = topic // Use the proper casing from map key
						validTag = true
						break
					}
				}

				if !validTag {
					app.Log("chat", "Invalid tag returned for post %s: '%s', skipping tag", postID, tag)
					continue
				}

				// Publish the generated tag
				data.Publish(data.Event{
					Type: data.EventTagGenerated,
					Data: map[string]interface{}{
						"post_id": postID,
						"tag":     tag,
						"type":    eventType,
					},
				})

				app.Log("chat", "Published generated tag for post %s: %s", postID, tag)
			}
		}
	}()

	go generateSummaries()
	go cleanupIdleRooms()
}

func generateSummaries() {
	app.Log("chat", "Generating summaries at %s", time.Now().String())

	newSummaries := map[string]string{}

	for topic, prompt := range prompts {
		// Search for relevant content for each topic
		ragEntries := data.Search(topic, 3)
		var ragContext []string
		for _, entry := range ragEntries {
			contentStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
			if len(contentStr) > 500 {
				contentStr = contentStr[:500]
			}
			ragContext = append(ragContext, contentStr)
		}

		resp, err := askLLM(&Prompt{
			Rag:      ragContext,
			Question: prompt,
			Priority: PriorityMedium, // Medium priority for topic summaries
		})

		if err != nil {
			app.Log("chat", "Failed to generate summary for topic %s: %v", topic, err)
			continue
		}
		newSummaries[topic] = resp

		// Stagger requests to avoid rate limit spikes
		time.Sleep(10 * time.Second)
	}

	mutex.Lock()
	summaries = newSummaries
	mutex.Unlock()

	// Save summaries to disk
	if err := data.SaveJSON("chat_summaries.json", summaries); err != nil {
		app.Log("chat", "Error saving summaries: %v", err)
	} else {
		app.Log("chat", "Saved %d summaries to disk", len(summaries))
	}

	// Generate topic summaries every 4 hours (not hourly) to reduce LLM calls
	time.Sleep(4 * time.Hour)

	go generateSummaries()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Check if this is a room-based chat (e.g., /chat?id=post_123)
	roomID := r.URL.Query().Get("id")

	// Check if this is a WebSocket upgrade request
	if r.Header.Get("Upgrade") == "websocket" && roomID != "" {
		room := getOrCreateRoom(roomID)
		if room == nil {
			http.Error(w, "Invalid room ID", http.StatusBadRequest)
			return
		}
		handleWebSocket(w, r, room)
		return
	}

	if r.Method == "GET" {
		// Check if JSON response is requested
		isJSON := strings.Contains(r.Header.Get("Accept"), "application/json")

		// Get room data with timeout to prevent hanging
		roomData := map[string]interface{}{}
		if roomID != "" {
			app.Log("chat", "GET request for room: %s", roomID)
			type roomResult struct {
				room *Room
			}
			resultChan := make(chan roomResult, 1)

			go func() {
				app.Log("chat", "Starting getOrCreateRoom for: %s", roomID)
				room := getOrCreateRoom(roomID)
				app.Log("chat", "getOrCreateRoom completed for: %s, room=%v", roomID, room != nil)
				resultChan <- roomResult{room: room}
			}()

			select {
			case result := <-resultChan:
				if result.room != nil {
					roomData["id"] = roomID
					roomData["title"] = result.room.Title
					roomData["summary"] = result.room.Summary
					roomData["url"] = result.room.URL
					roomData["isRoom"] = true
					app.Log("chat", "Room data loaded for: %s", roomID)
				} else {
					app.Log("chat", "Room is nil for: %s", roomID)
				}
			case <-time.After(5 * time.Second):
				app.Log("chat", "TIMEOUT creating room %s - likely blocked on data.GetByID()", roomID)
				http.Error(w, "Room creation timeout - server may be busy indexing content. Please try again.", http.StatusRequestTimeout)
				return
			}
		}

		// Now acquire mutex only for reading chat config
		mutex.RLock()
		topicsData := topics
		summariesData := summaries
		mutex.RUnlock()

		// Return JSON if requested
		if isJSON {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]interface{}{
				"topics":    topicsData,
				"summaries": summariesData,
			}
			if len(roomData) > 0 {
				response["room"] = roomData
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Return HTML
		topicTabs := app.Head("chat", topicsData)
		summariesJSON, _ := json.Marshal(summariesData)
		roomJSON, _ := json.Marshal(roomData)

		tmpl := app.RenderHTMLForRequest("Chat", "Chat with AI", fmt.Sprintf(Template, topicTabs), r)
		tmpl = strings.Replace(tmpl, "</body>", fmt.Sprintf(`<script>var summaries = %s; var roomData = %s;</script></body>`, summariesJSON, roomJSON), 1)

		w.Write([]byte(tmpl))
		return
	}

	if r.Method == "POST" {
		// Require authentication to send messages
		sess, err := auth.GetSession(r)
		if err != nil {
			http.Error(w, "Authentication required to chat", http.StatusUnauthorized)
			return
		}

		form := make(map[string]interface{})

		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := ioutil.ReadAll(r.Body)
			if len(b) == 0 {
				return
			}

			json.Unmarshal(b, &form)

			if form["prompt"] == nil {
				return
			}
		} else {
			// save the response
			r.ParseForm()

			// get the message
			ctx := r.Form.Get("context")
			msg := r.Form.Get("prompt")

			if len(msg) == 0 {
				return
			}

			// Limit prompt length to prevent abuse
			if len(msg) > 500 {
				http.Error(w, "Prompt must not exceed 500 characters", http.StatusBadRequest)
				return
			}

			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			form["context"] = ictx
			form["prompt"] = msg
		}

		var context History

		if vals := form["context"]; vals != nil {
			cvals := vals.([]interface{})
			// Keep only the last 3 messages to reduce context size and fit 4096 token limit
			startIdx := 0
			if len(cvals) > 3 {
				startIdx = len(cvals) - 3
			}
			for _, val := range cvals[startIdx:] {
				msg := val.(map[string]interface{})
				prompt := fmt.Sprintf("%v", msg["prompt"])
				answer := fmt.Sprintf("%v", msg["answer"])
				context = append(context, Message{Prompt: prompt, Answer: answer})
			}
		}

		q := fmt.Sprintf("%v", form["prompt"])

		// Check if this is a direct message (starts with @username)
		if strings.HasPrefix(strings.TrimSpace(q), "@") {
			// Direct message - don't invoke LLM, just echo back
			form["answer"] = "<p><em>Message sent. Direct messages are visible to everyone in this topic.</em></p>"

			// if JSON request then respond with json
			if ct := r.Header.Get("Content-Type"); ct == "application/json" {
				b, _ := json.Marshal(form)
				w.Header().Set("Content-Type", "application/json")
				w.Write(b)
				return
			}

			// Format a HTML response
			messages := fmt.Sprintf(`<div class="message"><span class="you">you</span><p>%v</p></div>`, form["prompt"])
			messages += fmt.Sprintf(`<div class="message"><span class="system">system</span><p>%v</p></div>`, form["answer"])

			mutex.RLock()
			topicTabs := app.Head("chat", topics)
			mutex.RUnlock()

			output := fmt.Sprintf(Template, topicTabs)
			renderHTML := app.RenderHTMLForRequest("Chat", "Chat with AI", output, r)
			renderHTML = strings.Replace(renderHTML, `<div id="messages"></div>`, fmt.Sprintf(`<div id="messages">%s</div>`, messages), 1)

			w.Write([]byte(renderHTML))
			return
		}

		// Check quota before LLM query
		canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpChatQuery)
		if !canProceed {
			// Return quota exceeded response
			if ct := r.Header.Get("Content-Type"); ct == "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(402) // Payment Required
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "quota_exceeded",
					"message": "Daily chat limit reached. Please top up credits or upgrade to member.",
					"cost":    cost,
				})
				return
			}
			// HTML response
			content := wallet.QuotaExceededPage(wallet.OpChatQuery, cost)
			html := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", content, r)
			w.Write([]byte(html))
			return
		}

		// Get topic for enhanced RAG
		topic := ""
		if t := form["topic"]; t != nil {
			topic = fmt.Sprintf("%v", t)
		}

		// Search the index for relevant context (RAG)
		// Search directly with the user's query to find best matches
		ragEntries := data.Search(q, 5)
		var ragContext []string
		for _, entry := range ragEntries {
			// Debug: Show raw entry
			app.Log("chat", "[RAG DEBUG] Entry: Type=%s, Title=%s, Content=%s", entry.Type, entry.Title, entry.Content)

			// Format each entry as context (600 chars to fit within 4096 token limit)
			contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
			if len(contextStr) > 600 {
				contextStr = contextStr[:600] + "..."
			}
			if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
				contextStr += fmt.Sprintf(" (Source: %s)", url)
			}
			ragContext = append(ragContext, contextStr)
		}

		// Debug: Log what we found
		if len(ragEntries) > 0 {
			app.Log("chat", "[RAG] Query: %s", q)
			app.Log("chat", "[RAG] Found %d entries:", len(ragEntries))
			for i, entry := range ragEntries {
				app.Log("chat", "  %d. [%s] %s", i+1, entry.Type, entry.Title)
			}
			app.Log("chat", "[RAG] Context being sent to LLM:")
			for i, ctx := range ragContext {
				app.Log("chat", "  %d. %s", i+1, ctx)
			}
		} else {
			app.Log("chat", "[RAG] Query: %s - NO RESULTS", q)
		}

		prompt := &Prompt{
			Topic:    topic,
			Rag:      ragContext,
			Context:  context,
			Question: q,
		}

		// query the llm
		resp, err := askLLM(prompt)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		if len(resp) == 0 {
			return
		}

		// Consume quota after successful LLM response
		wallet.ConsumeQuota(sess.Account, wallet.OpChatQuery)

		// save the response
		html := app.Render([]byte(resp))
		form["answer"] = string(html)

		// if JSON request then respond with json
		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := json.Marshal(form)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}

		// Format a HTML response
		messages := fmt.Sprintf(`<div class="message"><span class="you">you</span><p>%v</p></div>`, form["prompt"])
		messages += fmt.Sprintf(`<div class="message"><span class="micro">micro</span><p>%v</p></div>`, form["answer"])

		mutex.RLock()
		topicTabs := app.Head("chat", topics)
		mutex.RUnlock()

		output := fmt.Sprintf(Template, topicTabs)
		renderHTML := app.RenderHTMLForRequest("Chat", "Chat with AI", output, r)
		renderHTML = strings.Replace(renderHTML, `<div id="messages"></div>`, fmt.Sprintf(`<div id="messages">%s</div>`, messages), 1)

		w.Write([]byte(renderHTML))
	}
}

// llmAnalyzer implements the admin.LLMAnalyzer interface
type llmAnalyzer struct{}

func (a *llmAnalyzer) Analyze(promptText, question string) (string, error) {
	// Create a simple prompt for analysis
	prompt := &Prompt{
		System:   promptText,
		Question: question,
		Context:  nil,
		Rag:      nil,
	}
	return askLLM(prompt)
}

// cleanupIdleRooms periodically removes idle chat rooms to prevent memory leaks
func cleanupIdleRooms() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		idleThreshold := 30 * time.Minute

		roomsMutex.Lock()
		var toDelete []string

		for roomID, room := range rooms {
			room.mutex.RLock()
			clientCount := len(room.Clients)
			lastActivity := room.LastActivity
			room.mutex.RUnlock()

			// Remove room if it has no clients and has been idle for threshold
			if clientCount == 0 && now.Sub(lastActivity) > idleThreshold {
				toDelete = append(toDelete, roomID)
			}
		}

		// Delete idle rooms
		for _, roomID := range toDelete {
			if room, exists := rooms[roomID]; exists {
				// Signal room to shutdown
				select {
				case room.Shutdown <- true:
				// Shutdown signal sent
				default:
					// Channel might be full or already shutting down, skip
				}
				delete(rooms, roomID)
				app.Log("chat", "Cleaned up idle room: %s (total rooms: %d)", roomID, len(rooms))
			}
		}

		roomsMutex.Unlock()

		if len(toDelete) > 0 {
			app.Log("chat", "Cleaned up %d idle rooms (remaining: %d)", len(toDelete), len(rooms))
		}
	}
}
