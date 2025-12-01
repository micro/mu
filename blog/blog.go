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
	Content   string    `json:"content"` // Raw markdown content
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

		// Use pre-rendered HTML, truncate for preview
		content := post.Content

		// Truncate plain text before rendering
		if len(content) > 300 {
			lastSpace := 300
			for i := 299; i >= 0 && i < len(content); i-- {
				if content[i] == ' ' {
					lastSpace = i
					break
				}
			}
			content = content[:lastSpace] + "..."
		}

		// Add links and YouTube embeds
		content = Linkify(content)

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
		<div style="margin-bottom: 10px;">%s</div>
		<div class="info" style="color: #666; font-size: small;">%s by %s · <a href="/post?id=%s" style="color: #666;">Link</a> · <a href="/chat?id=post_%s" style="color: #666;">Discuss</a></div>
	</div>`, post.ID, title, content, app.TimeAgo(post.CreatedAt), authorLink, post.ID, post.ID)
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

		// Use pre-rendered HTML, truncate for list view
		content := post.Content

		// Truncate plain text before rendering
		if len(content) > 500 {
			lastSpace := 500
			for i := 499; i >= 0 && i < len(content); i-- {
				if content[i] == ' ' {
					lastSpace = i
					break
				}
			}
			content = content[:lastSpace] + "..."
		}

		// Add links and YouTube embeds
		content = Linkify(content)

		authorLink := post.Author
		if post.AuthorID != "" {
			authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
		}

		item := fmt.Sprintf(`<div class="post-item">
			<h3><a href="/post?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
			<div style="margin-bottom: 10px;">%s</div>
			<div class="info" style="color: #666; font-size: small;">%s by %s · <a href="/post?id=%s" style="color: #666;">Link</a> · <a href="/chat?id=post_%s" style="color: #666;">Discuss</a> · <a href="#" onclick="flagPost('%s'); return false;" style="color: #666;">Flag</a></div>
		</div>`, post.ID, title, content, app.TimeAgo(post.CreatedAt), authorLink, post.ID, post.ID, post.ID)
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

	// Use pre-rendered HTML and truncate for preview
	content := post.Content

	// Truncate plain text before rendering
	if len(content) > 256 {
		lastSpace := 256
		for i := 255; i >= 0 && i < len(content); i-- {
			if content[i] == ' ' {
				lastSpace = i
				break
			}
		}
		content = content[:lastSpace] + "..."
	}

	authorLink := post.Author
	if post.AuthorID != "" {
		authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
	}

	item := fmt.Sprintf(`<div class="post-item">
		<h3><a href="/post?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
		<div style="margin-bottom: 10px;">%s</div>
		<div class="info" style="color: #666; font-size: small;">
			%s by %s
			<span style="margin-left: 10px;">·</span>
			<a href="/chat?id=post_%s" style="color: #0066cc; margin-left: 10px;">💬 Discuss</a>
		</div>
	</div>`, post.ID, title, content, app.TimeAgo(post.CreatedAt), authorLink, post.ID)

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

	// Return JSON if requested
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		mutex.RLock()
		// Filter out flagged posts
		var visiblePosts []*Post
		for _, post := range posts {
			if !admin.IsHidden("post", post.ID) {
				visiblePosts = append(visiblePosts, post)
			}
		}
		mutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(visiblePosts)
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
				<textarea id="post-content" name="content" rows="6" placeholder="Share a thought. Be mindful of Allah" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical;"></textarea>
				<div style="display: flex; justify-content: space-between; align-items: center;">
					<span id="char-count" style="font-size: 12px; color: #666;">Minimum 50 characters</span>
					<button type="submit" style="padding: 10px 20px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Post</button>
				</div>
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

// UpdatePost updates an existing post
func UpdatePost(id, title, content string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for i, post := range posts {
		if post.ID == id {
			posts[i].Title = title
			posts[i].Content = content
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
// PostHandler serves individual blog posts (public, no auth required) and handles PATCH for editing
// Supports both HTML and JSON requests
func PostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// Handle POST to create new post (no id required)
	if r.Method == "POST" && id == "" {
		isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

		var title, content string

		if isJSON {
			var req struct {
				Title   string `json:"title"`
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(req.Title)
			content = strings.TrimSpace(req.Content)
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Failed to parse form", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(r.FormValue("title"))
			content = strings.TrimSpace(r.FormValue("content"))
		}

		// Validate content
		if content == "" {
			http.Error(w, "Content is required", http.StatusBadRequest)
			return
		}

		if len(content) < 50 {
			hasURL := strings.Contains(content, "http://") || strings.Contains(content, "https://")
			if !hasURL {
				http.Error(w, "Post content must be at least 50 characters", http.StatusBadRequest)
				return
			}
		}

		// Get authenticated user
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

		// Create post
		postID := fmt.Sprintf("%d", time.Now().UnixNano())
		if err := CreatePost(title, content, author, authorID); err != nil {
			http.Error(w, "Failed to save post", http.StatusInternalServerError)
			return
		}

		// Run async LLM-based content moderation
		go admin.CheckContent("post", postID, title, content)

		if isJSON {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"id":      postID,
			})
			return
		}

		http.Redirect(w, r, "/posts", http.StatusSeeOther)
		return
	}

	// For other methods, id is required
	if id == "" {
		http.Redirect(w, r, "/posts", 302)
		return
	}

	post := GetPost(id)
	if post == nil {
		http.Error(w, "Post not found", 404)
		return
	}

	// Handle PATCH - update the post
	if r.Method == "PATCH" || (r.Method == "POST" && r.FormValue("_method") == "PATCH") {
		isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

		// Must be authenticated
		sess, err := auth.GetSession(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		acc, err := auth.GetAccount(sess.Account)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if user is the author
		if post.AuthorID != acc.ID {
			http.Error(w, "Forbidden - you can only edit your own posts", http.StatusForbidden)
			return
		}

		var title, content string

		if isJSON {
			var req struct {
				Title   string `json:"title"`
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(req.Title)
			content = strings.TrimSpace(req.Content)
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Failed to parse form", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(r.FormValue("title"))
			content = strings.TrimSpace(r.FormValue("content"))
		}

		if content == "" {
			http.Error(w, "Content is required", http.StatusBadRequest)
			return
		}

		// Same validation as creating a post
		hasURL := strings.Contains(content, "http://") || strings.Contains(content, "https://")
		if !hasURL && len(content) < 50 {
			http.Error(w, "Post content must be at least 50 characters", http.StatusBadRequest)
			return
		}

		if err := UpdatePost(id, title, content); err != nil {
			http.Error(w, "Failed to update post", http.StatusInternalServerError)
			return
		}

		if isJSON {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"id":      id,
			})
			return
		}

		http.Redirect(w, r, "/post?id="+id, http.StatusSeeOther)
		return
	}

	// GET - return JSON if requested
	if r.Method == "GET" && strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
		return
	}

	// Check if edit mode is requested
	if r.URL.Query().Get("edit") == "true" {
		// Must be authenticated
		sess, err := auth.GetSession(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		acc, err := auth.GetAccount(sess.Account)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if user is the author
		if post.AuthorID != acc.ID {
			http.Error(w, "Forbidden - you can only edit your own posts", http.StatusForbidden)
			return
		}

		// Show edit form
		pageTitle := "Edit Post"
		if post.Title != "" {
			pageTitle = "Edit: " + post.Title
		}

		content := fmt.Sprintf(`<div id="blog">
			<form method="POST" action="/post?id=%s" style="display: flex; flex-direction: column; gap: 10px;">
				<input type="hidden" name="_method" value="PATCH">
				<input type="text" name="title" placeholder="Title (optional)" value="%s" style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
				<textarea name="content" rows="15" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical; font-family: monospace;">%s</textarea>
				<div style="font-size: 12px; color: #666; margin-top: -5px;">
					Supports markdown: **bold**, *italic*, `+"`code`"+`, `+"```"+` for code blocks, # headers, - lists
				</div>
				<div style="display: flex; gap: 10px;">
					<button type="submit" style="padding: 10px 20px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Save Changes</button>
					<a href="/post?id=%s" style="padding: 10px 20px; font-size: 14px; background-color: #ccc; color: #333; text-decoration: none; border-radius: 5px; display: inline-block;">Cancel</a>
				</div>
			</form>
		</div>`, post.ID, post.Title, post.Content, post.ID)

		html := app.RenderHTMLForRequest(pageTitle, "", content, r)
		w.Write([]byte(html))
		return
	}

	title := post.Title
	if title == "" {
		title = "Untitled"
	}

	// Add links and YouTube embeds for full post view
	contentHTML := Linkify(post.Content)

	authorLink := post.Author
	if post.AuthorID != "" {
		authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, post.AuthorID, post.Author)
	}

	// Check if current user is the author (to show edit button)
	var editButton string
	sess, err := auth.GetSession(r)
	if err == nil {
		acc, err := auth.GetAccount(sess.Account)
		if err == nil && acc.ID == post.AuthorID {
			editButton = ` · <a href="/post?id=` + post.ID + `&edit=true" style="color: #666;">Edit</a>`
		}
	}

	content := fmt.Sprintf(`<div id="blog">
		<div class="info" style="color: #666; font-size: small;">
			%s by %s · <a href="/chat?id=post_%s" style="color: #666;">Discuss</a>%s · <a href="#" onclick="flagPost('%s'); return false;" style="color: #666;">Flag</a>
		</div>
		<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>
		<div style="margin-bottom: 20px;">%s</div>
		<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>
		<a href="/posts">← Back to all posts</a>
	</div>`, app.TimeAgo(post.CreatedAt), authorLink, post.ID, editButton, post.ID, contentHTML)

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

// EditHandler serves the post edit form
// RenderMarkdown converts markdown to HTML without embeds (for storage/previews)
func RenderMarkdown(text string) string {
	return string(app.Render([]byte(text)))
}

// Linkify converts URLs in text to clickable links and embeds YouTube videos (for full post display)
func Linkify(text string) string {
	// Escape HTML
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// Replace YouTube URLs with embeds first
	youtubePattern := regexp.MustCompile(`https?://(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})(?:\S*)?`)
	text = youtubePattern.ReplaceAllStringFunc(text, func(match string) string {
		matches := youtubePattern.FindStringSubmatch(match)
		if len(matches) > 1 {
			videoID := matches[1]
			return fmt.Sprintf(`<div style="position: relative; padding-bottom: 56.25%%; height: 0; overflow: hidden; max-width: 100%%; margin: 15px 0;"><iframe src="/video?id=%s" style="position: absolute; top: 0; left: 0; width: 100%%; height: 100%%; border: 0;" allowfullscreen loading="lazy"></iframe></div>`, videoID)
		}
		return match
	})

	// Convert other URLs to links
	urlPattern := regexp.MustCompile(`https?://[^\s<]+`)
	text = urlPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Skip if it's already part of an iframe (YouTube embed)
		if strings.Contains(match, "iframe") {
			return match
		}
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, match, match)
	})

	// Convert newlines to <br>
	text = strings.ReplaceAll(text, "\n", "<br>")

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

	// Content validation: minimum length
	if len(content) < 50 {
		http.Error(w, "Post content must be at least 50 characters", http.StatusBadRequest)
		return
	}

	// Spam detection: check for common test patterns
	contentLower := strings.ToLower(content)
	spamPatterns := []string{
		"this is a test",
		"test post",
		"testing",
		"asdf",
		"qwerty",
		"lorem ipsum",
	}

	for _, pattern := range spamPatterns {
		if strings.Contains(contentLower, pattern) && len(content) < 100 {
			http.Error(w, "Post appears to be test content. Please share something meaningful.", http.StatusBadRequest)
			return
		}
	}

	// Advanced spam detection: check for low-quality content
	// Allow URLs to pass through
	hasURL := strings.Contains(content, "http://") || strings.Contains(content, "https://")
	if !hasURL {
		// Count words
		wordCount := len(strings.Fields(content))

		// Require at least 3 words/spaces for non-URL content
		if wordCount < 3 {
			http.Error(w, "Post must contain at least 3 words. Share something meaningful.", http.StatusBadRequest)
			return
		}

		// Check for excessive repeated characters (e.g., "aaaaaa" or "asdfasdfasdf")
		repeatedChars := 0
		lastChar := rune(0)
		for _, char := range content {
			if char == lastChar && char != ' ' && char != '\n' {
				repeatedChars++
				if repeatedChars > 4 {
					http.Error(w, "Post contains too many repeated characters. Please share something meaningful.", http.StatusBadRequest)
					return
				}
			} else {
				repeatedChars = 0
			}
			lastChar = char
		}

		// Check character diversity (should have at least 10 unique characters for 50+ char posts)
		uniqueChars := make(map[rune]bool)
		for _, char := range strings.ToLower(content) {
			if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
				uniqueChars[char] = true
			}
		}
		if len(uniqueChars) < 10 {
			http.Error(w, "Post lacks character diversity. Please share something meaningful.", http.StatusBadRequest)
			return
		}
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
	postID := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := CreatePost(title, content, author, authorID); err != nil {
		http.Error(w, "Failed to save post", http.StatusInternalServerError)
		return
	}

	// Run async LLM-based content moderation (non-blocking)
	go admin.CheckContent("post", postID, title, content)

	// Redirect back to posts page
	http.Redirect(w, r, "/posts", http.StatusSeeOther)
}
