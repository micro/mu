package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/apps"
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

func init() {
	b, _ := data.LoadFile("profiles.json")
	json.Unmarshal(b, &profiles)
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

	// Get the user account
	acc, err := auth.GetAccount(username)
	if err != nil {
		http.Error(w, "User not found", 404)
		return
	}

	// Get all posts by this user
	var userPosts string
	posts := blog.GetPostsByAuthor(acc.Name)
	
	// Check if viewer is a member/admin
	isMember := false
	if sess, err := auth.GetSession(r); err == nil {
		if viewerAcc, err := auth.GetAccount(sess.Account); err == nil {
			isMember = viewerAcc.Member || viewerAcc.Admin
		}
	}
	
	// Filter private posts for non-members
	var visiblePosts []*blog.Post
	for _, post := range posts {
		if !post.Private || isMember {
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

		userPosts += fmt.Sprintf(`<div class="post-item" style="margin-bottom: 30px; padding-bottom: 20px; border-bottom: 1px solid #eee;">
<h3><a href="/post?id=%s">%s</a></h3>
<div style="margin-bottom: 10px;">%s</div>
<div class="info">%s · <a href="/post?id=%s">Read more</a></div>
</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), post.ID)
	}

	if userPosts == "" {
		userPosts = "<p class='info'>No blog posts yet.</p>"
	}

	// Get user profile
	profile := GetProfile(acc.ID)

	// Check if viewing own profile
	sess, _ := auth.GetSession(r)
	isOwnProfile := sess != nil && sess.Account == username

	// Build status section
	statusSection := ""
	if profile.Status != "" {
		statusSection = fmt.Sprintf(`<p class="info" style="font-style: italic; margin: 10px 0 0 0;">"%s"</p>`, profile.Status)
	}

	// Build status edit form (only for own profile)
	statusEditForm := ""
	if isOwnProfile {
		statusEditForm = fmt.Sprintf(`
<form method="POST" style="margin-top: 15px;">
<input type="text" name="status" placeholder="Set your status..." value="%s" maxlength="100" style="width: 100%%; padding: 8px; border: 1px solid #e0e0e0; border-radius: 5px; box-sizing: border-box;">
<button type="submit" style="margin-top: 8px;">Update Status</button>
</form>`, profile.Status)
	}

	// Build message link (only show if not own profile)
	messageLink := ""
	if !isOwnProfile {
		messageLink = fmt.Sprintf(`<p style="margin: 15px 0 0 0;"><a href="/mail?compose=true&to=%s">Send a message</a></p>`, acc.ID)
	}

	// Build apps section
	userApps := apps.GetUserApps(acc.ID)
	var appsSection string
	if len(userApps) > 0 {
		appsSection = fmt.Sprintf(`<h3 style="margin-bottom: 20px;">Apps (%d)</h3><div style="margin-bottom: 30px;">`, len(userApps))
		for _, a := range userApps {
			if !a.Public && !isOwnProfile {
				continue // Skip private apps for non-owners
			}
			appsSection += fmt.Sprintf(`<p><a href="/apps/%s">%s</a></p>`, a.ID, a.Name)
		}
		appsSection += `</div>`
	}

	// Build the profile page content
	content := fmt.Sprintf(`<div style="max-width: 750px;">
<div style="margin-bottom: 30px; padding-bottom: 20px; border-bottom: 2px solid #333;">
<p class="info" style="margin: 0;">@%s</p>
<p class="info" style="margin: 10px 0 0 0;">Joined %s</p>
%s
%s
%s
</div>

%s

<h3 style="margin-bottom: 20px;">Posts (%d)</h3>
%s
</div>`, acc.ID, acc.Created.Format("January 2006"), statusSection, statusEditForm, messageLink, appsSection, postCount, userPosts)

	// Use name as page title
	html := app.RenderHTML(acc.Name, fmt.Sprintf("Profile of %s", acc.Name), content)
	w.Write([]byte(html))
}
