package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/app"

	"mu/auth"
	"mu/blog"
	"mu/data"
)

var profileMutex sync.RWMutex
var profiles = map[string]*Profile{}

// Profile stores additional user information beyond the Account
type Profile struct {
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`     // User's custom status message
	UpdatedAt time.Time `json:"updated_at"` // When the profile was last updated
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
	users := auth.GetOnlineUsers()

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
	users := auth.GetOnlineUsers()
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

// GetProfile retrieves a user's profile, creating a default one if it doesn't exist
func GetProfile(userID string) *Profile {
	profileMutex.RLock()
	profile, exists := profiles[userID]
	profileMutex.RUnlock()

	if !exists {
		profile = &Profile{
			UserID:    userID,
			Status:    "",
			UpdatedAt: time.Now(),
		}
	}

	return profile
}

// UpdateProfile saves a user's profile
func UpdateProfile(profile *Profile) error {
	profileMutex.Lock()
	defer profileMutex.Unlock()

	profile.UpdatedAt = time.Now()
	profiles[profile.UserID] = profile
	data.SaveJSON("profiles.json", profiles)

	return nil
}

// Handler renders a user profile page at /@username
func Handler(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path (remove /@ prefix)
	username := strings.TrimPrefix(r.URL.Path, "/@")
	username = strings.TrimSuffix(username, "/")

	if username == "" {
		http.Redirect(w, r, "/home", 302)
		return
	}

	// Handle POST request for status update
	if r.Method == "POST" {
		sess, _, err := auth.RequireSession(r)
		if err != nil {
			app.Unauthorized(w, r)
			return
		}

		// Only allow updating own status
		if sess.Account != username {
			app.Forbidden(w, r, "")
			return
		}

		status := r.FormValue("status")
		if len(status) > 100 {
			status = status[:100]
		}

		profile := GetProfile(sess.Account)
		profile.Status = status
		UpdateProfile(profile)

		// Redirect back to profile
		http.Redirect(w, r, "/@"+sess.Account, http.StatusSeeOther)
		return
	}

	// Get the user account
	acc, err := auth.GetAccount(username)
	if err != nil {
		http.Error(w, "User not found", 404)
		return
	}

	// Get all posts by this user
	var userPosts string
	posts := blog.GetPostsByAuthor(acc.Name)

	// Check if viewer is admin
	_, viewerAcc := auth.TrySession(r)
	isAdmin := viewerAcc != nil && viewerAcc.Admin

	// Filter private posts for non-admins
	var visiblePosts []*blog.Post
	for _, post := range posts {
		if !post.Private || isAdmin {
			visiblePosts = append(visiblePosts, post)
		}
	}

	postCount := len(visiblePosts)

	for _, post := range visiblePosts {
		title := post.Title
		if title == "" {
			title = "Untitled"
		}

		// Truncate content
		content := post.Content
		if len(content) > 300 {
			lastSpace := 300
			for i := 299; i >= 0 && i < len(content); i-- {
				if content[i] == ' ' {
					lastSpace = i
					break
				}
			}
			if lastSpace < len(content) {
				content = content[:lastSpace] + "..."
			}
		}

		// Linkify URLs and embed YouTube videos
		linkedContent := blog.Linkify(content)

		userPosts += fmt.Sprintf(`<div class="post-item">
<h3><a href="/post?id=%s">%s</a></h3>
<div class="mb-3">%s</div>
<div class="info">%s · <a href="/post?id=%s">Read more</a></div>
</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), post.ID)
	}

	if userPosts == "" {
		userPosts = "<p class='info'>No blog posts yet.</p>"
	}

	// Get user profile
	profile := GetProfile(acc.ID)

	// Check if viewing own profile
	sess, _ := auth.TrySession(r)
	isOwnProfile := sess != nil && sess.Account == username

	// Build status section
	statusSection := ""
	if profile.Status != "" {
		statusSection = fmt.Sprintf(`<p class="info italic mt-3">"%s"</p>`, profile.Status)
	}

	// Build status edit form (only for own profile)
	statusEditForm := ""
	if isOwnProfile {
		statusEditForm = fmt.Sprintf(`
<form method="POST" class="mt-4">
<input type="text" name="status" placeholder="Set your status..." value="%s" maxlength="100" class="form-input w-full">
<button type="submit" class="mt-2">Update Status</button>
</form>`, profile.Status)
	}

	// Build message link (only show if not own profile)
	messageLink := ""
	if !isOwnProfile {
		messageLink = fmt.Sprintf(`<p class="mt-4"><a href="/mail?compose=true&to=%s">Send a message</a></p>`, acc.ID)
	}

	// Apps section removed
	appsSection := ""

	// Build the profile page content
	content := fmt.Sprintf(`<div class="max-w-xl">
<div class="mb-6" style="padding-bottom: 20px; border-bottom: 2px solid #333;">
<p class="info m-0">@%s</p>
<p class="info mt-3">Joined %s</p>
%s
%s
%s
</div>

%s

<h3 class="mb-5">Posts (%d)</h3>
%s
</div>`, acc.ID, acc.Created.Format("January 2006"), statusSection, statusEditForm, messageLink, appsSection, postCount, userPosts)

	// Use name as page title
	html := app.RenderHTML(acc.Name, fmt.Sprintf("Profile of %s", acc.Name), content)
	w.Write([]byte(html))
}
