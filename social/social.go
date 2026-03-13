package social

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/admin"
	"mu/app"
	"mu/auth"
	"mu/data"
	"mu/wallet"
)

//go:embed topics.json
var topicsJSON []byte

var (
	mutex   sync.RWMutex
	threads []*Thread
	topics  []string

	// cached HTML
	previewHTML string
)

// Thread is a discussion topic
type Thread struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Link      string    `json:"link,omitempty"`  // optional URL
	Content   string    `json:"content"`         // markdown body
	Topic     string    `json:"topic"`           // must be a valid topic
	Author    string    `json:"author"`          // display name
	AuthorID  string    `json:"author_id"`       // username
	CreatedAt time.Time `json:"created_at"`
	Replies   []*Reply  `json:"replies,omitempty"`
}

// Reply is a response to a thread
type Reply struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	ParentID  string    `json:"parent_id,omitempty"` // for nested replies
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ReplyCount returns the number of replies on a thread
func (t *Thread) ReplyCount() int {
	if t.Replies == nil {
		return 0
	}
	return len(t.Replies)
}

func Load() {
	// Load topics
	json.Unmarshal(topicsJSON, &topics)

	// Load threads from disk
	b, err := data.LoadFile("social.json")
	if err == nil && len(b) > 0 {
		json.Unmarshal([]byte(b), &threads)
	}

	// Sort newest first
	sortThreads()

	// Update cached HTML
	updateCache()

	// Index existing threads
	go func() {
		for _, t := range threads {
			indexThread(t)
		}
	}()

	// Register admin deleter
	admin.RegisterDeleter("thread", &threadDeleter{})
}

func sortThreads() {
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].CreatedAt.After(threads[j].CreatedAt)
	})
}

func save() error {
	mutex.RLock()
	defer mutex.RUnlock()
	return data.SaveJSON("social.json", threads)
}

func indexThread(t *Thread) {
	data.Index(t.ID, "thread", t.Title, t.Content, map[string]interface{}{
		"url":    "/social?id=" + t.ID,
		"author": t.Author,
		"topic":  t.Topic,
	})
}

func updateCache() {
	mutex.RLock()
	defer mutex.RUnlock()

	var sb strings.Builder
	count := 0
	for _, t := range threads {
		if admin.IsHidden("thread", t.ID) {
			continue
		}
		if count >= 5 {
			break
		}
		replies := t.ReplyCount()
		replyText := "replies"
		if replies == 1 {
			replyText = "reply"
		}
		sb.WriteString(fmt.Sprintf(`<div style="padding:6px 0;border-bottom:1px solid #f0f0f0;">
<a href="/social?id=%s" style="font-weight:600;color:#111;">%s</a>
<div style="font-size:12px;color:#888;">%s · %s · %d %s</div>
</div>`, t.ID, html.EscapeString(t.Title), html.EscapeString(t.Topic), app.TimeAgo(t.CreatedAt), replies, replyText))
		count++
	}
	if count == 0 {
		sb.WriteString(`<p style="color:#888;font-size:13px;">No discussions yet.</p>`)
	}
	previewHTML = sb.String()
}

// Preview returns HTML for the home card
func Preview() string {
	mutex.RLock()
	defer mutex.RUnlock()
	return previewHTML
}

// GetTopics returns available topics
func GetTopics() []string {
	return topics
}

func isValidTopic(topic string) bool {
	for _, t := range topics {
		if t == topic {
			return true
		}
	}
	return false
}

func getThread(id string) *Thread {
	for _, t := range threads {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// Handler serves the social page
func Handler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	switch r.Method {
	case "POST":
		if id != "" {
			handleReply(w, r, id)
		} else {
			handleCreate(w, r)
		}
	case "DELETE":
		handleDelete(w, r, id)
	default:
		if id != "" {
			handleThread(w, r, id)
		} else {
			handleList(w, r)
		}
	}
}

// GuidelinesHandler serves the community guidelines page
func GuidelinesHandler(w http.ResponseWriter, r *http.Request) {
	content := string(app.Render([]byte(Guidelines)))
	html := app.RenderHTMLForRequest("Community Guidelines", "Community Guidelines", fmt.Sprintf(`<div class="post-item">%s</div>
<p class="mt-5"><a href="/social">← Back to discussions</a></p>`, content), r)
	w.Write([]byte(html))
}

func handleList(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")

	mutex.RLock()
	var visible []*Thread
	for _, t := range threads {
		if admin.IsHidden("thread", t.ID) {
			continue
		}
		if topic != "" && topic != "all" && t.Topic != topic {
			continue
		}
		visible = append(visible, t)
	}
	mutex.RUnlock()

	// JSON API
	if app.WantsJSON(r) {
		type threadJSON struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Link      string    `json:"link,omitempty"`
			Content   string    `json:"content"`
			Topic     string    `json:"topic"`
			Author    string    `json:"author"`
			AuthorID  string    `json:"author_id"`
			Replies   int       `json:"replies"`
			CreatedAt time.Time `json:"created_at"`
		}
		var out []threadJSON
		for _, t := range visible {
			out = append(out, threadJSON{
				ID:        t.ID,
				Title:     t.Title,
				Link:      t.Link,
				Content:   t.Content,
				Topic:     t.Topic,
				Author:    t.Author,
				AuthorID:  t.AuthorID,
				Replies:   t.ReplyCount(),
				CreatedAt: t.CreatedAt,
			})
		}
		app.RespondJSON(w, out)
		return
	}

	// HTML
	head := app.Head("social", topics)

	var sb strings.Builder

	// New thread form (shown if logged in)
	_, acc := auth.TrySession(r)
	if acc != nil {
		selectedTopic := topic
		if selectedTopic == "" || selectedTopic == "all" {
			selectedTopic = "all"
		}
		var topicOptions string
		for _, t := range topics {
			sel := ""
			if t == selectedTopic {
				sel = " selected"
			}
			topicOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, t, sel, t)
		}
		sb.WriteString(GuidelinesHTML)
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/social" class="blog-form">
<input type="text" name="title" placeholder="Title" required>
<input type="text" name="link" placeholder="Link (optional)">
<textarea name="content" rows="4" placeholder="What do you want to discuss?" required></textarea>
<div style="display:flex;gap:8px;align-items:center;">
<select name="topic">%s</select>
<button type="submit">Post</button>
</div>
</form>`, topicOptions))
	} else {
		sb.WriteString(`<p class="text-muted"><a href="/login">Login</a> to start a discussion</p>`)
	}

	// Thread list
	if len(visible) == 0 {
		sb.WriteString(`<p class="text-muted mt-5">No discussions yet. Be the first to start one.</p>`)
	}
	for _, t := range visible {
		replies := t.ReplyCount()
		replyText := "replies"
		if replies == 1 {
			replyText = "reply"
		}

		titleHTML := fmt.Sprintf(`<a href="/social?id=%s" style="font-weight:600;">%s</a>`, t.ID, html.EscapeString(t.Title))
		if t.Link != "" {
			titleHTML = fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" style="font-weight:600;">%s</a> <a href="/social?id=%s" style="font-size:12px;color:#888;">[discuss]</a>`,
				html.EscapeString(t.Link), html.EscapeString(t.Title), t.ID)
		}

		sb.WriteString(fmt.Sprintf(`<div class="card" style="padding:12px 16px;">
<div>%s</div>
<div style="font-size:12px;color:#888;margin-top:4px;">
<span class="category">%s</span>
<a href="/@%s" class="text-muted">%s</a> · %s · <a href="/social?id=%s" class="text-muted">%d %s</a>
</div>
</div>`, titleHTML, html.EscapeString(t.Topic), t.AuthorID, html.EscapeString(t.Author), app.TimeAgo(t.CreatedAt), t.ID, replies, replyText))
	}

	page := app.RenderHTMLForRequest("Social", "Discussions", fmt.Sprintf(`<div id="social">%s%s</div>`, head, sb.String()), r)
	w.Write([]byte(page))
}

func handleThread(w http.ResponseWriter, r *http.Request, id string) {
	mutex.RLock()
	t := getThread(id)
	mutex.RUnlock()

	if t == nil {
		http.NotFound(w, r)
		return
	}

	if admin.IsHidden("thread", t.ID) {
		http.NotFound(w, r)
		return
	}

	// JSON API
	if app.WantsJSON(r) {
		app.RespondJSON(w, t)
		return
	}

	// HTML
	var sb strings.Builder

	// Thread content
	contentHTML := string(app.Render([]byte(t.Content)))
	titleHTML := html.EscapeString(t.Title)
	if t.Link != "" {
		titleHTML = fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, html.EscapeString(t.Link), html.EscapeString(t.Title))
	}

	// Delete button for author/admin
	var deleteBtn string
	_, acc := auth.TrySession(r)
	if acc != nil && (acc.ID == t.AuthorID || acc.Admin) {
		deleteBtn = fmt.Sprintf(` · <a href="#" onclick="if(confirm('Delete this thread?')){var f=document.createElement('form');f.method='POST';f.action='/social?id=%s';var i=document.createElement('input');i.type='hidden';i.name='_method';i.value='DELETE';f.appendChild(i);document.body.appendChild(f);f.submit();}return false;" class="text-error">Delete</a>`, t.ID)
	}

	sb.WriteString(fmt.Sprintf(`<div class="card">
<h2 style="margin-top:0;">%s</h2>
<div style="font-size:13px;color:#888;margin-bottom:12px;">
<span class="category">%s</span>
<a href="/@%s" class="text-muted">%s</a> · %s%s
</div>
<div>%s</div>
</div>`, titleHTML, html.EscapeString(t.Topic), t.AuthorID, html.EscapeString(t.Author), app.TimeAgo(t.CreatedAt), deleteBtn, contentHTML))

	// Replies
	if len(t.Replies) > 0 {
		sb.WriteString(`<h3 style="margin-top:20px;">Replies</h3>`)
		for _, reply := range t.Replies {
			replyHTML := string(app.Render([]byte(reply.Content)))
			var replyDelete string
			if acc != nil && (acc.ID == reply.AuthorID || acc.Admin) {
				replyDelete = fmt.Sprintf(` · <a href="#" onclick="if(confirm('Delete this reply?')){var f=document.createElement('form');f.method='POST';f.action='/social?id=%s&reply=%s';var i=document.createElement('input');i.type='hidden';i.name='_method';i.value='DELETE';f.appendChild(i);document.body.appendChild(f);f.submit();}return false;" class="text-error">Delete</a>`, t.ID, reply.ID)
			}
			sb.WriteString(fmt.Sprintf(`<div class="card" style="padding:10px 16px;margin-left:20px;">
<div style="font-size:12px;color:#888;margin-bottom:6px;">
<a href="/@%s" class="text-muted">%s</a> · %s%s
</div>
<div>%s</div>
</div>`, reply.AuthorID, html.EscapeString(reply.Author), app.TimeAgo(reply.CreatedAt), replyDelete, replyHTML))
		}
	}

	// Reply form
	if acc != nil {
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/social?id=%s" class="blog-form mt-5">
<textarea name="content" rows="3" placeholder="Be respectful and stay on topic..." required></textarea>
<button type="submit">Reply</button>
</form>`, t.ID))
	} else {
		sb.WriteString(`<p class="text-muted mt-5"><a href="/login">Login</a> to reply</p>`)
	}

	sb.WriteString(`<p class="mt-5"><a href="/social">← Back to discussions</a></p>`)

	page := app.RenderHTMLForRequest("Social", t.Title, sb.String(), r)
	w.Write([]byte(page))
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var title, link, content, topic string

	if app.SendsJSON(r) {
		var req struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Content string `json:"content"`
			Topic   string `json:"topic"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.BadRequest(w, r, "invalid json")
			return
		}
		title = strings.TrimSpace(req.Title)
		link = strings.TrimSpace(req.Link)
		content = strings.TrimSpace(req.Content)
		topic = strings.TrimSpace(req.Topic)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		title = strings.TrimSpace(r.FormValue("title"))
		link = strings.TrimSpace(r.FormValue("link"))
		content = strings.TrimSpace(r.FormValue("content"))
		topic = strings.TrimSpace(r.FormValue("topic"))
	}

	// Validate
	if title == "" || content == "" {
		app.BadRequest(w, r, "Title and content are required")
		return
	}
	if len(title) > 200 {
		app.BadRequest(w, r, "Title must be under 200 characters")
		return
	}
	if len(content) < 10 {
		app.BadRequest(w, r, "Content must be at least 10 characters")
		return
	}
	if topic == "" {
		topic = "all"
	}
	if !isValidTopic(topic) {
		app.BadRequest(w, r, "Invalid topic")
		return
	}
	if link != "" && !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		app.BadRequest(w, r, "Link must be a valid URL")
		return
	}

	// Check account age
	if !auth.CanPost(acc.ID) {
		app.Forbidden(w, r, "New accounts must wait 30 minutes before posting")
		return
	}

	// Check quota
	canProceed, _, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpSocialPost)
	if !canProceed {
		if app.SendsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(402)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "quota_exceeded",
				"message": "Daily limit reached. Please top up credits at /wallet",
				"cost":    cost,
			})
			return
		}
		c := wallet.QuotaExceededPage(wallet.OpSocialPost, cost)
		page := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", c, r)
		w.Write([]byte(page))
		return
	}

	thread := &Thread{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		Link:      link,
		Content:   content,
		Topic:     topic,
		Author:    acc.Name,
		AuthorID:  acc.ID,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	threads = append([]*Thread{thread}, threads...)
	mutex.Unlock()

	// Consume quota
	wallet.ConsumeQuota(acc.ID, wallet.OpSocialPost)

	save()
	indexThread(thread)
	updateCache()

	// Content moderation
	go admin.CheckContent("thread", thread.ID, thread.Title, thread.Content)

	if app.SendsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true, "id": thread.ID})
		return
	}
	http.Redirect(w, r, "/social?id="+thread.ID, http.StatusSeeOther)
}

func handleReply(w http.ResponseWriter, r *http.Request, threadID string) {
	// Check for method override (DELETE)
	if r.FormValue("_method") == "DELETE" {
		handleDelete(w, r, threadID)
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var content string

	if app.SendsJSON(r) {
		var req struct {
			Content string `json:"content"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.BadRequest(w, r, "invalid json")
			return
		}
		content = strings.TrimSpace(req.Content)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		content = strings.TrimSpace(r.FormValue("content"))
	}

	if content == "" {
		app.BadRequest(w, r, "Content is required")
		return
	}
	if len(content) < 3 {
		app.BadRequest(w, r, "Reply must be at least 3 characters")
		return
	}

	// Check account age
	if !auth.CanPost(acc.ID) {
		app.Forbidden(w, r, "New accounts must wait 30 minutes before posting")
		return
	}

	// Check quota
	canProceed, _, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpSocialPost)
	if !canProceed {
		if app.SendsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(402)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "quota_exceeded",
				"message": "Daily limit reached. Please top up credits at /wallet",
				"cost":    cost,
			})
			return
		}
		c := wallet.QuotaExceededPage(wallet.OpSocialPost, cost)
		page := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", c, r)
		w.Write([]byte(page))
		return
	}

	mutex.Lock()
	t := getThread(threadID)
	if t == nil {
		mutex.Unlock()
		http.NotFound(w, r)
		return
	}

	reply := &Reply{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		ThreadID:  threadID,
		Content:   content,
		Author:    acc.Name,
		AuthorID:  acc.ID,
		CreatedAt: time.Now(),
	}
	t.Replies = append(t.Replies, reply)
	mutex.Unlock()

	// Consume quota
	wallet.ConsumeQuota(acc.ID, wallet.OpSocialPost)

	save()
	updateCache()

	// Content moderation
	go admin.CheckContent("thread", reply.ID, "", reply.Content)

	if app.SendsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true, "id": reply.ID})
		return
	}
	http.Redirect(w, r, "/social?id="+threadID, http.StatusSeeOther)
}

func handleDelete(w http.ResponseWriter, r *http.Request, threadID string) {
	// Support _method override from forms
	if r.Method == "POST" && r.FormValue("_method") != "DELETE" {
		// This is actually a reply, not a delete
		handleReply(w, r, threadID)
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	replyID := r.URL.Query().Get("reply")

	mutex.Lock()
	t := getThread(threadID)
	if t == nil {
		mutex.Unlock()
		http.NotFound(w, r)
		return
	}

	if replyID != "" {
		// Delete a reply
		for i, reply := range t.Replies {
			if reply.ID == replyID && (acc.ID == reply.AuthorID || acc.Admin) {
				t.Replies = append(t.Replies[:i], t.Replies[i+1:]...)
				break
			}
		}
	} else {
		// Delete the thread
		if acc.ID != t.AuthorID && !acc.Admin {
			mutex.Unlock()
			app.Forbidden(w, r, "You can only delete your own threads")
			return
		}
		for i, thread := range threads {
			if thread.ID == threadID {
				threads = append(threads[:i], threads[i+1:]...)
				break
			}
		}
	}
	mutex.Unlock()

	save()
	updateCache()

	if app.SendsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true})
		return
	}
	if replyID != "" {
		http.Redirect(w, r, "/social?id="+threadID, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/social", http.StatusSeeOther)
	}
}

// threadDeleter implements admin.ContentDeleter for threads
type threadDeleter struct{}

func (d *threadDeleter) Delete(id string) error {
	mutex.Lock()
	for i, t := range threads {
		if t.ID == id {
			threads = append(threads[:i], threads[i+1:]...)
			break
		}
	}
	mutex.Unlock()
	save()
	updateCache()
	return nil
}

func (d *threadDeleter) Get(id string) interface{} {
	mutex.RLock()
	defer mutex.RUnlock()
	return getThread(id)
}

func (d *threadDeleter) RefreshCache() {
	updateCache()
}
