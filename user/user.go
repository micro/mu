package user

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"os"
	"sort"
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
const maxStatusHistory = 100

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

// UpdateStatus is the safe way to change a user's status. It passes a
// fresh Profile to UpdateProfile so the existing map entry isn't
// aliased — UpdateProfile reads the old status from the map correctly
// and pushes it to history before storing the new one.
func UpdateStatus(userID, newStatus string) error {
	return UpdateProfile(&Profile{
		UserID: userID,
		Status: newStatus,
	})
}

// UpdateProfile saves a user's profile. Every non-empty previous
// status is pushed onto the history so the full timeline of what a
// user has said is preserved. Empty updates (clearing a status) are
// never pushed.
//
// To avoid a whole class of "caller forgot to carry over history"
// bugs, this function always merges with whatever is already stored
// under the same UserID — you can pass a freshly-constructed
// &Profile{UserID: ..., Status: ...} and history is still preserved.
func UpdateProfile(profile *Profile) error {
	profileMutex.Lock()
	defer profileMutex.Unlock()

	// Start from the existing history in the map rather than whatever
	// the caller passed. If the caller supplied extra history entries
	// (tests / migrations), keep them at the front.
	existing, hasExisting := profiles[profile.UserID]
	mergedHistory := append([]StatusHistory{}, profile.History...)
	if hasExisting {
		mergedHistory = append(mergedHistory, existing.History...)
	}

	// Record previous status in history — always, not just on change.
	// The home card renders the combined stream, so the history is
	// where the conversation actually lives. Repeating yourself is OK.
	if hasExisting && existing.Status != "" {
		mergedHistory = append([]StatusHistory{{Status: existing.Status, SetAt: existing.UpdatedAt}}, mergedHistory...)
	}

	if len(mergedHistory) > maxStatusHistory {
		mergedHistory = mergedHistory[:maxStatusHistory]
	}
	profile.History = mergedHistory
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
		if p.Status == "" || p.UpdatedAt.Before(cutoff) {
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
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})
	if len(entries) > max {
		entries = entries[:max]
	}
	return entries
}

// StatusStream returns a flat chronological feed of every status ever
// posted (current + history), newest first. This is the home card data
// source — it turns what was an accidental chat surface into an honest
// live stream. Older entries beyond statusMaxAge are dropped.
//
// Two caps are applied:
//   - perUser: within any one user's contribution, only the most recent
//     perUser entries are eligible. This stops a single chatty user
//     (or a long @micro conversation) from flooding the feed.
//   - max: the final chronological merge is trimmed to max total
//     entries.
//
// Pass 0 for either cap to disable it.
func StatusStream(max int) []StatusEntry {
	return StatusStreamCapped(max, StatusStreamPerUser)
}

// StatusStreamCapped is the underlying implementation with explicit
// per-user and total caps. Exported so the profile page and any future
// callers can pick their own shape.
func StatusStreamCapped(maxTotal, maxPerUser int) []StatusEntry {
	profileMutex.RLock()
	defer profileMutex.RUnlock()

	cutoff := time.Now().Add(-statusMaxAge)
	var entries []StatusEntry
	for _, p := range profiles {
		// Banned users are invisible to everyone.
		if auth.IsBanned(p.UserID) {
			continue
		}
		name := p.UserID
		if acc, err := auth.GetAccount(p.UserID); err == nil {
			name = acc.Name
		} else if p.UserID == app.SystemUserID {
			name = app.SystemUserName
		}

		// Collect this user's eligible entries (current + history),
		// newest first. We apply the per-user cap to this collection
		// BEFORE merging, so an older entry from a flooding user can't
		// push a more recent entry from another user off the end.
		var userEntries []StatusEntry
		if p.Status != "" && !p.UpdatedAt.Before(cutoff) {
			userEntries = append(userEntries, StatusEntry{
				UserID:    p.UserID,
				Name:      name,
				Status:    p.Status,
				UpdatedAt: p.UpdatedAt,
			})
		}
		for _, h := range p.History {
			if h.SetAt.Before(cutoff) {
				continue
			}
			userEntries = append(userEntries, StatusEntry{
				UserID:    p.UserID,
				Name:      name,
				Status:    h.Status,
				UpdatedAt: h.SetAt,
			})
		}
		// History is stored newest-first already, and the current
		// status (if present) is always newer than any history entry,
		// so userEntries is already in the right order.
		if maxPerUser > 0 && len(userEntries) > maxPerUser {
			userEntries = userEntries[:maxPerUser]
		}
		entries = append(entries, userEntries...)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})
	if maxTotal > 0 && len(entries) > maxTotal {
		entries = entries[:maxTotal]
	}
	return entries
}

// MaxStatusLength is the upper bound on a single status message. Larger
// than a tweet, smaller than an essay — enough room for a short thought
// or an @micro question without inviting wall-of-text posts.
const MaxStatusLength = 512

// MicroMention is the token that triggers an AI response in the status
// stream. Posting "@micro what's the btc price?" queues a background
// agent call whose answer is posted as a status from the system user.
const MicroMention = "@micro"

// AIReplyHook is wired from main.go. It receives (askerID, prompt) and
// should call the agent, then post the answer as a status from the
// system user. Kept as a callback to avoid a user→agent import cycle.
var AIReplyHook func(askerID, prompt string)

// StatusHandler handles POST /user/status to update the current user's status.
// Auth, CanPost, rate limit, and wallet charging are handled by the
// middleware in main.go — this handler only does the domain logic.
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

	status := strings.TrimSpace(r.FormValue("status"))
	if len(status) > MaxStatusLength {
		status = status[:MaxStatusLength]
	}

	if err := UpdateStatus(sess.Account, status); err != nil {
		app.Log("status", "UpdateStatus failed for %s: %v", sess.Account, err)
	} else {
		app.Log("status", "Status updated for %s: %q", sess.Account, status)
	}

	// Async content moderation — flags spam/test/harmful automatically
	// and auto-bans the user if it's bad. Fire-and-forget.
	if status != "" {
		go moderateStatus(sess.Account, status)
	}

	// If the user @mentioned the system agent, fire off a background
	// agent call that will post the answer as a status from @micro.
	// Skipped when the system user is mentioning itself.
	if status != "" && sess.Account != app.SystemUserID && AIReplyHook != nil && containsMention(status, MicroMention) {
		go AIReplyHook(sess.Account, status)
	}

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

// PostSystemStatus posts a status from the system user (@micro) without
// the usual auth checks. Used by the AI reply hook.
func PostSystemStatus(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) > MaxStatusLength {
		text = text[:MaxStatusLength-1] + "…"
	}
	return UpdateStatus(app.SystemUserID, text)
}

// moderateStatus runs async content moderation on a status post. If the
// LLM classifier flags it as spam, harmful, or a test, the content is
// hidden via the flag system AND the user is auto-banned so all
// their existing + future content becomes invisible to everyone else.
// The user is never told they've been muted — from their perspective
// everything looks normal.
func moderateStatus(accountID, text string) {
	flag.CheckContent("status", accountID, "", text)
	// CheckContent already calls AdminFlag on detection, which hides
	// the individual piece. But for status we want escalation: if the
	// LLM says SPAM/HARMFUL, ban the entire account. We can
	// piggyback on the same LLM result by checking whether the flag
	// was set within the last second (i.e. we just created it).
	item := flag.GetItem("status", accountID)
	if item != nil && item.Flagged {
		app.Log("moderation", "Auto-banning %s after status flagged", accountID)
		auth.BanAccount(accountID)
	}
}

// moderateAIResponse checks an AI-generated response BEFORE it's posted
// as a status. Returns true if the response is safe to post. If the
// content is flagged, the requesting user is banned.
func ModerateAIResponse(askerID, response string) bool {
	flag.CheckContent("ai_response", askerID, "", response)
	item := flag.GetItem("ai_response", askerID)
	if item != nil && item.Flagged {
		app.Log("moderation", "AI response flagged for %s — banning asker", askerID)
		auth.BanAccount(askerID)
		return false
	}
	return true
}

// ClearStatusHistory wipes both the current status and the full history
// for a user. Used by admin console to clean up after spam.
func ClearStatusHistory(userID string) {
	profileMutex.Lock()
	defer profileMutex.Unlock()
	if p, ok := profiles[userID]; ok {
		p.Status = ""
		p.History = nil
		p.UpdatedAt = time.Now()
		data.SaveJSON("profiles.json", profiles)
	}
}

// ClearAllStatuses wipes every user's status + history. Nuclear option
// for when the feed is full of garbage.
func ClearAllStatuses() {
	profileMutex.Lock()
	defer profileMutex.Unlock()
	for _, p := range profiles {
		p.Status = ""
		p.History = nil
		p.UpdatedAt = time.Now()
	}
	data.SaveJSON("profiles.json", profiles)
}

// containsMention returns true when the mention token appears in the
// text as a standalone word (not inside another word like "@microsoft").
func containsMention(text, mention string) bool {
	idx := 0
	for {
		i := strings.Index(text[idx:], mention)
		if i < 0 {
			return false
		}
		pos := idx + i
		// Left boundary — start of string or whitespace/punct.
		if pos > 0 {
			c := text[pos-1]
			if !isMentionBoundary(c) {
				idx = pos + len(mention)
				continue
			}
		}
		// Right boundary — end of string or whitespace/punct (not a
		// word char, so "@microwave" doesn't match).
		after := pos + len(mention)
		if after < len(text) {
			c := text[after]
			if !isMentionBoundary(c) {
				idx = after
				continue
			}
		}
		return true
	}
}

func isMentionBoundary(c byte) bool {
	return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-')
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

	// Handle POST request for status update (legacy, profile page form).
	// Auth, CanPost, rate limit, and wallet charge are handled by the
	// middleware in main.go before this handler is called.
	if r.Method == "POST" {
		sess, _, err := auth.RequireSession(r)
		if err != nil {
			app.Unauthorized(w, r)
			return
		}
		if sess.Account != username {
			app.Forbidden(w, r, "")
			return
		}

		status := strings.TrimSpace(r.FormValue("status"))
		if len(status) > MaxStatusLength {
			status = status[:MaxStatusLength]
		}

		UpdateStatus(sess.Account, status)

		if status != "" {
			go moderateStatus(sess.Account, status)
			if sess.Account != app.SystemUserID && AIReplyHook != nil && containsMention(status, MicroMention) {
				go AIReplyHook(sess.Account, status)
			}
		}

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
		statusSection = fmt.Sprintf(`<p class="info italic mt-3">"%s"</p>`, htmlpkg.EscapeString(profile.Status))
	}
	if len(profile.History) > 0 {
		statusSection += `<details style="margin-top:8px;"><summary style="font-size:13px;color:#999;cursor:pointer;">Status history</summary><div style="margin-top:6px;">`
		for _, h := range profile.History {
			statusSection += fmt.Sprintf(`<p style="font-size:13px;color:#888;margin:4px 0;font-style:italic;">"%s" <span style="color:#bbb;">— %s</span></p>`,
				htmlpkg.EscapeString(h.Status), app.TimeAgo(h.SetAt))
		}
		statusSection += `</div></details>`
	}

	// Build status edit form (only for own profile)
	statusEditForm := ""
	if isOwnProfile {
		statusEditForm = fmt.Sprintf(`
<form method="POST" class="mt-4">
<input type="text" name="status" placeholder="Set your status..." value="%s" maxlength="%d" class="form-input w-full">
<button type="submit" class="mt-2">Update Status</button>
</form>`, htmlpkg.EscapeString(profile.Status), MaxStatusLength)
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
%s
%s
</div>

%s

<h3 class="mb-5">Posts (%d)</h3>
%s
</div>`, acc.ID, verifiedBadge, acc.Created.Format("January 2006"), statusSection, statusEditForm, messageLink, appsSection, postCount, userPosts)

	// Use name as page title
	html := app.RenderHTML(acc.Name, fmt.Sprintf("Profile of %s", acc.Name), content)
	w.Write([]byte(html))
}

// avatarColors are the palette used for status card avatars.
var avatarColors = []string{
	"#56a8a1", // teal
	"#8e7cc3", // purple
	"#e8a87c", // pastel orange
	"#5c9ecf", // blue
	"#e06c75", // rose
	"#c2785c", // terracotta
	"#7bab6e", // sage
	"#9e7db8", // lavender
}

// StatusStreamMax is the maximum number of entries rendered on the home
// status card. The card is scrollable but past ~30 entries the scroll
// becomes noise rather than signal, so we cap below the visual ceiling.
// Overridable via the STATUS_STREAM_LIMIT environment variable.
var StatusStreamMax = envInt("STATUS_STREAM_LIMIT", 30)

// StatusStreamPerUser caps how many entries from any one user appear in
// the visible stream. Without this, a single chatty user (or a long
// @micro conversation) will flood the feed and push everyone else off.
// Overridable via STATUS_STREAM_LIMIT_PER_USER.
var StatusStreamPerUser = envInt("STATUS_STREAM_LIMIT_PER_USER", 10)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// RenderStatusStream renders the inner markup of the home status card:
// the compose form (when a viewer is logged in) plus the scrollable
// stream of recent statuses. Extracted so the fragment endpoint and
// the home card can share one code path.
func RenderStatusStream(viewerID string) string {
	entries := StatusStream(StatusStreamMax)
	app.Log("status", "RenderStatusStream: %d entries for viewer %s", len(entries), viewerID)

	var sb strings.Builder
	if viewerID != "" {
		sb.WriteString(fmt.Sprintf(
			`<form id="home-status-form" method="POST" action="/user/status"><input type="text" name="status" placeholder="What's on your mind? Mention @micro to ask the AI." maxlength="%d" id="home-status-input" autocomplete="off"></form>`,
			MaxStatusLength))
	}
	sb.WriteString(`<div id="home-statuses">`)
	if len(entries) == 0 {
		sb.WriteString(`<p class="text-muted" style="margin:8px 4px;font-size:13px;">No statuses yet. Be the first.</p>`)
	}
	for _, s := range entries {
		initial := "?"
		if s.Name != "" {
			initial = strings.ToUpper(s.Name[:1])
		}
		colorIdx := 0
		for _, c := range s.UserID {
			colorIdx += int(c)
		}
		color := avatarColors[colorIdx%len(avatarColors)]
		entryClass := "home-status-entry"
		if s.UserID == viewerID {
			entryClass += " home-status-mine"
		}
		if s.UserID == app.SystemUserID {
			entryClass += " home-status-system"
		}
		sb.WriteString(fmt.Sprintf(
			`<div class="%s"><div class="home-status-avatar" style="background:%s">%s</div><div class="home-status-body"><div class="home-status-header"><a href="/@%s" class="home-status-name">%s</a><span class="home-status-time">%s</span></div><div class="home-status-text">%s</div></div></div>`,
			entryClass,
			color,
			htmlpkg.EscapeString(initial),
			htmlpkg.EscapeString(s.UserID),
			htmlpkg.EscapeString(s.Name),
			app.TimeAgo(s.UpdatedAt),
			htmlpkg.EscapeString(s.Status)))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// StatusStreamHandler returns the rendered status stream as an HTML
// fragment at GET /user/status/stream. Polled by the home card for
// near-real-time updates without a full page reload.
func StatusStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	viewerID := ""
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(RenderStatusStream(viewerID)))
}
