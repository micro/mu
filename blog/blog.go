package blog

import (
	_ "embed"
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

//go:embed topics.json
var topicsJSON []byte

var mutex sync.RWMutex

// cached blog posts
var posts []*Post

// postsMap for O(1) lookups by ID
var postsMap map[string]*Post

// cached comments
var comments []*Comment

// cached HTML for home page preview
var postsPreviewHtml string

// cached HTML for full blog page
var postsList string

// Valid topics/categories for posts
var topics []string

type Post struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"` // Raw markdown content
	Author    string     `json:"author"`
	AuthorID  string     `json:"author_id"`
	Tags      string     `json:"tags"` // Comma-separated tags
	CreatedAt time.Time  `json:"created_at"`
	Comments  []*Comment `json:"-"` // Not persisted, populated on load
}

type Comment struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}

// tagRegex validates tag format: alphanumeric only
var tagRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// GetTopics returns the list of valid topics/categories
func GetTopics() []string {
	return topics
}

// formatTags splits comma-separated tags and formats them as individual badges
func formatTags(tags string) string {
	if tags == "" {
		return ""
	}

	parts := strings.Split(tags, ",")
	var badges []string
	for _, tag := range parts {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			badges = append(badges, fmt.Sprintf(`<span class="category">%s</span>`, tag))
		}
	}

	if len(badges) == 0 {
		return ""
	}
	return strings.Join(badges, " ")
}

// parseTags parses comma-separated tags, validates, and normalizes them
func parseTags(input string) string {
	if input == "" {
		return ""
	}

	// Split by comma
	parts := strings.Split(input, ",")
	var validTags []string
	seen := make(map[string]bool)

	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}

		// Check if it's alphanumeric
		if !tagRegex.MatchString(tag) {
			continue
		}

		// Normalize: capitalize first letter
		tag = strings.ToUpper(tag[:1]) + strings.ToLower(tag[1:])

		// Check for duplicates
		if seen[tag] {
			continue
		}
		seen[tag] = true

		validTags = append(validTags, tag)
	}

	return strings.Join(validTags, ", ")
}

// Load initializes the blog package and sets up event subscriptions
func Load() {
	// Load topics from embedded JSON
	if err := json.Unmarshal(topicsJSON, &topics); err != nil {
		app.Log("blog", "Error loading topics: %v", err)
		topics = []string{"Crypto", "Dev", "Finance", "Islam", "Politics", "Tech", "UK", "World"}
	}

	// Subscribe to tag generation responses
	tagSub := data.Subscribe(data.EventTagGenerated)
	go func() {
		for event := range tagSub.Chan {
			postID, okID := event.Data["post_id"].(string)
			tag, okTag := event.Data["tag"].(string)
			eventType, okType := event.Data["type"].(string)

			if okID && okTag && okType && eventType == "post" {
				app.Log("blog", "Received generated tag for post: %s", postID)

				// Update the post with the tag
				mutex.Lock()
				var post *Post
				for _, p := range posts {
					if p.ID == postID {
						p.Tags = tag
						post = p
						break
					}
				}
				mutex.Unlock()

				if post == nil {
					app.Log("blog", "Post %s not found for tagging", postID)
					continue
				}

				// Save to disk
				if err := save(); err != nil {
					app.Log("blog", "Error saving auto-tag for post %s: %v", postID, err)
					continue
				}

				// Update cached HTML
				updateCache()

				// Re-index with the new tag
				data.Index(
					post.ID,
					"post",
					post.Title,
					post.Content,
					map[string]interface{}{
						"url":    "/post?id=" + post.ID,
						"author": post.Author,
						"tags":   post.Tags,
					},
				)

				app.Log("blog", "Auto-tagged post %s with: %s", postID, tag)
			}
		}
	}()

	// Load existing posts from disk
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

	// Build postsMap for O(1) lookups
	postsMap = make(map[string]*Post)
	for _, post := range posts {
		postsMap[post.ID] = post
	}

	// Load comments
	commentData, err := data.LoadFile("comments.json")
	if err == nil {
		json.Unmarshal(commentData, &comments)
	} else {
		comments = []*Comment{}
	}

	// Sort comments by creation time (oldest first for threading)
	sort.Slice(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})

	// Link comments to posts
	populateComments()

	// Update cached HTML
	updateCache()

	// Index all existing posts for search/RAG
	go func() {
		for _, post := range posts {
			app.Log("blog", "Indexing existing post: %s", post.Title)
			data.Index(
				post.ID,
				"post",
				post.Title,
				post.Content,
				map[string]interface{}{
					"url":    "/post?id=" + post.ID,
					"author": post.Author,
					"tags":   post.Tags,
				},
			)
		}
	}()

	// Register with admin system
	admin.RegisterDeleter("post", &postDeleter{})
	admin.GetNewAccountBlog = getNewAccountBlogForAdmin
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

// getNewAccountBlogForAdmin returns blog posts from new accounts for the moderation page
func getNewAccountBlogForAdmin() []admin.PostContent {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []admin.PostContent
	for _, post := range posts {
		// Skip flagged/hidden posts
		if admin.IsHidden("post", post.ID) {
			continue
		}

		// Only include posts from new accounts
		if post.AuthorID != "" && auth.IsNewAccount(post.AuthorID) {
			result = append(result, admin.PostContent{
				ID:        post.ID,
				Title:     post.Title,
				Content:   post.Content,
				Author:    post.Author,
				CreatedAt: post.CreatedAt,
			})
		}
	}

	return result
}

func (d *postDeleter) RefreshCache() {
	updateCache()
}

// countComments returns the number of comments for a given post
func countComments(post *Post) int {
	if post == nil {
		return 0
	}
	return len(post.Comments)
}

// populateComments links comments to their respective blog posts
func populateComments() {
	// Clear existing comments on blog posts
	for _, post := range posts {
		post.Comments = nil
	}

	// Build a map for quick lookup
	postMap := make(map[string]*Post)
	for _, post := range posts {
		postMap[post.ID] = post
	}

	// Link comments to posts
	for _, comment := range comments {
		if post, ok := postMap[comment.PostID]; ok {
			post.Comments = append(post.Comments, comment)
		}
	}
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

	// Publish event to refresh home page cache
	data.Publish(data.Event{
		Type: "blog_updated",
		Data: map[string]interface{}{},
	})
}

// updateCacheUnlocked updates the cache without locking (caller must hold lock)
func updateCacheUnlocked() {
	// Generate preview for home page (latest 1 post, exclude flagged and new accounts)
	var preview []string
	count := 0
	for i := 0; i < len(posts) && count < 1; i++ {
		post := posts[i]
		// Skip flagged posts
		if admin.IsHidden("post", post.ID) {
			continue
		}
		// Skip posts from new accounts (< 24 hours old)
		if post.AuthorID != "" && auth.IsNewAccount(post.AuthorID) {
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
			authorLink = fmt.Sprintf(`<a href="/@%s">%s</a>`, post.AuthorID, post.Author)
		}

		tagsHtml := formatTags(post.Tags)
		if tagsHtml != "" {
			tagsHtml = `<div style="margin-bottom: 8px;">` + tagsHtml + `</div>`
		}

		// Add Reply/Replies count
		commentCount := countComments(post)
		replyLink := ""
		if commentCount == 0 {
			replyLink = fmt.Sprintf(` · <a href="/post?id=%s">Reply</a>`, post.ID)
		} else {
			replyLink = fmt.Sprintf(` · <a href="/post?id=%s">Replies (%d)</a>`, post.ID, commentCount)
		}

		item := fmt.Sprintf(`<div class="post-item">
		%s
		<h3><a href="/post?id=%s">%s</a></h3>
		<div>%s</div>
		<div class="info"><span data-timestamp="%d">%s</span> · Posted by %s%s</div>
	</div>`, tagsHtml, post.ID, title, content, post.CreatedAt.Unix(), app.TimeAgo(post.CreatedAt), authorLink, replyLink)
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

		// Skip posts from new accounts (< 24 hours old)
		if post.AuthorID != "" && auth.IsNewAccount(post.AuthorID) {
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
			authorLink = fmt.Sprintf(`<a href="/@%s">%s</a>`, post.AuthorID, post.Author)
		}

		tagsHtml := formatTags(post.Tags)
		if tagsHtml != "" {
			tagsHtml = `<div style="margin-bottom: 8px;">` + tagsHtml + `</div>`
		}

		// Add Reply/Replies count
		commentCount := countComments(post)
		replyLink := ""
		if commentCount == 0 {
			replyLink = fmt.Sprintf(` · <a href="/post?id=%s">Reply</a>`, post.ID)
		} else {
			replyLink = fmt.Sprintf(` · <a href="/post?id=%s">Replies (%d)</a>`, post.ID, commentCount)
		}

		item := fmt.Sprintf(`<div class="post-item">
			%s
			<h3><a href="/post?id=%s">%s</a></h3>
			<div>%s</div>
			<div class="info"><span data-timestamp="%d">%s</span> · Posted by %s%s</div>
		</div>`, tagsHtml, post.ID, title, content, post.CreatedAt.Unix(), app.TimeAgo(post.CreatedAt), authorLink, replyLink)
		fullList = append(fullList, item)
	}

	if len(fullList) == 0 {
		postsList = "<p>No blog posts yet. Write something below!</p>"
	} else {
		postsList = strings.Join(fullList, "\n")
	}
}

// Preview returns HTML preview of latest posts for home page
func Preview() string {
	// Use cached HTML for efficiency
	mutex.RLock()
	defer mutex.RUnlock()

	return postsPreviewHtml
}

// Deprecated: Use Preview() instead which now uses cache
func previewUncached() string {
	if len(posts) == 0 {
		return "<p>No posts yet. Be the first to share a thought!</p>"
	}

	// Get latest 1 post (exclude flagged and new accounts)
	var preview []string
	count := 0
	for i := 0; i < len(posts) && count < 1; i++ {
		post := posts[i]
		// Skip flagged posts
		if admin.IsHidden("post", post.ID) {
			continue
		}
		// Skip posts from new accounts (< 24 hours old)
		if post.AuthorID != "" && auth.IsNewAccount(post.AuthorID) {
			continue
		}
		count++

		content := post.Content
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

		content = Linkify(content)

		title := post.Title
		if title == "" {
			title = "Untitled"
		}

		authorLink := post.Author
		if post.AuthorID != "" {
			authorLink = fmt.Sprintf(`<a href="/@%s">%s</a>`, post.AuthorID, post.Author)
		}

		tagsHtml := ""
		if post.Tags != "" {
			for _, tag := range strings.Split(post.Tags, ",") {
				tagsHtml += fmt.Sprintf(` · <span class="category">%s</span>`, strings.TrimSpace(tag))
			}
		}

		// Add Reply/Replies count
		commentCount := countComments(post)
		replyLink := ""
		if commentCount == 0 {
			replyLink = fmt.Sprintf(` · <a href="/post?id=%s">Reply</a>`, post.ID)
		} else {
			replyLink = fmt.Sprintf(` · <a href="/post?id=%s">Replies (%d)</a>`, post.ID, commentCount)
		}

		// Generate fresh timestamp
		item := fmt.Sprintf(`<div class="post-item">
		<h3><a href="/post?id=%s">%s</a></h3>
		<div>%s</div>
		<div class="info">%s · Posted by %s%s%s</div>
	</div>`, post.ID, title, content, app.TimeAgo(post.CreatedAt), authorLink, tagsHtml, replyLink)
		preview = append(preview, item)
	}

	if len(preview) == 0 {
		return "<p>No posts yet. Be the first to share a thought!</p>"
	}
	return strings.Join(preview, "\n")
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
			Posted by %s
			<span style="margin-left: 10px;">·</span>
			<a href="/post?id=%s" style="color: #0066cc; margin-left: 10px;">Reply</a>
		</div>
	</div>`, post.ID, title, content, authorLink, post.ID)

	return item
}

// PostingForm returns the HTML for the posting form
func PostingForm(action string) string {
	return fmt.Sprintf(`<div id="post-form-container">
		<form id="post-form" method="POST" action="%s">
			<input type="text" name="title" placeholder="Title (optional)">
			<textarea name="content" rows="4" placeholder="Share a thought. Be mindful of Allah" required style="font-family: 'Nunito Sans', serif;"></textarea>
			<input type="text" name="tags" placeholder="Tags (optional, comma-separated)" style="font-size: 0.9em;">
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

	// Check if write mode is requested
	showWriteForm := r.URL.Query().Get("write") == "true"

	// Require authentication for write mode
	if showWriteForm {
		if _, err := auth.GetSession(r); err != nil {
			http.Error(w, "Authentication required to create blog posts", http.StatusUnauthorized)
			return
		}
	}

	var content string
	if showWriteForm {
		// Show only the posting form
		content = `<div id="blog">
			<div style="margin-bottom: 30px;">
				<form id="blog-form" method="POST" action="/blog" style="display: flex; flex-direction: column; gap: 10px;">
					<input type="text" name="title" placeholder="Title (optional)" style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
					<textarea id="post-content" name="content" rows="6" placeholder="Share a thought. Be mindful of Allah" required style="padding: 10px; font-family: 'Nunito Sans', serif; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical; min-height: 150px;"></textarea>
					<input type="text" name="tags" placeholder="Tags (optional, comma-separated)" style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
					<div style="display: flex; justify-content: space-between; align-items: center;">
						<span id="char-count" style="font-size: 12px; color: #666;">Min 50 chars</span>
						<div style="display: flex; gap: 10px;">
							<a href="/blog" style="padding: 10px 20px; font-size: 14px; background-color: #ccc; color: #333; text-decoration: none; border-radius: 5px; display: inline-block;">Cancel</a>
							<button type="submit" style="padding: 10px 20px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Post</button>
						</div>
					</div>
				</form>
			</div>
			<script>
				const textarea = document.getElementById('post-content');
				const charCount = document.getElementById('char-count');
				
				// Calculate max height based on viewport
				function getMaxHeight() {
					// Reserve space for title input (60px), buttons (60px), header (100px), padding (80px)
					const reserved = 300;
					return Math.max(200, window.innerHeight - reserved);
				}
				
				// Auto-grow textarea
				function autoGrow() {
					textarea.style.height = 'auto';
					const maxHeight = getMaxHeight();
					textarea.style.height = Math.min(textarea.scrollHeight, maxHeight) + 'px';
				}
				
				// Update character count
				function updateCharCount() {
					const len = textarea.value.length;
					if (len < 50) {
						charCount.textContent = 'Min 50 chars (' + len + ')';
						charCount.style.color = '#666';
					} else {
						charCount.textContent = len + ' chars';
						charCount.style.color = '#666';
					}
				}
				
				textarea.addEventListener('input', function() {
					autoGrow();
					updateCharCount();
				});
				
				// Recalculate on window resize
				window.addEventListener('resize', autoGrow);
				
				// Initial setup
				autoGrow();
			</script>
		</div>`
	} else {
		// Show posts list with conditional write link
		var actions string
		if sess, err := auth.GetSession(r); err == nil {
			// Get account to check member/admin status
			if acc, err := auth.GetAccount(sess.Account); err == nil && (acc.Member || acc.Admin) {
				// User is authenticated and is a member or admin, show write and moderate links
				actions = `<div style="margin-bottom: 15px;">
					<a href="/blog?write=true" style="color: #666; text-decoration: none; font-size: 14px;">Write a Post</a>
					<span style="margin: 0 8px; color: #ccc;">·</span>
					<a href="/admin/moderate" style="color: #666; text-decoration: none; font-size: 14px;">Moderate</a>
				</div>`
			} else if err == nil {
				// User is authenticated but not a member or admin, show only write link
				actions = `<div style="margin-bottom: 15px;">
					<a href="/blog?write=true" style="color: #666; text-decoration: none; font-size: 14px;">Write a Post</a>
				</div>`
			}
		} else {
			// Guest user, show login prompt
			actions = `<div style="margin-bottom: 15px; color: #666; font-size: 14px;">
				<a href="/login?redirect=/blog" style="color: #666; text-decoration: none;">Login</a> to write a post
			</div>`
		}
		content = fmt.Sprintf(`<div id="blog">
			%s
			<div id="posts-list">
				%s
			</div>
		</div>`, actions, list)
	}

	html := app.RenderHTMLForRequest("Blog", "Share your thoughts", content, r)
	w.Write([]byte(html))
}

// CreatePost creates a new post and returns error if any
func CreatePost(title, content, author, authorID, tags string) error {
	// Create new post
	post := &Post{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		Content:   content,
		Author:    author,
		AuthorID:  authorID,
		Tags:      tags,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	// Add to beginning of slice (newest first)
	posts = append([]*Post{post}, posts...)
	// Add to map for O(1) lookups
	postsMap[post.ID] = post
	mutex.Unlock()

	// Save to disk
	if err := save(); err != nil {
		return err
	}

	// Update cached HTML
	updateCache()

	// Index the post for search/RAG
	go func(id, title, content, author, tags string) {
		app.Log("blog", "Indexing post: %s", title)
		data.Index(
			id,
			"post",
			title,
			content,
			map[string]interface{}{
				"url":    "/post?id=" + id,
				"author": author,
				"tags":   tags,
			},
		)
	}(post.ID, post.Title, post.Content, post.Author, post.Tags)

	// Auto-tag if no tags provided
	if tags == "" {
		go autoTagPost(post.ID, title, content)
	}

	return nil
}

// autoTagPost requests AI categorization via pubsub
func autoTagPost(postID, title, content string) {
	app.Log("blog", "Requesting tag generation for post: %s", postID)

	// Publish tag generation request
	data.Publish(data.Event{
		Type: data.EventGenerateTag,
		Data: map[string]interface{}{
			"post_id": postID,
			"title":   title,
			"content": content,
			"type":    "post",
		},
	})
}

// CreateComment adds a comment to a post
func CreateComment(postID, content, author, authorID string) error {
	comment := &Comment{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		PostID:    postID,
		Content:   content,
		Author:    author,
		AuthorID:  authorID,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	comments = append(comments, comment)

	// Add comment directly to the post's Comments slice using map lookup
	if post := postsMap[postID]; post != nil {
		post.Comments = append(post.Comments, comment)
	}

	updateCacheUnlocked()
	mutex.Unlock()

	// Save to disk
	return data.SaveJSON("comments.json", comments)
}

// GetComments retrieves all comments for a post
func GetComments(postID string) []*Comment {
	mutex.RLock()
	defer mutex.RUnlock()

	// Get from post's Comments field using map lookup
	if post := postsMap[postID]; post != nil {
		return post.Comments
	}

	// Fallback: iterate through comments array (shouldn't happen normally)
	var postComments []*Comment
	for _, comment := range comments {
		if comment.PostID == postID {
			postComments = append(postComments, comment)
		}
	}
	return postComments
}

// GetPost retrieves a post by ID
func GetPost(id string) *Post {
	mutex.RLock()
	defer mutex.RUnlock()

	return postsMap[id]
}

// DeletePost removes a post by ID
func DeletePost(id string) error {
	mutex.Lock()
	defer mutex.Unlock()

	// Check if post exists in map first
	if _, exists := postsMap[id]; !exists {
		return fmt.Errorf("post not found")
	}

	// Remove from map
	delete(postsMap, id)

	// Remove from slice
	for i, post := range posts {
		if post.ID == id {
			posts = append(posts[:i], posts[i+1:]...)
			break
		}
	}

	save()
	updateCacheUnlocked()
	return nil
}

// UpdatePost updates an existing post
func UpdatePost(id, title, content, tags string) error {
	mutex.Lock()
	defer mutex.Unlock()

	post := postsMap[id]
	if post == nil {
		return fmt.Errorf("post not found")
	}

	post.Title = title
	post.Content = content
	post.Tags = tags
	save()
	updateCacheUnlocked()

	// Re-index the updated post
	go func(id, title, content, author, tags string) {
		app.Log("blog", "Re-indexing updated post: %s", title)
		data.Index(
			id,
			"post",
			title,
			content,
			map[string]interface{}{
				"url":    "/post?id=" + id,
				"author": author,
				"tags":   tags,
			},
		)
	}(post.ID, post.Title, post.Content, post.Author, post.Tags)

	return nil
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

		var title, content, tags string

		if isJSON {
			var req struct {
				Title   string `json:"title"`
				Content string `json:"content"`
				Tags    string `json:"tags"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(req.Title)
			content = strings.TrimSpace(req.Content)
			tags = parseTags(req.Tags)
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Failed to parse form", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(r.FormValue("title"))
			content = strings.TrimSpace(r.FormValue("content"))
			tags = parseTags(r.FormValue("tags"))
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

				// Check if account can post (30 minute minimum)
				if !auth.CanPost(acc.ID) {
					accountAge := time.Since(acc.Created).Round(time.Minute)
					remaining := (30*time.Minute - time.Since(acc.Created)).Round(time.Minute)
					http.Error(w, fmt.Sprintf("New accounts must wait 30 minutes before posting. Your account is %v old. Please wait %v more.", accountAge, remaining), http.StatusForbidden)
					return
				}
			}
		}

		// Create post
		postID := fmt.Sprintf("%d", time.Now().UnixNano())
		if err := CreatePost(title, content, author, authorID, tags); err != nil {
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

		http.Redirect(w, r, "/blog", http.StatusSeeOther)
		return
	}

	// For other methods, id is required
	if id == "" {
		http.Redirect(w, r, "/blog", 302)
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

		var title, content, tags string

		if isJSON {
			var req struct {
				Title   string `json:"title"`
				Content string `json:"content"`
				Tags    string `json:"tags"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(req.Title)
			content = strings.TrimSpace(req.Content)
			tags = parseTags(req.Tags)
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Failed to parse form", http.StatusBadRequest)
				return
			}
			title = strings.TrimSpace(r.FormValue("title"))
			content = strings.TrimSpace(r.FormValue("content"))
			tags = parseTags(r.FormValue("tags"))
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

		if err := UpdatePost(id, title, content, tags); err != nil {
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

	// Handle DELETE - remove the post
	if r.Method == "DELETE" || (r.Method == "POST" && r.FormValue("_method") == "DELETE") {
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
			http.Error(w, "Forbidden - you can only delete your own posts", http.StatusForbidden)
			return
		}

		if err := DeletePost(id); err != nil {
			http.Error(w, "Failed to delete post", http.StatusInternalServerError)
			return
		}

		if isJSON {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
			})
			return
		}

		http.Redirect(w, r, "/blog", http.StatusSeeOther)
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
				<textarea name="content" rows="15" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical; font-family: 'Nunito Sans', serif;">%s</textarea>
				<input type="text" name="tags" placeholder="Tags (optional, comma-separated)" value="%s" style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
				<div style="font-size: 12px; color: #666; margin-top: -5px;">
					Supports markdown: **bold**, *italic**, `+"`code`"+`, `+"```"+` for code blocks, # headers, - lists
				</div>
				<div style="display: flex; gap: 10px;">
					<button type="submit" style="padding: 10px 20px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Save Changes</button>
					<a href="/post?id=%s" style="padding: 10px 20px; font-size: 14px; background-color: #ccc; color: #333; text-decoration: none; border-radius: 5px; display: inline-block;">Cancel</a>
				</div>
			</form>
		</div>`, post.ID, post.Title, post.Content, post.Tags, post.ID)

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

	// Check if current user is the author (to show edit and delete buttons)
	var editButton string
	sess, err := auth.GetSession(r)
	if err == nil {
		acc, err := auth.GetAccount(sess.Account)
		if err == nil && acc.ID == post.AuthorID {
			editButton = ` · <a href="/post?id=` + post.ID + `&edit=true" style="color: #666;">Edit</a> · <a href="#" onclick="if(confirm('Delete this post?')){var f=document.createElement('form');f.method='POST';f.action='/post?id=` + post.ID + `';var i=document.createElement('input');i.type='hidden';i.name='_method';i.value='DELETE';f.appendChild(i);document.body.appendChild(f);f.submit();}return false;" style="color: #d9534f;">Delete</a>`
		}
	}

	tagsHtml := ""
	if post.Tags != "" {
		for _, tag := range strings.Split(post.Tags, ",") {
			tagsHtml += fmt.Sprintf(` · <span class="category">%s</span>`, strings.TrimSpace(tag))
		}
	}
	content := fmt.Sprintf(`<div id="blog">
		<div class="info" style="color: #666; font-size: small;">
			%s · %s%s%s · <a href="#" onclick="flagPost('%s'); return false;" style="color: #666;">Flag</a> · <a href="#" onclick="navigator.share ? navigator.share({title: document.title, url: window.location.href}) : navigator.clipboard.writeText(window.location.href).then(() => alert('Link copied to clipboard!')); return false;" style="color: #666;">Share</a>
		</div>
		<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>
		<div style="margin-bottom: 20px;">%s</div>
		<hr style='margin: 20px 0; border: none; border-top: 1px solid #eee;'>
		<h3 style="margin-top: 30px;">Comments</h3>
		%s
		<div style="margin-top: 30px;">
			<a href="/blog" style="color: #666; text-decoration: none;">← Back to posts</a>
		</div>
	</div>`, app.TimeAgo(post.CreatedAt), authorLink, tagsHtml, editButton, post.ID, contentHTML, renderComments(post.ID, r))

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

// renderComments displays comments for a post
func renderComments(postID string, r *http.Request) string {
	postComments := GetComments(postID)

	var commentsHTML strings.Builder

	// Add comment form if authenticated
	_, err := auth.GetSession(r)
	isAuthenticated := err == nil

	if isAuthenticated {
		commentsHTML.WriteString(fmt.Sprintf(`
			<form method="POST" action="/post/%s/comment" style="margin: 20px 0; display: flex; flex-direction: column; gap: 10px;">
				<textarea name="content" rows="3" placeholder="Add a comment..." required style="padding: 10px; font-family: 'Nunito Sans', serif; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical;"></textarea>
				<div>
					<button type="submit" style="padding: 8px 16px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Add Comment</button>
				</div>
			</form>
		`, postID))
	} else {
		commentsHTML.WriteString(`<p style="color: #666; margin: 20px 0;"><a href="/login" style="color: #0066cc;">Login</a> to add a comment</p>`)
	}

	if len(postComments) == 0 {
		commentsHTML.WriteString(`<p style="color: #999; font-style: italic; margin: 20px 0;">No comments yet. Be the first to comment!</p>`)
		return commentsHTML.String()
	}

	commentsHTML.WriteString(`<div style="margin-top: 20px;">`)
	for _, comment := range postComments {
		authorLink := comment.Author
		if comment.AuthorID != "" {
			authorLink = fmt.Sprintf(`<a href="/@%s" style="color: #0066cc;">%s</a>`, comment.AuthorID, comment.Author)
		}

		commentsHTML.WriteString(fmt.Sprintf(`
			<div style="padding: 15px; background: #f9f9f9; border-radius: 5px; margin-bottom: 10px;">
				<div style="color: #666; font-size: 12px; margin-bottom: 5px;">%s · %s</div>
				<div style="white-space: pre-wrap;">%s</div>
			</div>
		`, app.TimeAgo(comment.CreatedAt), authorLink, comment.Content))
	}
	commentsHTML.WriteString(`</div>`)

	return commentsHTML.String()
}

// EditHandler serves the post edit form
// RenderMarkdown converts markdown to HTML without embeds (for storage/previews)
func RenderMarkdown(text string) string {
	return string(app.Render([]byte(text)))
}

// Linkify converts markdown to HTML and embeds YouTube videos (for full post display)
func Linkify(text string) string {
	// Render markdown to HTML first
	html := string(app.Render([]byte(text)))

	// Find YouTube links in the rendered HTML and replace with embeds
	// Pattern matches: <a href="youtube_url">youtube_url</a>
	youtubePattern := regexp.MustCompile(`<a href="https?://(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})[^"]*"[^>]*>.*?</a>`)
	html = youtubePattern.ReplaceAllStringFunc(html, func(match string) string {
		// Extract video ID from the match
		idPattern := regexp.MustCompile(`(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})`)
		matches := idPattern.FindStringSubmatch(match)
		if len(matches) > 1 {
			videoID := matches[1]
			return fmt.Sprintf(`<div style="position: relative; padding-bottom: 56.25%%; height: 0; overflow: hidden; max-width: 100%%; margin: 15px 0;"><iframe src="/video?id=%s" style="position: absolute; top: 0; left: 0; width: 100%%; height: 100%%; border: 0;" allowfullscreen loading="lazy"></iframe></div>`, videoID)
		}
		return match
	})

	return html
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	// Require authentication for posting
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Authentication required to create posts", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	tags := parseTags(r.FormValue("tags"))

	if content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	// Content validation: minimum and maximum length
	if len(content) < 50 {
		http.Error(w, "Post content must be at least 50 characters", http.StatusBadRequest)
		return
	}
	if len(content) > 10000 {
		http.Error(w, "Post content must not exceed 10,000 characters", http.StatusBadRequest)
		return
	}

	// Spam detection: check for common test patterns and inappropriate content
	contentLower := strings.ToLower(content)
	titleLower := strings.ToLower(title)
	combined := titleLower + " " + contentLower

	spamPatterns := []string{
		"this is a test",
		"test post",
		"testing",
		"asdf",
		"qwerty",
		"lorem ipsum",
		"topkek",
		"lmao",
		"lmfao",
		"lol",
		"haha",
		"hehe",
		"dawg",
		"bruh",
		"yolo",
	}

	for _, pattern := range spamPatterns {
		if strings.Contains(combined, pattern) && len(content) < 200 {
			http.Error(w, "Post appears to be spam or inappropriate. Please share meaningful content.", http.StatusBadRequest)
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

	// Get the authenticated user (session already validated at function start)
	author := "Anonymous"
	authorID := ""
	acc, err := auth.GetAccount(sess.Account)
	if err == nil {
		author = acc.Name
		authorID = acc.ID
	}

	// Create the post
	postID := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := CreatePost(title, content, author, authorID, tags); err != nil {
		http.Error(w, "Failed to save post", http.StatusInternalServerError)
		return
	}

	// Run async LLM-based content moderation (non-blocking)
	go admin.CheckContent("post", postID, title, content)

	// Redirect back to posts page
	http.Redirect(w, r, "/blog", http.StatusSeeOther)
}

// CommentHandler handles comment submissions
func CommentHandler(w http.ResponseWriter, r *http.Request) {
	// Only handle /post/{postID}/comment paths
	if !strings.Contains(r.URL.Path, "/comment") {
		// Not a comment path, pass through to PostHandler
		PostHandler(w, r)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require authentication
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Extract post ID from URL path (/post/{postID}/comment)
	path := strings.TrimPrefix(r.URL.Path, "/post/")
	path = strings.TrimSuffix(path, "/comment")
	postID := path

	// Verify post exists
	post := GetPost(postID)
	if post == nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		http.Error(w, "Comment content is required", http.StatusBadRequest)
		return
	}

	// Get the authenticated user
	author := "Anonymous"
	authorID := ""
	acc, err := auth.GetAccount(sess.Account)
	if err == nil {
		author = acc.Name
		authorID = acc.ID
	}

	// Create the comment
	if err := CreateComment(postID, content, author, authorID); err != nil {
		http.Error(w, "Failed to save comment", http.StatusInternalServerError)
		return
	}

	// Redirect back to the post
	http.Redirect(w, r, "/post?id="+postID, http.StatusSeeOther)
}
