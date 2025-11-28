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
	"mu/app"
	"mu/auth"
	"mu/blog"
	"mu/data"
)

//go:embed *.json
var f embed.FS

type Prompt struct {
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

// ChatRoom represents a discussion room for a specific item
type ChatRoom struct {
	ID          string                 // e.g., "post_123", "news_456", "video_789"
	Type        string                 // "post", "news", "video"
	Title       string                 // Item title
	Summary     string                 // Item summary/description
	URL         string                 // Original item URL
	Messages    []RoomMessage          // Last 20 messages
	Clients     map[*websocket.Conn]*Client // Connected clients
	Broadcast   chan RoomMessage       // Broadcast channel
	Register    chan *Client           // Register client
	Unregister  chan *Client           // Unregister client
	mutex       sync.RWMutex
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
	roomsMutex.Lock()
	defer roomsMutex.Unlock()

	if room, exists := rooms[id]; exists {
		return room
	}

	// Parse the ID to determine type and fetch item details
	parts := strings.SplitN(id, "_", 2)
	if len(parts) != 2 {
		return nil
	}

	itemType := parts[0]
	itemID := parts[1]

	room := &ChatRoom{
		ID:         id,
		Type:       itemType,
		Clients:    make(map[*websocket.Conn]*Client),
		Broadcast:  make(chan RoomMessage, 256),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Messages:   make([]RoomMessage, 0, 20),
	}

	// Fetch item details based on type
	switch itemType {
	case "post":
		if post := blog.GetPost(itemID); post != nil {
			room.Title = post.Title
			if room.Title == "" {
				room.Title = "Untitled Post"
			}
			// Truncate content for summary
			room.Summary = post.Content
			if len(room.Summary) > 200 {
				room.Summary = room.Summary[:200] + "..."
			}
			room.URL = "/post?id=" + itemID
		}
	case "news":
		// For news, lookup by exact ID
		entry := data.GetByID(itemID)
		if entry != nil {
			room.Title = entry.Title
			room.Summary = entry.Content
			if len(room.Summary) > 200 {
				room.Summary = room.Summary[:200] + "..."
			}
			if url, ok := entry.Metadata["url"].(string); ok {
				room.URL = url
			}
		}
	case "video":
		// For videos, lookup by exact ID
		entry := data.GetByID(itemID)
		if entry != nil {
			room.Title = entry.Title
			room.Summary = entry.Content
			if len(room.Summary) > 200 {
				room.Summary = room.Summary[:200] + "..."
			}
			if url, ok := entry.Metadata["url"].(string); ok {
				room.URL = url
			}
		}
	}

	rooms[id] = room
	go room.run()

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
		fmt.Println("WebSocket upgrade error:", err)
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
						}
						
						// Search for additional context (only if needed)
						searchQuery := content
						if room.Title != "" {
							searchQuery = room.Title + " " + content
						}
						
						ragEntries := data.Search(searchQuery, 2) // Reduced to 2 since room context is primary
						for _, entry := range ragEntries {
							contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
							if len(contextStr) > 500 {
								contextStr = contextStr[:500]
							}
							if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
								contextStr += fmt.Sprintf(" (Source: %s)", url)
							}
							ragContext = append(ragContext, contextStr)
						}

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
		fmt.Println("Error parsing topics.json", err)
	}

	for topic, _ := range prompts {
		topics = append(topics, topic)
	}

	sort.Strings(topics)

	// Generate head with topics (rooms will be added dynamically)
	head = app.Head("chat", topics)

	// Load existing summaries from disk
	if b, err := data.LoadFile("chat_summaries.json"); err == nil {
		if err := json.Unmarshal(b, &summaries); err != nil {
			fmt.Println("Error loading summaries:", err)
		} else {
			fmt.Println("Loaded", len(summaries), "summaries from disk")
		}
	}

	go generateSummaries()
}

func generateSummaries() {
	fmt.Println("Generating summaries", time.Now().String())

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
			fmt.Println("Failed to generate summary for topic:", topic, err)
			continue
		}
		newSummaries[topic] = resp
	}

	mutex.Lock()
	summaries = newSummaries
	mutex.Unlock()

	// Save summaries to disk
	if err := data.SaveJSON("chat_summaries.json", summaries); err != nil {
		fmt.Println("Error saving summaries:", err)
	} else {
		fmt.Println("Saved", len(summaries), "summaries to disk")
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
		mutex.RLock()

		// Use Head() to format topics (rooms don't appear in topic list)
		topicTabs := app.Head("chat", topics)

		// Pass summaries and room info as JSON to frontend
		summariesJSON, _ := json.Marshal(summaries)
		roomData := map[string]interface{}{}
		if roomID != "" {
			room := getOrCreateRoom(roomID)
			if room != nil {
				roomData["id"] = roomID
				roomData["title"] = room.Title
				roomData["summary"] = room.Summary
				roomData["url"] = room.URL
				roomData["isRoom"] = true
			}
		}
		roomJSON, _ := json.Marshal(roomData)

		tmpl := app.RenderHTMLForRequest("Chat", "Chat with AI", fmt.Sprintf(Template, topicTabs), r)
		tmpl = strings.Replace(tmpl, "</body>", fmt.Sprintf(`<script>var summaries = %s; var roomData = %s;</script></body>`, summariesJSON, roomJSON), 1)

		mutex.RUnlock()

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
			fmt.Printf("[RAG DEBUG] Entry: Type=%s, Title=%s, Content=%s\n", entry.Type, entry.Title, entry.Content)
			
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
			fmt.Printf("[RAG] Query: %s\n", searchQuery)
			fmt.Printf("[RAG] Found %d entries:\n", len(ragEntries))
			for i, entry := range ragEntries {
				fmt.Printf("  %d. [%s] %s\n", i+1, entry.Type, entry.Title)
			}
			fmt.Printf("[RAG] Context being sent to LLM:\n")
			for i, ctx := range ragContext {
				fmt.Printf("  %d. %s\n", i+1, ctx)
			}
		} else {
			fmt.Printf("[RAG] Query: %s - NO RESULTS\n", searchQuery)
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
