package user

import (
"encoding/json"
"fmt"
"net/http"
"strings"
"sync"
"time"

"mu/app"
"mu/auth"
"mu/blog"
"mu/data"
)

var profileMutex sync.RWMutex
var profiles = map[string]*UserProfile{}

// UserProfile stores additional user information beyond the Account
type UserProfile struct {
UserID    string    `json:"user_id"`
Status    string    `json:"status"`     // User's custom status message
UpdatedAt time.Time `json:"updated_at"` // When the profile was last updated
}

func init() {
b, _ := data.LoadFile("profiles.json")
json.Unmarshal(b, &profiles)
}

// GetProfile retrieves a user's profile, creating a default one if it doesn't exist
func GetProfile(userID string) *UserProfile {
profileMutex.RLock()
profile, exists := profiles[userID]
profileMutex.RUnlock()

if !exists {
profile = &UserProfile{
UserID:    userID,
Status:    "",
UpdatedAt: time.Now(),
}
}

return profile
}

// UpdateProfile saves a user's profile
func UpdateProfile(profile *UserProfile) error {
profileMutex.Lock()
defer profileMutex.Unlock()

profile.UpdatedAt = time.Now()
profiles[profile.UserID] = profile
data.SaveJSON("profiles.json", profiles)

return nil
}

// Profile handler renders a user profile page at /@username
func Profile(w http.ResponseWriter, r *http.Request) {
// Extract username from URL path (remove /@ prefix)
username := strings.TrimPrefix(r.URL.Path, "/@")
username = strings.TrimSuffix(username, "/")

if username == "" {
http.Redirect(w, r, "/home", 302)
return
}

	// Handle POST request for status update
	if r.Method == "POST" {
		sess, err := auth.GetSession(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Only allow updating own status
		if sess.Account != username {
			http.Error(w, "Forbidden", http.StatusForbidden)
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
posts := blog.GetPostsByAuthor(acc.Name)
postCount := len(posts)

for _, post := range posts {
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

userPosts += fmt.Sprintf(`<div class="post-item" style="margin-bottom: 30px; padding-bottom: 20px; border-bottom: 1px solid #eee;">
			<h3><a href="/post?id=%s">%s</a></h3>
			<div style="margin-bottom: 10px;">%s</div>
			<div class="info">%s · <a href="/post?id=%s">Read more</a></div>
		</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), post.ID)
}

if userPosts == "" {
		userPosts = "<p class='info'>No posts yet.</p>"

// Get user profile
profile := GetProfile(acc.ID)

// Check if viewing own profile
sess, _ := auth.GetSession(r)
isOwnProfile := sess != nil && sess.Account == username

// Build status section
statusSection := ""
if profile.Status != "" {
statusSection = fmt.Sprintf(`<p style="color: #666; margin: 10px 0 0 0; font-style: italic;">"%s"</p>`, profile.Status)
}

// Build status edit form (only for own profile)
statusEditForm := ""
if isOwnProfile {
statusEditForm = fmt.Sprintf(`
		<form method="POST" style="margin-top: 15px;">

// Build message link (only show if not own profile)
messageLink := ""
if !isOwnProfile {
messageLink = fmt.Sprintf(`<p style="margin: 15px 0 0 0;"><a href="/mail?compose=true&to=%s" style="color: #666;">Send a message</a></p>`, acc.ID)
}

// Build the profile page content
content := fmt.Sprintf(`<div style="max-width: 750px;">
<div style="margin-bottom: 30px; padding-bottom: 20px; border-bottom: 2px solid #333;">
<p style="color: #666; margin: 0;">@%s</p>
<p style="color: #666; margin: 10px 0 0 0;">Joined %s</p>
%s
%s
%s
</div>

<h3 style="margin-bottom: 20px;">Posts (%d)</h3>
%s
</div>`, acc.ID, acc.Created.Format("January 2006"), statusSection, statusEditForm, messageLink, postCount, userPosts)

// Use name as page title
html := app.RenderHTML(acc.Name, fmt.Sprintf("Profile of %s", acc.Name), content)
w.Write([]byte(html))
}
