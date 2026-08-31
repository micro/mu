package chat

import (
	"embed"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
)

//go:embed *.json
var f embed.FS

// Template is the room itself: the conversation, and a box to say something
// into it. The form has no action — mu.js binds it to the websocket once the
// room connects, so a message goes to the people present rather than to a
// model.
//
// There used to be a second thing on this page: a topic picker and a question
// box that asked an LLM, answered from the index, and had no other participant
// in it. That was the assistant, from before there was an /agent to be the
// assistant. Two boxes, one of which quietly meant something else, is the
// confusion this page is now free of — chat is where people talk to each
// other, /agent is where you talk to something that acts.
var Template = `
%s
<div id="messages"></div>
<form id="chat-form" onsubmit="return false;">
<input id="topic" name="topic" type="hidden">
<input id="prompt" name="prompt" type="text" placeholder="Say something" autocomplete=off>
<button>Send</button>
</form>`

var mutex sync.RWMutex

var prompts = map[string]string{}

// askLLM is internal helper for ai.Ask
func askLLM(prompt *ai.Prompt) (string, error) {
	if prompt.Caller == "" {
		prompt.Caller = "chat"
	}
	resp, err := ai.Ask(prompt)
	if err != nil {
		return resp, err
	}
	return app.StripLatexDollars(resp), nil
}

var summaries = map[string]string{}
var summaryMeta = SummaryMetadata{}

// SummaryMetadata describes when and how the generated topic summaries were refreshed.
type SummaryMetadata struct {
	GeneratedAt time.Time `json:"generated_at,omitempty"`
	Source      string    `json:"source"`
	Status      string    `json:"status"`
}

func currentSummaryMeta() SummaryMetadata {
	if summaryMeta.Status == "" {
		return SummaryMetadata{Source: "Mu indexed public content", Status: "unavailable"}
	}
	return summaryMeta
}

var topics = []string{}

// The lobby: the room somebody is in when they have not chosen a room.
//
// A chat with no default is a list, and a list is a page you read rather than
// a place you are. Every messaging product this could be compared to drops you
// somewhere — a general channel, the last thread, an empty compose — because
// arriving at a directory of conversations is arriving at none of them.
//
// It is an ordinary topic room. Nothing about it is special except that it has
// no subject, which is what makes it the one room where anything can be said.
const (
	lobbyTopic = "lobby"
	lobbyID    = "chat_" + lobbyTopic
)

// channels is the row of rooms across the top of one, so there is a way out of
// the room you are in that is not the back button.
//
// The lobby first, then the topics this instance follows, then whatever else
// has somebody in it right now. That last group is the reason this is not a
// static list: an article's discussion room is where the conversation actually
// is on the day somebody starts one, and it existed nowhere a person could
// find it except the article.
//
// The same .head row news and video use for their topics, because switching
// channel and switching topic are the same gesture and there is no reason for
// this page to invent a second look for it.
func channels(current string) string {
	mutex.RLock()
	names := append([]string(nil), topics...)
	mutex.RUnlock()
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(`<div id="topics">`)

	tab := func(id, label string) {
		if id == current {
			b.WriteString(`<span class="head head-on">` + htmlpkg.EscapeString(label) + `</span>`)
			return
		}
		b.WriteString(`<a class="head" href="/chat?id=` + url.QueryEscape(id) + `">` +
			htmlpkg.EscapeString(label) + `</a>`)
	}

	tab(lobbyID, "Lobby")
	for _, topic := range names {
		if topic == lobbyTopic {
			continue
		}
		tab("chat_"+topic, topic)
	}

	// And anything live that is not a topic — an article or a video somebody is
	// discussing. Capped, newest first: this is a way to what is happening, not
	// a second catalogue.
	var live []*Room
	roomsMutex.RLock()
	for _, room := range rooms {
		room.mutex.RLock()
		if !strings.HasPrefix(room.ID, "chat_") && (len(room.Clients) > 0 || len(room.Messages) > 0) {
			live = append(live, room)
		}
		room.mutex.RUnlock()
	}
	roomsMutex.RUnlock()
	sort.Slice(live, func(i, j int) bool { return live[i].LastActivity.After(live[j].LastActivity) })
	if len(live) > liveTabs {
		live = live[:liveTabs]
	}
	for _, room := range live {
		room.mutex.RLock()
		id, title := room.ID, roomName(room.Title)
		room.mutex.RUnlock()
		tab(id, trimTo(title, 28))
	}

	b.WriteString(`</div>`)
	return b.String()
}

// liveTabs caps how many item rooms reach the channel row. Four is a row on a
// phone; past that it wraps to a second line and stops reading as navigation.
const liveTabs = 4

// trimTo shortens a channel label. An article's title is a headline and a
// channel is a word or two, so the whole headline in a tab row is one tab.
func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n]), " ,;:—-") + "…"
}

var head string

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// agentName is who the agent is in a room: the name on its messages, and the
// name in the list of who is here. It was spelled out at five call sites, and
// the one that mattered was the list — a room where the agent answers every
// message showed nobody in it but you.
const agentName = "micro"

// AgentName is who the agent is in a room, for whoever answers as it.
//
// Exported because agent/chat has to say a message is from the agent and the
// room has to render it as such — and the name has to be one string, not two
// that agree until somebody changes one.
const AgentName = agentName

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
	Conn           *websocket.Conn
	UserID         string
	Room           *Room
	InMicroConvo   bool      // true if user started a conversation with @micro
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
// Messages older than 24 hours are pruned
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

	// Prune messages older than 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)
	var recent []RoomMessage
	for _, msg := range messages {
		if msg.Timestamp.After(cutoff) {
			recent = append(recent, msg)
		}
	}

	if len(recent) < len(messages) {
		app.Log("chat", "Pruned %d old messages for room %s", len(messages)-len(recent), roomID)
	}

	app.Log("chat", "Loaded %d messages for room %s", len(recent), roomID)
	return recent
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
				entry := data.ByID("market_" + p.symbol)
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
			entry := data.ByID("market_" + symbol)
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
			entry := data.ByID("market_" + symbol)
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

// getOrCreateRoom gets an existing room or creates a new one
// Say puts a message into a room that already exists.
//
// The door an agent speaks through, and the reason it can exist at all: an
// agent may import a service, so agent/chat calls this, and this package never
// learns that an agent exists. Same shape as blog.CreatePost for agent/blog.
//
// It will not create a room. A room is made when somebody opens it, and an
// answer arriving into a conversation nobody is in should be dropped rather
// than conjure the conversation — the caller has gone, and the message would
// sit in a room with no readers holding memory until the cleanup found it.
//
// Watching reports whether an account has a live connection to a room right
// now.
//
// Not auth.OnlineUsers, which is a three minute window over the whole instance
// and answers "have they used this server lately" — true of somebody who is
// reading their mail in another tab. This is the narrower fact the inbox needs:
// they are in this room, with this conversation on screen, so a message landing
// in it has been seen.
//
// A live socket, so it goes false the moment the tab closes. That is the right
// side to err on: marking a message read that somebody did not see loses it,
// and leaving one unread that they did costs them a click.
func Watching(roomID, account string) bool {
	if strings.TrimSpace(roomID) == "" || strings.TrimSpace(account) == "" {
		return false
	}
	roomsMutex.RLock()
	room, ok := rooms[roomID]
	roomsMutex.RUnlock()
	if !ok {
		return false
	}
	room.mutex.RLock()
	defer room.mutex.RUnlock()
	for _, c := range room.Clients {
		if c != nil && c.UserID == account {
			return true
		}
	}
	return false
}

// Reports whether it landed, so a caller can say so rather than assume.
func Say(roomID, from, text string) bool {
	if strings.TrimSpace(roomID) == "" || strings.TrimSpace(text) == "" {
		return false
	}
	roomsMutex.RLock()
	room, ok := rooms[roomID]
	roomsMutex.RUnlock()
	if !ok {
		return false
	}
	// Non-blocking. Broadcast is consumed by the room's own goroutine, and a
	// room whose reader has stopped would otherwise hold this one forever —
	// which for a subscriber means the whole event loop, not one message.
	select {
	case room.Broadcast <- RoomMessage{
		UserID:    from,
		Content:   text,
		Timestamp: time.Now(),
		IsLLM:     from == agentName,
	}:
		return true
	default:
		app.Log("chat", "room %s is not reading, dropped a message from %s", roomID, from)
		return false
	}
}

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
			entryChan <- data.ByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up post %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting post %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "Post"
			room.Summary = "Loading post content..."
			room.URL = "/blog/post?id=" + itemID
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
			room.URL = "/blog/post?id=" + itemID
			app.Log("chat", "Room context - Title: %s, Summary length: %d, URL: %s", room.Title, len(room.Summary), room.URL)
		} else if room.Title == "" {
			app.Log("chat", "Post %s not found in index", itemID)
			room.Title = "Post"
			room.URL = "/blog/post?id=" + itemID
		}
	case "news":
		// For news, lookup by exact ID
		app.Log("chat", "Attempting to get news item %s from index", itemID)

		// Try with a timeout to avoid blocking during heavy indexing
		entryChan := make(chan *data.IndexEntry, 1)
		go func() {
			entryChan <- data.ByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up news item %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting news %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "News"
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
				room.Title = "News"
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
			entryChan <- data.ByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up video item %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting video %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "Video"
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
			room.Title = "Video"
		}
	case "chat":
		// For chat topics, use the topic name from summaries
		room.Title = itemID
		if itemID == lobbyTopic {
			// The one room that is not about anything. No summary, so no
			// About block draws over the messages — there is nothing to say
			// about a room whose subject is whoever is in it.
			room.Title = "Lobby"
		} else {
			mutex.RLock()
			if summary, exists := summaries[itemID]; exists {
				room.Summary = summary
			} else {
				room.Summary = "General discussion about " + itemID
			}
			mutex.RUnlock()
		}
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
	case "reminder":
		// For reminder, lookup by exact ID
		app.Log("chat", "Attempting to get reminder item %s from index", itemID)

		// Try with a timeout to avoid blocking during heavy indexing
		entryChan := make(chan *data.IndexEntry, 1)
		go func() {
			entryChan <- data.ByID(itemID)
		}()

		var entry *data.IndexEntry
		select {
		case entry = <-entryChan:
			app.Log("chat", "Looking up reminder item %s, found: %v", itemID, entry != nil)
		case <-time.After(2 * time.Second):
			app.Log("chat", "Timeout getting reminder %s from index, will create room with minimal context", itemID)
			// Create room with minimal context
			room.Title = "Daily Reminder"
			room.Summary = "Loading reminder content..."
		}

		if entry != nil {
			room.Title = "Daily Reminder"
			room.Summary = entry.Content
			if len(room.Summary) > 2000 {
				room.Summary = room.Summary[:2000] + "..."
			}
			room.URL = "https://reminder.dev"
			app.Log("chat", "Room context - Title: %s, Summary length: %d, URL: %s", room.Title, len(room.Summary), room.URL)
		} else if room.Title == "" {
			app.Log("chat", "Reminder item %s not found in index", itemID)
			room.Title = "Daily Reminder"
			room.URL = "https://reminder.dev"
		}

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
		sub := event.Subscribe(event.IndexComplete)
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
						entry := data.ByID(itemID)
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
						entry := data.ByID(parts[1])
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

// roster is who is in the room: everybody connected, and the agent.
//
// The agent is in every room because it answers in every room. This listed it
// for chat_ rooms only, and the rule about when it answers lives in another
// function and says something else — an item room (news, video, post,
// reminder) always gets a reply. So "Discuss with AI" on a news article showed
// one person present, nobody else, and then the AI answered. Two conditions
// about the same fact, written apart, disagreeing.
//
// Split out from broadcastUserList so that fact can be tested without a
// websocket on the other end of it.
func (room *Room) roster() []string {
	room.mutex.RLock()
	names := make([]string, 0, len(room.Clients)+1)
	for _, client := range room.Clients {
		names = append(names, client.UserID)
	}
	room.mutex.RUnlock()
	return append(names, agentName)
}

// broadcastUserList sends the current list of usernames to all clients
func (room *Room) broadcastUserList() {
	userListMsg := map[string]interface{}{
		"type":  "user_list",
		"users": room.roster(),
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

	var prompt *ai.Prompt
	if summary != "" {
		prompt = &ai.Prompt{
			System: "You are a friendly chat participant in a " + topicName + " room. " +
				"The summary below is already printed at the top of the page, so do not " +
				"repeat, restate or paraphrase it — the reader has just read it. Ask one " +
				"thought-provoking question that follows from it. One sentence. " +
				"Conversational, not formal.",
			Question: "Current " + topicName + " summary: " + summary + "\n\nStart a conversation:",
			Priority: ai.PriorityLow,
		}
	} else {
		prompt = &ai.Prompt{
			System:   "You are a friendly chat participant in a " + topicName + " discussion room. Start a brief, engaging conversation about " + topicName + ". Ask a thought-provoking question or share an interesting observation. Keep it to 1-2 sentences. Be conversational, not formal.",
			Question: "Start a conversation about " + topicName + ":",
			Priority: ai.PriorityLow,
		}
	}

	resp, err := askLLM(prompt)
	if err != nil || resp == "" {
		app.Log("chat", "AI greeting failed for room %s: %v", room.ID, err)
		return
	}

	msg := RoomMessage{
		UserID:    agentName,
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
	_, acc, err := auth.RequireSession(r)
	if err != nil {
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
				// For item-specific rooms (news_, video_, post_), ALWAYS respond - these are AI discussions
				// For topic chat rooms (chat_), respond when mentioned or alone (public room behavior)
				contentLower := strings.ToLower(content)
				mentionedMicro := strings.Contains(contentLower, "@micro")

				// Item-specific rooms always get AI responses (this is "discuss with AI")
				isItemRoom := strings.HasPrefix(room.ID, "news_") ||
					strings.HasPrefix(room.ID, "video_") ||
					strings.HasPrefix(room.ID, "post_") ||
					strings.HasPrefix(room.ID, "reminder_")

				// Check if user is alone in a topic chat room
				room.mutex.RLock()
				isAlone := strings.HasPrefix(room.ID, "chat_") && len(room.Clients) == 1
				room.mutex.RUnlock()

				// inActiveConvo only applies when the user is alone
				// When multiple users are present, micro only responds to explicit @micro mentions
				inActiveConvo := isAlone && client.InMicroConvo && time.Since(client.LastMicroReply) < 2*time.Minute

				if mentionedMicro || isAlone || isItemRoom {
					client.InMicroConvo = true
				}

				if mentionedMicro || inActiveConvo || isAlone || isItemRoom {
					// Deterministic lookups first, with no model involved.
					// Answering "what is the weather" from a table is a service
					// answering a question about state, which is this package's
					// job and cheaper than a model call.
					if response := handlePatternMatch(content, room); response != "" {
						app.Log("chat", "pattern match, no agent needed")
						client.LastMicroReply = time.Now()
						Say(room.ID, agentName, response)
						continue
					}

					// Otherwise say what happened and let whoever answers
					// answer. This used to be a hundred and ninety lines here:
					// its own RAG over the index, its own decision about
					// whether to search the web, its own history assembled
					// from the room, and a model call, all inside a websocket
					// goroutine.
					//
					// That was a hand-rolled agent inside a service. The rule
					// it broke is the one with a reason rather than a
					// convention behind it — a service answers a question
					// about state, an agent decides which question to ask —
					// and the cost of breaking it was that the agent in a room
					// could reach two sources while the agent everywhere else
					// in this product reaches a hundred and eighteen tools.
					//
					// The gate above stays here, because who is in the room
					// and whether the agent was named are facts about the
					// room. What is published is only what passed it. See
					// event.ChatForAgent, and agent/chat, which subscribes.
					room.mutex.RLock()
					title, summary, url := room.Title, room.Summary, room.URL
					room.mutex.RUnlock()
					client.LastMicroReply = time.Now()
					event.RequestChatReply(room.ID, title, summary, url, client.UserID, content)
				}
			}
		}
	}()
}

func Load() {
	// This service's own record, the way service/mail loads its mailbox.
	LoadStore()

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

	// No moderation analyzer registered here any more.
	//
	// This service used to fill in internal/flag's `analyzer` variable, which
	// meant content moderation for the whole instance — social, blog, apps —
	// depended on the chat service loading, and was silently off if it did
	// not. Nothing about chat made it the right place; it was where somebody
	// had an LLM call handy. See agent/moderate.

	// Load existing summaries from disk
	if b, err := data.LoadFile("chat_summaries.json"); err == nil {
		if err := json.Unmarshal(b, &summaries); err != nil {
			app.Log("chat", "Error loading summaries: %v", err)
		} else {
			app.Log("chat", "Loaded %d summaries from disk", len(summaries))
			if len(summaries) > 0 {
				summaryMeta = SummaryMetadata{Source: "Mu indexed public content", Status: "cached"}
			}
		}
	}
	if b, err := data.LoadFile("chat_summaries_meta.json"); err == nil {
		if err := json.Unmarshal(b, &summaryMeta); err != nil {
			app.Log("chat", "Error loading summary metadata: %v", err)
		}
	}

	// Subscribe to summary generation requests
	summaryRequestSub := event.Subscribe(event.GenerateSummary)
	go func() {
		for evt := range summaryRequestSub.Chan {
			uri, okUri := evt.Data["uri"].(string)
			content, okContent := evt.Data["content"].(string)
			eventType, okType := evt.Data["type"].(string)

			if okUri && okContent && okType {
				// Nothing to ask, so nothing to report. A fresh instance has no
				// provider — the install guide says so and means it — and every
				// article that arrived then produced its own red line saying the
				// same sentence, so a first run looked like a broken one. The
				// topic loop above already skips with a single line; this is the
				// same decision one event at a time.
				if !ai.Configured() {
					continue
				}
				app.Log("chat", "Received summary generation request for %s (%s)", uri, eventType)

				// Generate summary using LLM (low priority - background task, Haiku for cost)
				prompt := &ai.Prompt{
					System:   "You are a helpful assistant that creates concise summaries. Provide only the summary content itself without any introductory phrases like 'Here is a summary' or 'This article is about'. Just write 2-3 clear sentences that capture the key points. Report what the article says without editorialising, fact-checking, or adding commentary about whether claims are true or false. Never add disclaimers, caveats, or corrections.",
					Question: fmt.Sprintf("Summarize this article:\n\n%s", content),
					Priority: ai.PriorityLow,
					Model:    ai.BackgroundModel(),
					Caller:   "article-summary",
				}

				summary, err := askLLM(prompt)
				if err != nil {
					app.Log("chat", "Error generating summary for %s: %v", uri, err)
					continue
				}

				// Publish the generated summary
				event.Publish(event.Event{
					Type: event.SummaryGenerated,
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
	tagRequestSub := event.Subscribe(event.GenerateTag)
	go func() {
		for evt := range tagRequestSub.Chan {
			title, _ := evt.Data["title"].(string)
			content, okContent := evt.Data["content"].(string)
			eventType, okType := evt.Data["type"].(string)

			if !okContent || !okType {
				continue
			}

			// Handle blog post tagging (predefined categories)
			if eventType == "post" {
				postID, ok := evt.Data["post_id"].(string)
				if !ok {
					continue
				}
				app.Log("chat", "Received tag generation request for post %s", postID)

				var topics []string
				for topic := range prompts {
					topics = append(topics, topic)
				}
				if len(topics) == 0 {
					app.Log("chat", "No topics available for tag generation")
					continue
				}

				prompt := &ai.Prompt{
					System:   fmt.Sprintf("You are a content categorization assistant. Your task is to categorize posts into ONE of these categories ONLY: %s. If the post does not clearly fit into any of these categories, respond with 'None'. Respond with ONLY the category name or 'None', nothing else.", strings.Join(topics, ", ")),
					Question: fmt.Sprintf("Categorize this post:\n\nTitle: %s\n\nContent: %s\n\nWhich single category best fits this post?", title, content),
					Priority: ai.PriorityLow,
					Model:    ai.BackgroundModel(),
					Caller:   "auto-tag-post",
				}

				tag, err := askLLM(prompt)
				if err != nil {
					app.Log("chat", "Error generating tag for post %s: %v", postID, err)
					continue
				}

				tag = strings.TrimSpace(tag)
				if tag == "None" || tag == "none" || tag == "" {
					continue
				}

				validTag := false
				for topic := range prompts {
					if strings.EqualFold(tag, topic) {
						tag = topic
						validTag = true
						break
					}
				}

				if !validTag {
					continue
				}

				event.Publish(event.Event{
					Type: event.TagGenerated,
					Data: map[string]interface{}{
						"post_id": postID,
						"tag":     tag,
						"type":    eventType,
					},
				})
				app.Log("chat", "Published generated tag for post %s: %s", postID, tag)
			}

			// Handle note tagging (free-form single tag)
			if eventType == "note" {
				noteID, ok := evt.Data["note_id"].(string)
				if !ok {
					continue
				}
				userID, ok := evt.Data["user_id"].(string)
				if !ok {
					continue
				}
				app.Log("chat", "Received tag generation request for note %s", noteID)

				prompt := &ai.Prompt{
					System:   "You are a note organization assistant. Given a note, suggest ONE short tag (1-2 words, lowercase) that best categorizes it. Examples: 'work', 'ideas', 'shopping', 'todo', 'recipe', 'travel', 'health', 'finance'. Respond with ONLY the tag, nothing else. If the note is too short or unclear, respond with 'personal'.",
					Question: content,
					Priority: ai.PriorityLow,
					Model:    ai.BackgroundModel(),
					Caller:   "auto-tag-note",
				}

				tag, err := askLLM(prompt)
				if err != nil {
					app.Log("chat", "Error generating tag for note %s: %v", noteID, err)
					continue
				}

				tag = strings.TrimSpace(strings.ToLower(tag))
				if tag == "" {
					tag = "personal"
				}
				// Limit to reasonable length
				if len(tag) > 20 {
					tag = tag[:20]
				}

				event.Publish(event.Event{
					Type: event.TagGenerated,
					Data: map[string]interface{}{
						"note_id": noteID,
						"user_id": userID,
						"tag":     tag,
						"type":    eventType,
					},
				})
				app.Log("chat", "Published generated tag for note %s: %s", noteID, tag)
			}
		}
	}()

	go generateSummaries()
	go cleanupIdleRooms()
}

func generateSummaries() {
	// On a fresh instance with no AI provider yet, skip quietly instead of
	// failing once per topic — the setup flow (/setup or `mu setup`) prompts
	// for a provider, after which summaries resume on the next cycle.
	if !ai.Configured() {
		app.Log("chat", "Skipping topic summaries — no AI provider configured (run setup or set a provider key)")
		return
	}

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

		resp, err := askLLM(&ai.Prompt{
			Rag:      ragContext,
			Question: prompt,
			Priority: ai.PriorityMedium,
			Model:    ai.BackgroundModel(),
			Caller:   "topic-summary",
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
	summaryMeta = SummaryMetadata{GeneratedAt: time.Now().UTC(), Source: "Mu indexed public content", Status: "fresh"}
	mutex.Unlock()

	// Save summaries to disk
	if err := data.SaveJSON("chat_summaries.json", summaries); err != nil {
		app.Log("chat", "Error saving summaries: %v", err)
	} else {
		app.Log("chat", "Saved %d summaries to disk", len(summaries))
	}
	if err := data.SaveJSON("chat_summaries_meta.json", summaryMeta); err != nil {
		app.Log("chat", "Error saving summary metadata: %v", err)
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

	switch r.Method {
	case "GET":
		handleGetChat(w, r, roomID)
	case "DELETE":
		handleClearChat(w, r, roomID)
	default:
		// Saying something into a room is a websocket frame, not a POST. The
		// POST that used to be here asked a model a question, which is what
		// /agent is for.
		http.Error(w, "Messages are sent over the room websocket; ask the agent at /agent", http.StatusMethodNotAllowed)
	}
}

// handleClearChat handles DELETE /chat - clear chat history (admin only)
func handleClearChat(w http.ResponseWriter, r *http.Request, roomID string) {
	// Require admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if roomID != "" {
		// Clear room messages
		roomsMutex.Lock()
		if room, exists := rooms[roomID]; exists {
			room.mutex.Lock()
			room.Messages = nil
			room.mutex.Unlock()
		}
		roomsMutex.Unlock()

		// Delete persisted messages
		filename := "room_" + strings.ReplaceAll(roomID, "/", "_") + ".json"
		data.DeleteFile(filename)
		app.Log("chat", "Admin cleared messages for room %s", roomID)
	}

	w.WriteHeader(http.StatusOK)
}

// handleGetChat handles GET /chat - returns chat info as JSON or HTML
func handleGetChat(w http.ResponseWriter, r *http.Request, roomID string) {
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
			app.Log("chat", "TIMEOUT creating room %s - likely blocked on data.ByID()", roomID)
			http.Error(w, "Room creation timeout - server may be busy indexing content. Please try again.", http.StatusRequestTimeout)
			return
		}
	}

	// Without a room id, a person goes to the lobby and a program gets the
	// catalogue.
	//
	// This showed everybody the catalogue: a list of topics, each with a
	// paragraph of summary under it and a Join link. Nobody arrives at a chat
	// wanting to read about rooms — they arrive wanting to be in one, and the
	// list was a page of prose standing between them and the only thing on it
	// worth doing. The channels are a row across the top of the room now, so
	// the list has nothing left to do that being in a room does not do better.
	//
	// A JSON caller still gets the list, because "what rooms are there" is a
	// real question for something that is not a person and cannot click.
	if roomID == "" {
		if app.WantsJSON(r) {
			listRooms(w, r)
			return
		}
		http.Redirect(w, r, "/chat?id="+lobbyID, http.StatusFound)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"room": roomData})
		return
	}

	guestNotice := ""
	if _, acc := auth.TrySession(r); acc == nil {
		guestNotice = guestChatAuthNotice()
	}

	roomJSON, _ := json.Marshal(roomData)
	title := "Chat"
	if t, ok := roomData["title"].(string); ok && t != "" {
		title = roomName(t)
	}

	// Which room this is, inside the content rather than at the end of <body>.
	//
	// It was a global assigned by a script appended to the document, and soft
	// navigation swaps #content and re-runs only the scripts inside it — so
	// arriving here by clicking a link left the browser with whatever global the
	// last full page load had set, which is nothing on the way in and the
	// previous room on the way out. Inside the content it travels with the page
	// it describes.
	//
	// Data rather than code: a JSON block is read, never executed, so there is
	// no ordering to get right and nothing to leak into the next page. json
	// escapes < as <, so a room title cannot close the tag.
	about := aboutRoom(roomData)

	content := fmt.Sprintf(Template, channels(roomID)+guestNotice+about) +
		`<script type="application/json" id="room-data">` + string(roomJSON) + `</script>`

	app.Respond(w, r, app.Response{Title: title, Description: "Live discussion", HTML: content})
}

// aboutRoom is what this room is about: the summary, foldable, and the thing it
// came from.
//
// Folded. It was open, on the argument that a room you have just arrived in
// needs saying what it is — which is true once, and this is above the messages
// on every visit afterwards. A summary is two or three sentences and the box
// it sits in took most of a phone screen, so the room a person came to read
// started below the fold. One line now, and a tap when the question is
// actually "what is this".
//
// One of these. There were two, and both were on screen at once — this one
// above the messages, and a second built in JavaScript and inserted as the
// first thing inside #messages, reading "Discussion: <title>", the same summary
// again, a Hide summary link and → View Original. So the page opened by saying
// the same paragraph twice, a few pixels apart, with the room's name repeated
// between them. Reported as: chat summaries are above the message box and
// inside it.
//
// The server's copy is the one kept, for three reasons. It is there in the
// first paint rather than 100ms later, so the page does not visibly rearrange
// itself. It escapes the summary; the JavaScript wrote it through innerHTML,
// which makes any room whose summary came from a fetched page an injection.
// And it is above #messages rather than inside it, so it stays put while the
// conversation scrolls, instead of being a "message" that nobody sent which
// scrolls away and never comes back.
//
// What the JavaScript had and this did not is now here: folding it away, and
// the link to whatever the room is about. Those were the reasons to keep the
// other one, so they had to move rather than be lost.
//
// <details> rather than a toggle script. The open and closed states are the
// element's own, they work before any JavaScript runs and after soft
// navigation, and there is no display property to be argued with — the old
// toggle set style.display, which is one !important away from doing nothing at
// all. See test/reveal_test.go.
func aboutRoom(roomData map[string]interface{}) string {
	sum, _ := roomData["summary"].(string)
	sum = strings.TrimSpace(sum)
	src, _ := roomData["url"].(string)
	src = strings.TrimSpace(src)
	if sum == "" && src == "" {
		return ""
	}

	// A room made from something on this instance links back to it; one made
	// from a fetched page links out. Same question either way — what is this
	// about — so it is one link with the wording the destination deserves.
	link := ""
	if src != "" {
		label := "View original"
		rel := ""
		if !strings.HasPrefix(src, "/") {
			rel = ` target="_blank" rel="noopener noreferrer"`
		}
		link = `<a class="link room-source" href="` + htmlpkg.EscapeString(src) + `"` + rel + `>` +
			label + ` →</a>`
	}

	if sum == "" {
		return `<p class="room-about">` + link + `</p>`
	}
	return `<details class="room-about">` +
		`<summary>About this room</summary>` +
		`<p>` + htmlpkg.EscapeString(sum) + `</p>` + link +
		`</details>`
}

// roomName is a room's title without the word nobody needed.
//
// Every room was named "<something> Discussion" — Dev Discussion, Post
// Discussion, Daily Reminder Discussion — so the browser tab, the heading and
// the nav all carried a noun that is true of every room on the page and
// distinguishes none of them. New rooms are named without it.
//
// Trimmed here as well as at the source, because a room's title is stored with
// it: the rooms that already exist keep the name they were given, and a
// migration to delete one word from a title is not worth writing.
func roomName(title string) string {
	if t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(title), "Discussion")); t != "" {
		return t
	}
	return title
}

// listRooms renders what is being discussed right now.
//
// Two kinds of room appear. Topic rooms are permanent — one per entry in
// prompts.json — and carry the generated summary of what is current in them,
// which is what makes an empty room worth walking into. Item rooms are
// ephemeral, created when someone opens a discussion on a post, a news story
// or a video, and are only listed while they have messages or people in them.
func listRooms(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	topicsData := append([]string(nil), topics...)
	summariesData := make(map[string]string, len(summaries))
	for k, v := range summaries {
		summariesData[k] = v
	}
	meta := currentSummaryMeta()
	mutex.RUnlock()

	var live []RoomInfo
	roomsMutex.RLock()
	for _, room := range rooms {
		room.mutex.RLock()
		// A topic room is already listed below under its own name.
		if !strings.HasPrefix(room.ID, "chat_") && (len(room.Messages) > 0 || len(room.Clients) > 0) {
			live = append(live, RoomInfo{
				ID: room.ID, Type: room.Type, Title: room.Title,
				Participants: len(room.Clients), LastActivity: room.LastActivity,
			})
		}
		room.mutex.RUnlock()
	}
	roomsMutex.RUnlock()
	sort.Slice(live, func(i, j int) bool { return live[i].LastActivity.After(live[j].LastActivity) })

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"topics":       topicsData,
			"summaries":    summariesData,
			"summary_meta": meta,
			"rooms":        live,
		})
		return
	}

	var b strings.Builder
	b.WriteString(`<div class="rooms">`)

	if len(live) > 0 {
		b.WriteString(`<h3>Happening now</h3>`)
		for _, room := range live {
			b.WriteString(`<div class="summary-item"><a class="link" href="/chat?id=` +
				htmlpkg.EscapeString(room.ID) + `"><strong>` + htmlpkg.EscapeString(room.Title) + `</strong></a>`)
			b.WriteString(`<p class="summary-meta">` + describeRoom(room) + `</p></div>`)
		}
	}

	b.WriteString(`<h3>Topics</h3>`)
	for _, topic := range topicsData {
		b.WriteString(`<div class="summary-item"><span class="category">` + htmlpkg.EscapeString(topic) + `</span>`)
		if s := summariesData[topic]; s != "" {
			b.WriteString(`<p>` + htmlpkg.EscapeString(s) + `</p>`)
		}
		b.WriteString(`<a class="link" href="/chat?id=chat_` + url.QueryEscape(topic) + `">Join →</a></div>`)
	}
	if len(summariesData) > 0 {
		b.WriteString(`<p class="summary-meta">Summaries: ` + htmlpkg.EscapeString(meta.Source) + ` · ` + htmlpkg.EscapeString(meta.Status) + `</p>`)
	}
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Chat", Description: "Live discussion rooms", HTML: b.String()})
}

// describeRoom says who is there and when it last moved, which is the whole
// basis for deciding whether to walk in.
func describeRoom(room RoomInfo) string {
	var parts []string
	switch room.Participants {
	case 0:
		parts = append(parts, "nobody here now")
	case 1:
		parts = append(parts, "1 person here")
	default:
		parts = append(parts, fmt.Sprintf("%d people here", room.Participants))
	}
	if !room.LastActivity.IsZero() {
		parts = append(parts, "last message "+app.TimeAgo(room.LastActivity))
	}
	return strings.Join(parts, " · ")
}

// guestChatAuthNotice tells a signed-out reader why they cannot send here, and
// where they can.
//
// The where matters and was wrong: this offered "the public agent" at /agent,
// and /agent checks auth in its handler and bounces to /login. So the one line
// on the page addressed to people with no account sent them to the one page
// that will not open without one. The link read as a way in and was a wall.
//
// The front page is the door. Its box posts to /agent and a stranger gets an
// answer inline, bounded and public — that is the thing being offered, so it is
// what the link names and where it goes.
func guestChatAuthNotice() string {
	return `<div id="chat-auth-notice" class="notice">
  <strong>Sign in to use saved chat.</strong>
  <p>This room keeps conversation history for your account, so sending here needs a login. The box on the front page answers without one.</p>
  <p><a class="link" href="/">Ask without an account</a> · <a class="link" href="/login?redirect=/chat">Log in</a> · <a class="link" href="/signup?redirect=/chat">Sign up</a></p>
</div>`
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
