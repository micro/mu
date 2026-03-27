package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// UserPost is a simplified post representation for profile rendering.
// Wired from blog building block via GetUserPosts callback.
type UserPost struct {
	ID        string
	Title     string
	Content   string
	CreatedAt time.Time
	Private   bool
}

// GetUserPosts returns posts by author name. Wired from main.go.
var GetUserPosts func(authorName string) []UserPost

// UserApp is a simplified app representation for profile rendering.
type UserApp struct {
	Slug        string
	Name        string
	Description string
	Icon        string
}

// GetUserApps returns public apps by author ID. Wired from main.go.
var GetUserApps func(authorID string) []UserApp

// LinkifyContent converts URLs in text to clickable links. Wired from main.go.
var LinkifyContent func(text string) string

var profileMutex sync.RWMutex
var profiles = map[string]*Profile{}

// Profile stores additional user information beyond the Account
type Profile struct {
	UserID    string          `json:"user_id"`
	Status    string          `json:"status"`     // User's custom status message
	History   []StatusHistory `json:"history"`     // Past statuses, newest first
	UpdatedAt time.Time       `json:"updated_at"` // When the profile was last updated
}

// StatusHistory records a previous status.
type StatusHistory struct {
	Status string    `json:"status"`
	SetAt  time.Time `json:"set_at"`
}

// maxStatusHistory is the number of past statuses to keep per user.
const maxStatusHistory = 20

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

// UpdateProfile saves a user's profile. If the status changed and the
// previous status was non-empty, it's pushed onto the history.
func UpdateProfile(profile *Profile) error {
	profileMutex.Lock()
	defer profileMutex.Unlock()

	// Record previous status in history if it changed
	if old, ok := profiles[profile.UserID]; ok && old.Status != "" && old.Status != profile.Status {
		profile.History = append([]StatusHistory{{Status: old.Status, SetAt: old.UpdatedAt}}, profile.History...)
		if len(profile.History) > maxStatusHistory {
			profile.History = profile.History[:maxStatusHistory]
		}
	}

	profile.UpdatedAt = time.Now()
	profiles[profile.UserID] = profile
	data.SaveJSON("profiles.json", profiles)

	return nil
}

// StatusEntry represents a user's status for display on the home page.
type StatusEntry struct {
	UserID    string
	Name      string // display name
	Status    string
	UpdatedAt time.Time
}

// statusMaxAge is how old a status can be before it stops appearing on home.
const statusMaxAge = 7 * 24 * time.Hour

// RecentStatuses returns users who have set a status within the last 7 days,
// most recently updated first. Limited to max entries. Excludes the given userID.
func RecentStatuses(viewerID string, max int) []StatusEntry {
	profileMutex.RLock()
	defer profileMutex.RUnlock()

	cutoff := time.Now().Add(-statusMaxAge)
	var entries []StatusEntry
	for _, p := range profiles {
		if p.Status == "" || p.UserID == viewerID || p.UpdatedAt.Before(cutoff) {
			continue
		}
		name := p.UserID
		if acc, err := auth.GetAccount(p.UserID); err == nil {
			name = acc.Name
		}
		entries = append(entries, StatusEntry{
			UserID:    p.UserID,
			Name:      name,
			Status:    p.Status,
			UpdatedAt: p.UpdatedAt,
		})
	}
	// Sort newest first
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].UpdatedAt.After(entries[i].UpdatedAt) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	if len(entries) > max {
		entries = entries[:max]
	}
	return entries
}

// StatusHandler handles POST /user/status to update the current user's status.
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	status := r.FormValue("status")
	if len(status) > 100 {
		status = status[:100]
	}

	profile := GetProfile(sess.Account)
	profile.Status = status
	UpdateProfile(profile)

	// Redirect back to referrer or home
	ref := r.Header.Get("Referer")
	if ref == "" || !strings.HasPrefix(ref, "/") {
		// Extract path from full URL referer
		if i := strings.Index(ref, "://"); i >= 0 {
			if j := strings.Index(ref[i+3:], "/"); j >= 0 {
				ref = ref[i+3+j:]
			} else {
				ref = "/"
			}
		} else {
			ref = "/"
		}
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

// Handler renders a user profile page at /@username
func Handler(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path (remove /@ prefix)
	username := strings.TrimPrefix(r.URL.Path, "/@")
	username = strings.TrimSuffix(username, "/")
	username = strings.ToLower(username)

	if username == "" {
		http.Redirect(w, r, "/home", 302)
		return
	}

	// Handle POST request for status update (legacy, profile page form)
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

	// Get all posts by this user via callback (wired in main.go)
	var userPosts string
	var postCount int
	if GetUserPosts != nil {
		posts := GetUserPosts(acc.Name)

		// Check if viewer is admin
		_, viewerAcc := auth.TrySession(r)
		isAdmin := viewerAcc != nil && viewerAcc.Admin

		// Filter private posts for non-admins
		var visiblePosts []UserPost
		for _, post := range posts {
			if !post.Private || isAdmin {
				visiblePosts = append(visiblePosts, post)
			}
		}

		postCount = len(visiblePosts)

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
			linkedContent := content
			if LinkifyContent != nil {
				linkedContent = LinkifyContent(content)
			}

			userPosts += fmt.Sprintf(`<div class="post-item">
<h3><a href="/blog/post?id=%s">%s</a></h3>
<div class="mb-3">%s</div>
<div class="info">%s · <a href="/blog/post?id=%s">Read more</a></div>
</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), post.ID)
		}
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
	if len(profile.History) > 0 {
		statusSection += `<details style="margin-top:8px;"><summary style="font-size:13px;color:#999;cursor:pointer;">Status history</summary><div style="margin-top:6px;">`
		for _, h := range profile.History {
			statusSection += fmt.Sprintf(`<p style="font-size:13px;color:#888;margin:4px 0;font-style:italic;">"%s" <span style="color:#bbb;">— %s</span></p>`,
				h.Status, app.TimeAgo(h.SetAt))
		}
		statusSection += `</div></details>`
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

	// Apps section
	appsSection := ""
	if GetUserApps != nil {
		userApps := GetUserApps(acc.ID)
		if len(userApps) > 0 {
			var appsSB strings.Builder
			appsSB.WriteString(fmt.Sprintf(`<h3 class="mb-5">Apps (%d)</h3>`, len(userApps)))
			for _, a := range userApps {
				icon := a.Icon
				if icon == "" {
					icon = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>`
				}
				desc := a.Description
				if len(desc) > 80 {
					desc = desc[:80] + "..."
				}
				appsSB.WriteString(fmt.Sprintf(`<div class="post-item"><h3><a href="/apps/%s/run">%s %s</a></h3><p class="info">%s</p></div>`, a.Slug, icon, a.Name, desc))
			}
			appsSection = appsSB.String()
		}
	}

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
