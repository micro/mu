package blog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

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
	CreatedAt time.Time `json:"created_at"`
}

// Load blog posts from disk
func Load() {
	b, err := data.Load("blog.json")
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
}

// Save blog posts to disk
func save() error {
	return data.SaveJSON("blog.json", posts)
}

// Update cached HTML
func updateCache() {
	mutex.Lock()
	defer mutex.Unlock()

	// Generate preview for home page (latest 3 posts)
	var preview []string
	count := 3
	if len(posts) < count {
		count = len(posts)
	}
	for i := 0; i < count; i++ {
		post := posts[i]
		content := post.Content
		if len(content) > 150 {
			content = content[:150] + "..."
		}
		
		title := post.Title
		if title == "" {
			title = "Untitled"
		}
		
		item := fmt.Sprintf(`<div class="post-item">
			<strong>%s</strong><br>
			<span style="white-space: pre-wrap;">%s</span><br>
			<small style="color: #666;">%s</small>
		</div>`, title, content, post.CreatedAt.Format("Jan 2, 2006 3:04 PM"))
		preview = append(preview, item)
	}
	
	if len(preview) == 0 {
		postsPreviewHtml = "<p>No posts yet. Be the first to share a thought!</p>"
	} else {
		postsPreviewHtml = strings.Join(preview, "\n")
	}

	// Generate full list for blog page
	var fullList []string
	for _, post := range posts {
		title := post.Title
		if title == "" {
			title = "Untitled"
		}
		
		item := fmt.Sprintf(`<div class="post-item">
			<h3>%s</h3>
			<p style="white-space: pre-wrap;">%s</p>
			<small style="color: #666;">Posted by %s on %s</small>
		</div>`, title, post.Content, post.Author, post.CreatedAt.Format("January 2, 2006 at 3:04 PM"))
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
	mutex.RLock()
	defer mutex.RUnlock()
	return postsPreviewHtml
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
		<h2>Posts</h2>
		<div style="margin-bottom: 30px;">
			<form id="blog-form" method="POST" action="/blog" style="display: flex; flex-direction: column; gap: 10px;">
				<input type="text" name="title" placeholder="Title (optional)" style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
				<textarea name="content" rows="6" placeholder="Share a thought. Be mindful of Allah" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical;"></textarea>
				<button type="submit" style="padding: 10px 20px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer; align-self: flex-start;">Post</button>
			</form>
		</div>
		<hr style='margin: 30px 0; border: none; border-top: 2px solid #333;'>
		<div id="posts-list">
			%s
		</div>
	</div>`, list)

	html := app.RenderHTML("Blog", "Share your thoughts", content)
	w.Write([]byte(html))
}

// handlePost processes the POST request to create a new blog post
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
	sess, err := auth.GetSession(r)
	if err == nil {
		acc, err := auth.GetAccount(sess.Account)
		if err == nil {
			author = acc.Name
		}
	}

	// Create new post
	post := &Post{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		Content:   content,
		Author:    author,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	// Add to beginning of slice (newest first)
	posts = append([]*Post{post}, posts...)
	mutex.Unlock()

	// Save to disk
	if err := save(); err != nil {
		http.Error(w, "Failed to save post", http.StatusInternalServerError)
		return
	}

	// Update cached HTML
	updateCache()

	// Redirect back to blog page
	http.Redirect(w, r, "/blog", http.StatusSeeOther)
}
