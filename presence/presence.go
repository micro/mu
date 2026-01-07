package presence

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/app"
	"mu/auth"
)

// Client represents a connected user
type Client struct {
	Conn     *websocket.Conn
	UserID   string
	LastSeen time.Time
}

var (
	clients      = make(map[*websocket.Conn]*Client)
	clientsMutex sync.RWMutex
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// PresenceMessage is sent to clients
type PresenceMessage struct {
	Type  string   `json:"type"`
	Users []string `json:"users"`
	Count int      `json:"count"`
}

func Load() {
	// Start broadcaster that periodically sends presence updates
	go broadcaster()
}

func broadcaster() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		broadcastUserList()
	}
}

func broadcastUserList() {
	// Get online users from auth package
	users := auth.GetOnlineUsers()

	msg := PresenceMessage{
		Type:  "presence",
		Users: users,
		Count: len(users),
	}

	data, _ := json.Marshal(msg)

	clientsMutex.RLock()
	for conn := range clients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			conn.Close()
		}
	}
	clientsMutex.RUnlock()
}

// Handler handles WebSocket connections for presence
func Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.Log("presence", "WebSocket upgrade error: %v", err)
		return
	}

	// Get user session (optional - anonymous users can see presence but won't be shown)
	var userID string
	if sess, err := auth.GetSession(r); err == nil {
		userID = sess.Account
		// Update presence immediately
		auth.UpdatePresence(userID)
	}

	client := &Client{
		Conn:     conn,
		UserID:   userID,
		LastSeen: time.Now(),
	}

	clientsMutex.Lock()
	clients[conn] = client
	clientsMutex.Unlock()

	if userID != "" {
		app.Log("presence", "User connected: %s (total: %d)", userID, len(clients))
	}

	// Send current user list immediately
	users := auth.GetOnlineUsers()
	msg := PresenceMessage{
		Type:  "presence",
		Users: users,
		Count: len(users),
	}
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)

	// Handle incoming messages (pings to keep presence alive)
	go func() {
		defer func() {
			clientsMutex.Lock()
			delete(clients, conn)
			clientsMutex.Unlock()
			conn.Close()
			if userID != "" {
				app.Log("presence", "User disconnected: %s (total: %d)", userID, len(clients)-1)
			}
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Update presence on any message (heartbeat)
			if userID != "" {
				auth.UpdatePresence(userID)
			}
			clientsMutex.Lock()
			if c, ok := clients[conn]; ok {
				c.LastSeen = time.Now()
			}
			clientsMutex.Unlock()
		}
	}()
}
