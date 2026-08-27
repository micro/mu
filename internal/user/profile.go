// Package user is what is left of a person on this instance once the page
// about them is gone: whether they are here, and a moderation check.
//
// It rendered /@username — a name, a tick, a join date, a status box, an apps
// grid and their posts. That is a social network's page, and this is not one:
// internal/app/content.go deleted Save, Hide and Block on the grounds that
// "those three are the controls of a feed… Mu has no feed", and a profile is
// downstream of the same thing. Nobody asked to be looked at.
//
// /@somebody is the conversation with them now — see inbox/person.go. What an
// address is worth is what it lets you do, and a page you can only read does
// nothing.
//
// This is not the account either. internal/auth holds identity and credentials.
// What is here is furniture belonging to one account and answering no question
// about the world.
//
// # Why it is not a service
//
// It was two packages, and one of them was a service. internal/profile rendered
// the public page; service/user carried a Spec with seven methods over saving,
// hiding, flagging and blocking — which read as a service because it has a noun
// and some verbs, but the noun is the caller and the verbs change nothing
// anybody else can observe.
//
// That is the same shelf as changing your email or rotating a token: account
// furniture, which AGENTS.md already settles for the balance. A service answers
// a question about state that does not depend on who is asking; "what have I
// saved" is a question about the asker. The split also cost the obvious thing —
// two packages named for one person, and the page about you at /@you had no
// relationship in code with the page about what you kept at /user.
//
// The word user is still free for a service, and should be spent on the one
// that is missing: a directory. Who exists here, people and agents alike, and
// how to reach them. That is a question about the world, the answer does not
// depend on who asks, and nothing in this package is it.
package user

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/flag"
)

var profileMutex sync.RWMutex
var profiles = map[string]*Profile{}

// Profile is what an account looks like to other people.
//
// It held a status and a hundred entries of status history, feeding a stream on
// the home screen. The history and the stream went; the status went with them,
// which was one thing too many — a profile with a name and a join date on it is
// a record, and nobody opens a record twice. One line, no history, and nothing
// reads it but the profile page. See status.go.
type Profile struct {
	UserID    string    `json:"user_id"`
	Status    string    `json:"status,omitempty"`
	UpdatedAt time.Time `json:"updated_at"` // When the status was last set
}

// Presence tracking
var (
	presenceClients      = make(map[*websocket.Conn]*PresenceClient)
	presenceClientsMutex sync.RWMutex
)

var presenceUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// PresenceClient represents a connected user for presence tracking
type PresenceClient struct {
	Conn     *websocket.Conn
	UserID   string
	LastSeen time.Time
}

// PresenceMessage is sent to clients
type PresenceMessage struct {
	Type  string   `json:"type"`
	Users []string `json:"users"`
	Count int      `json:"count"`
}

func init() {
	b, _ := data.LoadFile("profiles.json")
	json.Unmarshal(b, &profiles)
}

// Load initializes presence broadcasting
func Load() {
	go presenceBroadcaster()
}

func presenceBroadcaster() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		broadcastPresence()
	}
}

func broadcastPresence() {
	users := auth.OnlineUsers()

	msg := PresenceMessage{
		Type:  "presence",
		Users: users,
		Count: len(users),
	}

	data, _ := json.Marshal(msg)

	presenceClientsMutex.RLock()
	for conn := range presenceClients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			conn.Close()
		}
	}
	presenceClientsMutex.RUnlock()
}

// PresenceHandler handles WebSocket connections for presence
func PresenceHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := presenceUpgrader.Upgrade(w, r, nil)
	if err != nil {
		app.Log("user", "WebSocket upgrade error: %v", err)
		return
	}

	var userID string
	sess, _ := auth.TrySession(r)
	if sess != nil {
		userID = sess.Account
		auth.UpdatePresence(userID)
	}

	client := &PresenceClient{
		Conn:     conn,
		UserID:   userID,
		LastSeen: time.Now(),
	}

	presenceClientsMutex.Lock()
	presenceClients[conn] = client
	presenceClientsMutex.Unlock()

	if userID != "" {
		app.Log("user", "Presence connected: %s (total: %d)", userID, len(presenceClients))
	}

	// Send current user list immediately
	users := auth.OnlineUsers()
	msg := PresenceMessage{
		Type:  "presence",
		Users: users,
		Count: len(users),
	}
	msgData, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, msgData)

	// Handle incoming messages (pings to keep presence alive)
	go func() {
		defer func() {
			presenceClientsMutex.Lock()
			delete(presenceClients, conn)
			presenceClientsMutex.Unlock()
			conn.Close()
			if userID != "" {
				app.Log("user", "Presence disconnected: %s", userID)
			}
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if userID != "" {
				auth.UpdatePresence(userID)
			}
			presenceClientsMutex.Lock()
			if c, ok := presenceClients[conn]; ok {
				c.LastSeen = time.Now()
			}
			presenceClientsMutex.Unlock()
		}
	}()
}

// AIResponseAllowed checks an AI-generated response BEFORE it is posted.
// Returns true if the response is safe to post. If the content is flagged, the
// requesting user is banned (admins are exempt).
//
// It stays here rather than going with the status stream it was written for:
// the stream service's @micro replies run through the same check, and that is
// the surviving one.
func AIResponseAllowed(askerID, response string) bool {
	if acc, err := auth.GetAccount(askerID); err == nil && acc.Admin {
		return true
	}
	event.Published("ai_response", askerID, "", response)
	item := flag.Item("ai_response", askerID)
	if item != nil && item.Flagged {
		app.Log("moderation", "AI response flagged for %s — banning asker", askerID)
		auth.BanAccount(askerID)
		return false
	}
	return true
}
