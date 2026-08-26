// Package user is a person on this instance: the face they show other people,
// whether they are here, and what they have decided about everybody else.
//
// This is not the account. internal/auth holds identity and credentials — who
// you are and how you prove it. This is what that identity looks like from
// outside (the page at /@username, presence) and the view it has of everyone
// else (what it saved, hid and blocked). Neither is a question about the world;
// both are furniture belonging to one account.
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
	"fmt"
	htmlpkg "html"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
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

// ProfileHandler serves /@username: what an account looks like to other people.
//
// Named for its page rather than left as the package's plain Handler, because
// this package now has two: the face you show other people, and the page about
// what you have decided to keep and hide, which is yours alone.
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
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

		for _, post := range visiblePosts {
			title := post.Title
			if title == "" {
				title = "Untitled"
			}
			// Title and when, and nothing else.
			//
			// This printed three hundred characters of the body under every
			// title, linkified, with a "Read more" after it — which is a feed,
			// and a feed on a person is the shape this page is getting out of.
			// What somebody wants here is to see that a thing exists and go to
			// it; the body of it is at the other end of the link.
			userPosts += fmt.Sprintf(`<div class="post-item"><h3><a href="/blog/post?id=%s">%s</a></h3>`+
				`<p class="info">%s</p></div>`,
				htmlpkg.EscapeString(post.ID), htmlpkg.EscapeString(title), app.TimeAgo(post.CreatedAt))
		}
	}

	// A heading only where there is something under it. "Posts (0)" and "No
	// blog posts yet" are both the page telling you about an absence, on a page
	// whose job is to tell you who somebody is and how to reach them.
	postsSection := ""
	if userPosts != "" {
		postsSection = `<h3 class="mb-5">Posts</h3>` + userPosts
	}

	// Check if viewing own profile
	sess, _ := auth.TrySession(r)
	isOwnProfile := sess != nil && sess.Account == username

	// Somewhere to write to them, and what they are doing.
	//
	// The link used to be /mail?compose=true, a screen that no longer exists —
	// so the one action on a profile went nowhere. It is /inbox/new at their
	// address now, which is the person rather than their agent. See status.go.
	messageLink := ""
	if !isOwnProfile {
		messageLink = writeLink(acc.ID)
	}
	csrf := ""
	if sess != nil {
		csrf = auth.CSRFToken(r)
	}
	status := statusBlock(acc.ID, isOwnProfile, csrf)

	// Apps section
	appsSection := ""
	if GetUserApps != nil {
		userApps := GetUserApps(acc.ID)
		if len(userApps) > 0 {
			var appsSB strings.Builder
			appsSB.WriteString(`<h3 class="mb-5">Apps</h3>`)
			for _, a := range userApps {
				icon := a.Icon
				if icon == "" {
					icon = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>`
				}
				desc := a.Description
				if len(desc) > 80 {
					desc = desc[:80] + "..."
				}
				appsSB.WriteString(fmt.Sprintf(`<div class="post-item"><h3><a href="/apps/%s" class="d-flex items-center gap-2"><span class="profile-app-icon">%s</span> %s</a></h3><p class="info">%s</p></div>`, a.Slug, icon, a.Name, desc))
			}
			appsSection = appsSB.String()
		}
	}

	// Verified badge — green tick for accounts with a verified email,
	// admins, or admin-approved accounts. Skipped on instances without
	// email verification configured.
	verifiedBadge := ""
	if acc.Admin || acc.Approved || acc.EmailVerified {
		verifiedBadge = ` <span title="Verified" aria-label="Verified" class="verified">✓</span>`
	}

	// The address, which this page has always known and never shown.
	//
	// writeLink already asks for it — its comment says "addressOf answers that
	// by producing the address, which is not shown" — so the one fact a
	// directory entry exists to carry was computed here and discarded, while
	// the page led with a count of blog posts.
	//
	// A profile is the answer to "who is this and how do I reach them". That is
	// what users_find says it is for, in its own documentation: turning a name
	// somebody mentioned into an address you can write to. The rest of this
	// page is an index of what they have published, underneath, without
	// tallies — a number beside somebody's name is a scoreboard, and nobody
	// has ever needed to know how many posts a person has.
	addr := ""
	if a := addressOf(acc.ID); a != "" {
		addr = `<p class="pf-addr"><code>` + htmlpkg.EscapeString(a) + `</code></p>`
	}

	content := fmt.Sprintf(`<div class="max-w-xl">
<div class="mb-6 page-head">
<p class="info m-0">@%s%s</p>
%s
%s
<p class="info mt-3">Joined %s</p>
%s
</div>

%s

%s
</div>`, acc.ID, verifiedBadge, status, addr, acc.Created.Format("January 2006"),
		messageLink, appsSection, postsSection)

	// Use name as page title
	app.Respond(w, r, app.Response{Title: acc.Name, Description: fmt.Sprintf("Profile of %s", acc.Name), HTML: content})
}

// avatarColors are the palette used for status card avatars.
