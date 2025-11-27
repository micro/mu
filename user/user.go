package user

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
	"mu/blog"
)

// Profile handler renders a user profile page at /@username
func Profile(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path (remove /@ prefix)
	username := strings.TrimPrefix(r.URL.Path, "/@")
	username = strings.TrimSuffix(username, "/")
	
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

	// Get all posts by this user
	var userPosts string
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
			<h3><a href="/post?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
			<p style="white-space: pre-wrap; color: #333;">%s</p>
			<div class="info" style="color: #666; font-size: small;">%s · <a href="/post?id=%s" style="color: #666;">Read more</a></div>
		</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), post.ID)
	}
	
	if userPosts == "" {
		userPosts = "<p style='color: #666;'>No posts yet.</p>"
	}

	// Build the profile page content
	content := fmt.Sprintf(`<div style="max-width: 750px;">
		<div style="margin-bottom: 30px; padding-bottom: 20px; border-bottom: 2px solid #333;">
			<h2 style="margin: 0 0 10px 0;">%s</h2>
			<p style="color: #666; margin: 0;">@%s</p>
			<p style="color: #666; margin: 10px 0 0 0;">Member since %s</p>
		</div>
		
		<h3 style="margin-bottom: 20px;">Posts (%d)</h3>
		%s
	</div>`, acc.Name, acc.ID, acc.Created.Format("January 2006"), postCount, userPosts)

	// Render with name in browser title but empty page title to avoid duplicate
	html := app.RenderHTMLWithLang("", fmt.Sprintf("%s (@%s)", acc.Name, acc.ID), content, "en")
	// Fix the title tag to show the name
	html = strings.Replace(html, "<title> | Mu</title>", fmt.Sprintf("<title>%s | Mu</title>", acc.Name), 1)
	w.Write([]byte(html))
}
