// Package profile is the public face of an account: the page at /@username,
// and who is online.
//
// It was internal/user, which collided by name with service/user and described
// neither of them. This is not the account — that is internal/auth, which holds
// identity and credentials — and it is not a service: the Profile type carries
// a user id and a timestamp, and the page it renders is assembled from
// auth.Account plus posts and apps fetched through hooks. A Spec over it would
// be a Spec over a view.
package profile

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
	"mu/internal/flag"
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

// GetUserPosts returns an account's posts. Wired from main.go.
//
// By id and name, not name alone: the blog links an author at /@<id> and shows
// their display name, so matching posts on the name only worked when the two
// agreed. They did not for the system user — posts signed "Mu", id "micro" —
// so every digest linked to a profile with no posts on it.
var GetUserPosts func(authorID, authorName string) []UserPost

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
// Profile is what an account looks like to other people. It used to also hold
// a status message and a hundred entries of status history, which fed a stream
// on the home screen — see the comment on Handler.
type Profile struct {
	UserID    string    `json:"user_id"`
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
	flag.CheckContent("ai_response", askerID, "", response)
	item := flag.Item("ai_response", askerID)
	if item != nil && item.Flagged {
		app.Log("moderation", "AI response flagged for %s — banning asker", askerID)
		auth.BanAccount(askerID)
		return false
	}
	return true
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path (remove /@ prefix)
	username := strings.TrimPrefix(r.URL.Path, "/@")
	username = strings.TrimSuffix(username, "/")
	username = strings.ToLower(username)

	if username == "" {
		http.Redirect(w, r, "/home", 302)
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
		posts := GetUserPosts(acc.ID, acc.Name)

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

	// Check if viewing own profile
	sess, _ := auth.TrySession(r)
	isOwnProfile := sess != nil && sess.Account == username

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
				appsSB.WriteString(fmt.Sprintf(`<div class="post-item"><h3><a href="/apps/%s/run" style="display:flex;align-items:center;gap:8px"><span class="profile-app-icon">%s</span> %s</a></h3><p class="info">%s</p></div>`, a.Slug, icon, a.Name, desc))
			}
			appsSection = appsSB.String()
		}
	}

	// Verified badge — green tick for accounts with a verified email,
	// admins, or admin-approved accounts. Skipped on instances without
	// email verification configured.
	verifiedBadge := ""
	if acc.Admin || acc.Approved || acc.EmailVerified {
		verifiedBadge = ` <span title="Verified" aria-label="Verified" style="display:inline-block;vertical-align:middle;width:16px;height:16px;background:#22c55e;color:#fff;border-radius:50%;text-align:center;line-height:16px;font-size:11px;font-weight:700">✓</span>`
	}

	// Build the profile page content
	content := fmt.Sprintf(`<div class="max-w-xl">
<div class="mb-6" style="padding-bottom: 20px; border-bottom: 2px solid #333;">
<p class="info m-0">@%s%s</p>
<p class="info mt-3">Joined %s</p>
%s
</div>

%s

<h3 class="mb-5">Posts (%d)</h3>
%s
</div>`, acc.ID, verifiedBadge, acc.Created.Format("January 2006"), messageLink, appsSection, postCount, userPosts)

	// Use name as page title
	html := app.RenderHTMLForRequest(acc.Name, fmt.Sprintf("Profile of %s", acc.Name), content, r)
	w.Write([]byte(html))
}

// avatarColors are the palette used for status card avatars.
