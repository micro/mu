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
)

//go:embed *.json
var f embed.FS

type Prompt struct {
	System   string   `json:"system"` // System prompt override
	Rag      []string `json:"rag"`
	Context  History  `json:"context"`
	Question string   `json:"question"`
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
</form>`

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

// ChatRoom represents a discussion room for a specific item.
// Room state is ephemeral - messages exist only in memory while the server runs.
// The last 20 messages are kept in memory for new joiners.
// Client-side sessionStorage is used so participants see their conversation until they leave.
type ChatRoom struct {
	ID         string                      // e.g., "post_123", "news_456", "video_789"
	Type       string                      // "post", "news", "video"
	Title      string                      // Item title
	Summary    string                      // Item summary/description
	URL        string                      // Original item URL
	Messages   []RoomMessage               // Last 20 messages (in-memory only)
	Clients    map[*websocket.Conn]*Client // Connected clients
	Broadcast  chan RoomMessage            // Broadcast channel
	Register   chan *Client                // Register client
	Unregister chan *Client                // Unregister client
	mutex      sync.RWMutex
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
	Conn   *websocket.Conn
	UserID string
	Room   *ChatRoom
}

var rooms = make(map[string]*ChatRoom)
var roomsMutex sync.RWMutex

// getOrCreateRoom gets an existing room or creates a new one
func getOrCreateRoom(id string) *ChatRoom {
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
	room := &ChatRoom{
		ID:         id,
		Type:       itemType,
		Clients:    make(map[*websocket.Conn]*Client),
		Broadcast:  make(chan RoomMessage, 256),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Messages:   make([]RoomMessage, 0, 20),
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
		} else if room.Title == "" {
			app.Log("chat", "News item %s not found in index", itemID)
			room.Title = "News Discussion"
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

	go room.run()

	app.Log("chat", "[getOrCreateRoom] Created room %s (total time %v)", id, time.Since(start))
	return room
}

// broadcastUserList sends the current list of usernames to all clients
func (room *ChatRoom) broadcastUserList() {
	room.mutex.RLock()
	usernames := make([]string, 0, len(room.Clients))
	for _, client := range room.Clients {
		usernames = append(usernames, client.UserID)
	}
	room.mutex.RUnlock()

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
func (room *ChatRoom) run() {
	for {
		select {
		case client := <-room.Register:
			room.mutex.Lock()
			room.Clients[client.Conn] = client
			room.mutex.Unlock()

			// Broadcast updated user list
			room.broadcastUserList()

		case client := <-room.Unregister:
			room.mutex.Lock()
			if _, ok := room.Clients[client.Conn]; ok {
				delete(room.Clients, client.Conn)
				client.Conn.Close()
			}
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
			room.mutex.Unlock()

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

// handleWebSocket handles WebSocket connections for chat rooms
func handleWebSocket(w http.ResponseWriter, r *http.Request, room *ChatRoom) {
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
				// Check if this is a direct message or should go to LLM
				if strings.HasPrefix(strings.TrimSpace(content), "@") {
					// Direct message - just broadcast it
					room.Broadcast <- RoomMessage{
						UserID:    client.UserID,
						Content:   content,
						Timestamp: time.Now(),
						IsLLM:     false,
					}
				} else {
					// Regular message - broadcast user message first
					userMsg := RoomMessage{
						UserID:    client.UserID,
						Content:   content,
						Timestamp: time.Now(),
						IsLLM:     false,
					}
					room.Broadcast <- userMsg

					// Then invoke LLM and broadcast response
					go func() {
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
							}
							if room.URL != "" {
								roomContext += " (Source: " + room.URL + ")"
							}
							ragContext = append(ragContext, roomContext)
							app.Log("chat", "Added room context: %s", roomContext)
						} else {
							app.Log("chat", "No room context available - Title: '%s', Summary: '%s'", room.Title, room.Summary)
						}

						// Perform multiple searches to gather comprehensive context
						seenIDs := make(map[string]bool)
						
						// Search 1: Question + title context for related content
						searchQuery1 := content
						if room.Title != "" {
							searchQuery1 = room.Title + " " + content
						}
						ragEntries1 := data.Search(searchQuery1, 5)
						app.Log("chat", "Search 1 (title+question) for '%s' returned %d results", searchQuery1, len(ragEntries1))
						
						// Search 2: Just the question to find directly relevant content
						ragEntries2 := data.Search(content, 5)
						app.Log("chat", "Search 2 (question only) for '%s' returned %d results", content, len(ragEntries2))
						
						// Combine and deduplicate results
						allEntries := append(ragEntries1, ragEntries2...)
						for _, entry := range allEntries {
							if seenIDs[entry.ID] {
								continue
							}
							seenIDs[entry.ID] = true
							
							contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
							if len(contextStr) > 1000 {
								contextStr = contextStr[:1000] + "..."
							}
							if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
								contextStr += fmt.Sprintf(" (Source: %s)", url)
							}
							ragContext = append(ragContext, contextStr)
						}

						app.Log("chat", "Total RAG context items: %d", len(ragContext))

						prompt := &Prompt{
							Rag:      ragContext,
							Context:  nil, // No history in rooms for now
							Question: content,
						}

						resp, err := askLLM(prompt)
						if err == nil && len(resp) > 0 {
							llmMsg := RoomMessage{
								UserID:    "AI",
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

	go generateSummaries()
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
		})

		if err != nil {
			app.Log("chat", "Failed to generate summary for topic %s: %v", topic, err)
			continue
		}
		newSummaries[topic] = resp
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

	time.Sleep(time.Hour)

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
		// Get room data with timeout to prevent hanging
		roomData := map[string]interface{}{}
		if roomID != "" {
			app.Log("chat", "GET request for room: %s", roomID)
			type roomResult struct {
				room *ChatRoom
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
		topicTabs := app.Head("chat", topics)
		summariesJSON, _ := json.Marshal(summaries)
		mutex.RUnlock()

		roomJSON, _ := json.Marshal(roomData)

		tmpl := app.RenderHTMLForRequest("Chat", "Chat with AI", fmt.Sprintf(Template, topicTabs), r)
		tmpl = strings.Replace(tmpl, "</body>", fmt.Sprintf(`<script>var summaries = %s; var roomData = %s;</script></body>`, summariesJSON, roomJSON), 1)

		w.Write([]byte(tmpl))
		return
	}

	if r.Method == "POST" {
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

			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			form["context"] = ictx
			form["prompt"] = msg
		}

		var context History

		if vals := form["context"]; vals != nil {
			cvals := vals.([]interface{})
			// Keep only the last 5 messages to reduce context size
			startIdx := 0
			if len(cvals) > 5 {
				startIdx = len(cvals) - 5
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

			output := fmt.Sprintf(Template, head, messages)
			renderHTML := app.RenderHTMLForRequest("Chat", "Chat with AI", output, r)

			w.Write([]byte(renderHTML))
			return
		}

		// Get topic for enhanced RAG
		topic := ""
		if t := form["topic"]; t != nil {
			topic = fmt.Sprintf("%v", t)
		}

		// Search the index for relevant context (RAG)
		// If topic is provided, use it as additional context for search
		searchQuery := q
		if len(topic) > 0 {
			searchQuery = topic + " " + q
		}
		ragEntries := data.Search(searchQuery, 3)
		var ragContext []string
		for _, entry := range ragEntries {
			// Debug: Show raw entry
			app.Log("chat", "[RAG DEBUG] Entry: Type=%s, Title=%s, Content=%s", entry.Type, entry.Title, entry.Content)

			// Format each entry as context
			contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
			if len(contextStr) > 500 {
				contextStr = contextStr[:500]
			}
			if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
				contextStr += fmt.Sprintf(" (Source: %s)", url)
			}
			ragContext = append(ragContext, contextStr)
		}

		// Debug: Log what we found
		if len(ragEntries) > 0 {
			app.Log("chat", "[RAG] Query: %s", searchQuery)
			app.Log("chat", "[RAG] Found %d entries:", len(ragEntries))
			for i, entry := range ragEntries {
				app.Log("chat", "  %d. [%s] %s", i+1, entry.Type, entry.Title)
			}
			app.Log("chat", "[RAG] Context being sent to LLM:")
			for i, ctx := range ragContext {
				app.Log("chat", "  %d. %s", i+1, ctx)
			}
		} else {
			app.Log("chat", "[RAG] Query: %s - NO RESULTS", searchQuery)
		}

		prompt := &Prompt{
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
		messages += fmt.Sprintf(`<div class="message"><span class="llm">llm</span><p>%v</p></div>`, form["answer"])

		output := fmt.Sprintf(Template, head, messages)
		renderHTML := app.RenderHTMLForRequest("Chat", "Chat with AI", output, r)

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
