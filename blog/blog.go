package blog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/admin"
	"mu/app"
	"mu/auth"
	"mu/data"
)

var mutex sync.RWMutex

// cached blog posts
var posts []*Post

// cached HTML for home page preview
var postsPreviewHtml string

// cached HTML for full blog page
var postsList string

type Post struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Load blog posts from disk
func Load() {
	b, err := data.LoadFile("blog.json")
	if err != nil {
		posts = []*Post{}
		return
	}

	if err := json.Unmarshal(b, &posts); err != nil {
		posts = []*Post{}
		return
	}

	// Sort posts by creation time (newest first)
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].CreatedAt.After(posts[j].CreatedAt)
	})

	// Update cached HTML
	updateCache()

	// Register with admin system
	admin.RegisterDeleter("post", &postDeleter{})
}

// postDeleter implements admin.ContentDeleter interface
type postDeleter struct{}

func (d *postDeleter) Delete(id string) error {
	return DeletePost(id)
}

func (d *postDeleter) Get(id string) interface{} {
	post := GetPost(id)
	if post == nil {
		return nil
	}
	return admin.PostContent{
		Title:     post.Title,
		Content:   post.Content,
		Author:    post.Author,
		CreatedAt: post.CreatedAt,
	}
}

func (d *postDeleter) RefreshCache() {
	updateCache()
}

// Save blog posts to disk
func save() error {
	return data.SaveJSON("blog.json", posts)
}

// Update cached HTML
func updateCache() {
	mutex.Lock()
	defer mutex.Unlock()
	updateCacheUnlocked()
}

// updateCacheUnlocked updates the cache without locking (caller must hold lock)
func updateCacheUnlocked() {
	// Generate preview for home page (latest 1 post, exclude flagged)
	var preview []string
	count := 0
	for i := 0; i < len(posts) && count < 1; i++ {
		post := posts[i]
		// Skip flagged posts
		if admin.IsHidden("post", post.ID) {
			continue
		}
		count++

		content := post.Content
		if len(content) > 500 {
			// Find the last space before 500 chars
			lastSpace := 500
			for j := 499; j >= 0; j-- {
				if content[j] == ' ' {
					lastSpace = j
					break
				}
			}
			content = content[:lastSpace] + "..."
		}

		linkedContent := Linkify(content)

		title := post.Title
		if title == "" {
			title = "Untitled"
		}

	authorLink := post.Author
	if post.AuthorID != "" {
		authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
	}

	item := fmt.Sprintf(`<div class="post-item">
		<h3><a href="/post?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
		<p style="white-space: pre-wrap;">%s</p>
		<div class="info" style="color: #666; font-size: small;">%s by %s · <a href="/post?id=%s" style="color: #666;">Link</a></div>
	</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), authorLink, post.ID)
	preview = append(preview, item)
	}

	if len(preview) == 0 {
		postsPreviewHtml = "<p>No posts yet. Be the first to share a thought!</p>"
	} else {
		postsPreviewHtml = strings.Join(preview, "\n")
	}

	// Generate full list for blog page (exclude flagged posts)
	var fullList []string
	for _, post := range posts {
		// Skip flagged posts
		if admin.IsHidden("post", post.ID) {
			continue
		}

		title := post.Title
		if title == "" {
			title = "Untitled"
		}

		linkedContent := Linkify(post.Content)

		authorLink := post.Author
		if post.AuthorID != "" {
			authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
		}

		item := fmt.Sprintf(`<div class="post-item">
			<h3><a href="/post?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
			<p style="white-space: pre-wrap;">%s</p>
			<div class="info" style="color: #666; font-size: small;">%s by %s · <a href="/post?id=%s" style="color: #666;">Link</a> · <a href="/chat?id=post_%s" style="color: #666;">💬 Discuss</a> · <a href="#" onclick="flagPost('%s'); return false;" style="color: #666;">Flag</a></div>
		</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), authorLink, post.ID, post.ID, post.ID)
		fullList = append(fullList, item)
	}

	if len(fullList) == 0 {
		postsList = "<p>No posts yet. Write something below!</p>"
	} else {
		postsList = strings.Join(fullList, "\n<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>\n")
	}
}

// Preview returns HTML preview of latest posts for home page
func Preview() string {
	updateCache()
	mutex.RLock()
	defer mutex.RUnlock()
	return postsPreviewHtml
}

// FullFeed returns HTML for all posts (for home page feed)
func FullFeed() string {
	updateCache()
	mutex.RLock()
	defer mutex.RUnlock()
	return postsList
}

// HomeFeed returns HTML for limited posts (latest 1 for home page)
func HomeFeed() string {
	mutex.RLock()
	defer mutex.RUnlock()

	if len(posts) == 0 {
		return "<p>No posts yet. Be the first to share a thought!</p>"
	}

	// Show only the latest post
	post := posts[0]
	return renderPostPreview(post)
}

func renderPostPreview(post *Post) string {
	title := post.Title
	if title == "" {
		title = "Untitled"
	}

	// Truncate content to first paragraph (first double newline or max 300 chars)
	content := post.Content
	
	// Look for double newline (paragraph break)
	if idx := strings.Index(content, "\n\n"); idx > 0 && idx < 300 {
		content = content[:idx] + "..."
	} else if idx := strings.Index(content, "\r\n\r\n"); idx > 0 && idx < 300 {
		content = content[:idx] + "..."
	} else if len(content) > 300 {
		// No paragraph break found, truncate at word boundary
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

	linkedContent := Linkify(content)

	authorLink := post.Author
	if post.AuthorID != "" {
		authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
	}

	item := fmt.Sprintf(`<div class="post-item">
		<h3><a href="/post?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
		<p style="white-space: pre-wrap;">%s</p>
		<div class="info" style="color: #666; font-size: small;">
			%s by %s
			<span style="margin-left: 10px;">·</span>
			<a href="/chat?id=post_%s" style="color: #0066cc; margin-left: 10px;">💬 Discuss</a>
		</div>
	</div>`, post.ID, title, linkedContent, app.TimeAgo(post.CreatedAt), authorLink, post.ID)

	return item
}

// PostingForm returns the HTML for the posting form
func PostingForm(action string) string {
	return fmt.Sprintf(`<div id="post-form-container">
		<form id="post-form" method="POST" action="%s">
			<input type="text" name="title" placeholder="Title (optional)">
			<textarea name="content" rows="4" placeholder="Share a thought. Be mindful of Allah" required></textarea>
			<button type="submit">Post</button>
		</form>
	</div>`, action)
}

// Handler serves the blog page
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		handlePost(w, r)
		return
	}

	mutex.RLock()
	list := postsList
	mutex.RUnlock()

	// Create the blog page with posting form
	content := fmt.Sprintf(`<div id="blog">
		<div style="margin-bottom: 30px;">
			<form id="blog-form" method="POST" action="/posts" style="display: flex; flex-direction: column; gap: 10px;">
				<input type="text" name="title" placeholder="Title (optional)" style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
				<textarea name="content" rows="6" placeholder="Share a thought. Be mindful of Allah" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical;"></textarea>
				<button type="submit" style="padding: 10px 20px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer; align-self: flex-start;">Post</button>
			</form>
		</div>
		<div style="margin-bottom: 15px;">
			<a href="/moderate" style="color: #666; text-decoration: none; font-size: 14px;">Moderate</a>
		</div>
		<hr style='margin: 0 0 30px 0; border: none; border-top: 2px solid #333;'>
		<div id="posts-list">
			%s
		</div>
	</div>`, list)

	html := app.RenderHTMLForRequest("Posts", "Share your thoughts", content, r)
	w.Write([]byte(html))
}

// CreatePost creates a new post and returns error if any
func CreatePost(title, content, author, authorID string) error {
	// Create new post
	post := &Post{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		Content:   content,
		Author:    author,
		AuthorID:  authorID,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	// Add to beginning of slice (newest first)
	posts = append([]*Post{post}, posts...)
	mutex.Unlock()

	// Save to disk
	if err := save(); err != nil {
		return err
	}

	// Update cached HTML
	updateCache()

	return nil
}

// GetPost retrieves a post by ID
func GetPost(id string) *Post {
	mutex.RLock()
	defer mutex.RUnlock()

	for _, post := range posts {
		if post.ID == id {
			return post
		}
	}
	return nil
}

// DeletePost removes a post by ID
func DeletePost(id string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for i, post := range posts {
		if post.ID == id {
			posts = append(posts[:i], posts[i+1:]...)
			save()
			updateCacheUnlocked()
			return nil
		}
	}
	return fmt.Errorf("post not found")
}

// RefreshCache updates the cached HTML
func RefreshCache() {
	updateCache()
}

// GetPostsByAuthor returns all posts by a specific author (for user profiles)
func GetPostsByAuthor(authorName string) []*Post {
	mutex.RLock()
	defer mutex.RUnlock()
	
	var userPosts []*Post
	for _, post := range posts {
		if post.Author == authorName {
			userPosts = append(userPosts, post)
		}
	}
	return userPosts
}

// handlePost processes the POST request to create a new blog post
// PostHandler serves individual blog posts (public, no auth required)
func PostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Redirect(w, r, "/posts", 302)
		return
	}

	post := GetPost(id)
	if post == nil {
		http.Error(w, "Post not found", 404)
		return
	}

	title := post.Title
	if title == "" {
		title = "Untitled"
	}

	linkedContent := Linkify(post.Content)

	authorLink := post.Author
	if post.AuthorID != "" {
		authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
	}

	content := fmt.Sprintf(`<div id="blog">
	<div class="info" style="color: #666; font-size: small;">%s by %s</div>
		<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>
		<div style="white-space: pre-wrap;">%s</div>
		<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>
		<a href="/posts">← Back to all posts</a>
	</div>`, app.TimeAgo(post.CreatedAt), authorLink, linkedContent)

	// Check if user is authenticated to show logout link
	var token string
	if c, err := r.Cookie("session"); err == nil && c != nil {
		token = c.Value
	}
	showLogout := auth.ValidateToken(token) == nil

	html := app.RenderHTMLWithLogout(title, post.Content[:min(len(post.Content), 150)], content, showLogout)
	w.Write([]byte(html))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// linkify converts URLs in text to clickable links
// Linkify converts URLs in text to clickable links and embeds YouTube videos
func Linkify(text string) string {
	// First, handle YouTube embeds (both youtube.com and youtu.be)
	youtubePattern := regexp.MustCompile(`https?://(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})(?:[^\s<>"]*)?`)
	text = youtubePattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract video ID
		matches := youtubePattern.FindStringSubmatch(match)
		if len(matches) > 1 {
			videoID := matches[1]
			return fmt.Sprintf(`<div style="position: relative; padding-bottom: 56.25%%; height: 0; overflow: hidden; max-width: 100%%; margin: 15px 0;"><iframe src="/video?id=%s" style="position: absolute; top: 0; left: 0; width: 100%%; height: 100%%; border: 0;" allowfullscreen loading="lazy"></iframe></div>`, videoID)
		}
		return match
	})
	
	// Then linkify remaining URLs
	urlPattern := regexp.MustCompile(`(https?://[^\s<>"]+)`)
	text = urlPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Skip if already part of an iframe (already embedded)
		if strings.Contains(match, "/video?id=") {
			return match
		}
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, match, match)
	})
	
	return text
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))

	if content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	// Get the authenticated user
	author := "Anonymous"
	authorID := ""
	sess, err := auth.GetSession(r)
	if err == nil {
		acc, err := auth.GetAccount(sess.Account)
		if err == nil {
			author = acc.Name
			authorID = acc.ID
		}
	}

	// Create the post
	if err := CreatePost(title, content, author, authorID); err != nil {
		http.Error(w, "Failed to save post", http.StatusInternalServerError)
		return
	}

	// Redirect back to posts page
	http.Redirect(w, r, "/posts", http.StatusSeeOther)
}
