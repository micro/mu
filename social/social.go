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

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/moderation"
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
// CommunityNote is a fact-check annotation attached to a thread or reply.
// Generated automatically by searching the web for claims in the post
// and using AI to assess accuracy.
type CommunityNote struct {
	Content   string    `json:"content"`              // the fact-check text
	Sources   []Source  `json:"sources,omitempty"`     // reference links
	Status    string    `json:"status"`                // "accurate", "misleading", "missing_context", "unverifiable"
	CheckedAt time.Time `json:"checked_at"`
}

// Source is a reference link for a community note
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Thread struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Link      string         `json:"link,omitempty"`  // optional URL
	Content   string         `json:"content"`         // markdown body
	Topic     string         `json:"topic"`           // must be a valid topic
	Author    string         `json:"author"`          // display name
	AuthorID  string         `json:"author_id"`       // username
	CreatedAt time.Time      `json:"created_at"`
	Replies   []*Reply       `json:"replies,omitempty"`
	Note      *CommunityNote `json:"note,omitempty"` // fact-check annotation
}

// Reply is a response to a thread
type Reply struct {
	ID        string         `json:"id"`
	ThreadID  string         `json:"thread_id"`
	ParentID  string         `json:"parent_id,omitempty"` // for nested replies
	Content   string         `json:"content"`
	Author    string         `json:"author"`
	AuthorID  string         `json:"author_id"`
	CreatedAt time.Time      `json:"created_at"`
	Note      *CommunityNote `json:"note,omitempty"` // fact-check annotation
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

	// Load blocklist for seeded content filtering
	loadBlocklist()

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
	moderation.RegisterDeleter("thread", &threadDeleter{})
}

func sortThreads() {
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].CreatedAt.After(threads[j].CreatedAt)
	})
}

// Save persists threads to disk. Used by agent and internal functions.
func Save() error {
	return save()
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

// UpdateCache refreshes internal caches. Used by agent and internal functions.
func UpdateCache() {
	updateCache()
}

func updateCache() {
	mutex.RLock()
	defer mutex.RUnlock()

	var sb strings.Builder
	count := 0
	for _, t := range threads {
		if moderation.IsHidden("thread", t.ID) {
			continue
		}
		if count >= 5 {
			break
		}
		replies := t.ReplyCount()
		replyLink := fmt.Sprintf(`<a href="/social?id=%s" style="color:#888;">Reply</a>`, t.ID)
		if replies > 0 {
			replyLink = fmt.Sprintf(`<a href="/social?id=%s" style="color:#888;">Replies (%d)</a>`, t.ID, replies)
		}
		sb.WriteString(fmt.Sprintf(`<div style="padding:6px 0;border-bottom:1px solid #f0f0f0;">
<a href="/social?id=%s" style="font-weight:600;color:#111;">%s</a>
<div style="font-size:12px;color:#888;">%s · %s · %s</div>
</div>`, t.ID, html.EscapeString(t.Title), html.EscapeString(t.Topic), app.TimeAgo(t.CreatedAt), replyLink))
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

// GetThread returns a thread by ID. Used by agent and internal functions.
func GetThread(id string) *Thread {
	mutex.RLock()
	defer mutex.RUnlock()
	return getThreadLocked(id)
}

// getThreadLocked is the internal version that assumes mutex is held.
func getThreadLocked(id string) *Thread {
	for _, t := range threads {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// getThread returns a thread by ID. Caller must hold mutex (read or write).
func getThread(id string) *Thread {
	return getThreadLocked(id)
}

// Handler serves the social page
func Handler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	switch r.Method {
	case "POST":
		if id != "" && r.URL.Query().Get("factcheck") == "true" {
			FactCheckHandler(w, r, id)
			return
		}
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
		if moderation.IsHidden("thread", t.ID) {
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

	// HTML — topic selector with query params for actual filtering
	var headBuf strings.Builder
	if topic == "" || topic == "all" {
		headBuf.WriteString(`<a href="/social" class="head active">All</a>`)
	} else {
		headBuf.WriteString(`<a href="/social" class="head">All</a>`)
	}
	for _, t := range topics {
		if strings.EqualFold(t, "all") {
			continue
		}
		cls := "head"
		if t == topic {
			cls = "head active"
		}
		headBuf.WriteString(fmt.Sprintf(`<a href="/social?topic=%s" class="%s">%s</a>`, t, cls, t))
	}
	head := headBuf.String()

	var sb strings.Builder

	// Action bar and optional post form
	_, acc := auth.TrySession(r)
	showForm := r.URL.Query().Get("post") == "true"
	if acc != nil && showForm {
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
	} else if acc != nil {
		sb.WriteString(`<div class="mt-4 mb-4">
<a href="/social?post=true" class="btn">+ New Discussion</a>
<a href="/social/guidelines" class="text-muted text-sm ml-4">Guidelines</a>
</div>`)
	} else {
		sb.WriteString(`<div class="mt-4 mb-4 text-muted text-sm">
<a href="/login?redirect=/social" class="text-muted">Login</a> to start a discussion
</div>`)
	}

	// Thread list
	if len(visible) == 0 {
		sb.WriteString(`<p class="text-muted mt-5">No discussions yet. Be the first to start one.</p>`)
	} else if topic != "" && topic != "all" {
		// Filtered view — show all threads for this topic
		for _, t := range visible {
			sb.WriteString(renderThreadCard(t, acc))
		}
	} else {
		// "All" view — show latest first, then grouped by topic
		latestCount := 5
		if len(visible) < latestCount {
			latestCount = len(visible)
		}

		// Latest section
		for _, t := range visible[:latestCount] {
			sb.WriteString(renderThreadCard(t, acc))
		}

		// Group remaining by topic
		byTopic := map[string][]*Thread{}
		shownIDs := map[string]bool{}
		for _, t := range visible[:latestCount] {
			shownIDs[t.ID] = true
		}
		for _, t := range visible {
			if shownIDs[t.ID] {
				continue
			}
			byTopic[t.Topic] = append(byTopic[t.Topic], t)
		}

		// Render per-topic sections
		topicOrder := []string{}
		for _, t := range topics {
			if strings.EqualFold(t, "all") {
				continue
			}
			if len(byTopic[t]) > 0 {
				topicOrder = append(topicOrder, t)
			}
		}

		for _, topicName := range topicOrder {
			threads := byTopic[topicName]
			sb.WriteString(fmt.Sprintf(`<hr id="%s" class="anchor"><h3 style="margin:16px 0 8px;">%s</h3>`, topicName, topicName))
			for _, t := range threads {
				sb.WriteString(renderThreadCard(t, acc))
			}
		}
	}

	page := app.RenderHTMLForRequest("Social", "Discussions", fmt.Sprintf(`<div id="social">%s%s</div>`, head, sb.String()), r)
	w.Write([]byte(page))
}

// renderThreadCard renders a single thread as a card in the listing
func renderThreadCard(t *Thread, acc *auth.Account) string {
	replies := t.ReplyCount()
	replyLink := fmt.Sprintf(` · <a href="/social?id=%s">Reply</a>`, t.ID)
	if replies > 0 {
		replyLink = fmt.Sprintf(` · <a href="/social?id=%s">Replies (%d)</a>`, t.ID, replies)
	}

	linkHTML := ""
	if t.Link != "" {
		linkHTML = fmt.Sprintf(` · <a href="%s" target="_blank" rel="noopener noreferrer">Link</a>`, html.EscapeString(t.Link))
	}

	// Dismiss button for admin on system-seeded threads
	dismissBtn := ""
	if acc != nil && acc.Admin && t.AuthorID == app.SystemUserID {
		dismissBtn = fmt.Sprintf(` · <form method="POST" action="/social/dismiss" style="display:inline;"><input type="hidden" name="id" value="%s"><button type="submit" style="background:none;border:none;color:#c00;font-size:12px;cursor:pointer;padding:0;">Dismiss</button></form>`, t.ID)
	}

	// Build content preview — strip markdown formatting for plain text snippet
	preview := stripMarkdown(t.Content)
	if len(preview) > 150 {
		preview = preview[:150] + "..."
	}
	previewHTML := ""
	if preview != "" {
		previewHTML = fmt.Sprintf(`<div style="font-size:13px;color:#aaa;margin-top:4px;">%s</div>`, html.EscapeString(preview))
	}

	return fmt.Sprintf(`<div class="card" style="padding:12px 16px;">
<div><a href="/social?id=%s" style="font-weight:600;">%s</a></div>%s
<div style="font-size:12px;color:#888;margin-top:4px;">
<span class="category">%s</span>
<a href="/@%s" class="text-muted">%s</a> · %s%s%s%s
</div>
</div>`, t.ID, html.EscapeString(t.Title), previewHTML, html.EscapeString(t.Topic), t.AuthorID, html.EscapeString(t.Author), app.TimeAgo(t.CreatedAt), replyLink, linkHTML, dismissBtn)
}

// stripMarkdown removes common markdown formatting to produce plain text for previews.
func stripMarkdown(s string) string {
	// Remove code blocks
	for {
		start := strings.Index(s, "```")
		if start == -1 {
			break
		}
		end := strings.Index(s[start+3:], "```")
		if end == -1 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+3+end+3:]
	}
	// Remove inline code
	for strings.Contains(s, "`") {
		start := strings.Index(s, "`")
		end := strings.Index(s[start+1:], "`")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+1:start+1+end] + s[start+1+end+1:]
	}
	// Remove images ![alt](url)
	for strings.Contains(s, "![") {
		start := strings.Index(s, "![")
		end := strings.Index(s[start:], ")")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	// Replace links [text](url) with text
	for strings.Contains(s, "](") {
		lb := strings.Index(s, "](")
		// find opening [
		ob := strings.LastIndex(s[:lb], "[")
		if ob == -1 {
			break
		}
		// find closing )
		cb := strings.Index(s[lb:], ")")
		if cb == -1 {
			break
		}
		text := s[ob+1 : lb]
		s = s[:ob] + text + s[lb+cb+1:]
	}
	// Remove heading markers, list markers, bold/italic markers
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// Strip heading markers
		for strings.HasPrefix(line, "#") {
			line = strings.TrimPrefix(line, "#")
		}
		// Strip list markers
		line = strings.TrimLeft(line, " ")
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "> ") {
			line = line[2:]
		}
		lines[i] = line
	}
	s = strings.Join(lines, " ")
	// Strip bold/italic markers
	s = strings.ReplaceAll(s, "***", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	// Collapse whitespace
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// DismissHandler handles POST /social/dismiss — admin dismisses a seeded thread
// and trains the filter to avoid similar content in future.
func DismissHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	threadID := r.FormValue("id")
	if threadID == "" {
		app.BadRequest(w, r, "Thread ID required")
		return
	}

	// Add to blocklist and flag
	DismissThread(threadID)
	moderation.AdminFlag("thread", threadID, "system")

	app.Log("social", "Admin dismissed thread %s", threadID)
	http.Redirect(w, r, "/social", http.StatusSeeOther)
}

func handleThread(w http.ResponseWriter, r *http.Request, id string) {
	mutex.RLock()
	t := getThread(id)
	mutex.RUnlock()

	if t == nil {
		http.NotFound(w, r)
		return
	}

	if moderation.IsHidden("thread", t.ID) {
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
		deleteBtn = app.DeleteButton("/social?id="+t.ID, "Delete", "Delete this thread?")
	}

	// Fact-check button for logged-in users
	var factCheckBtn string
	if acc != nil {
		factCheckBtn = renderFactCheckButton(t.ID)
	}

	sb.WriteString(fmt.Sprintf(`<div class="card">
<h2 style="margin-top:0;">%s</h2>
%s
<div>%s</div>
%s
</div>`, titleHTML,
		app.ItemMeta(app.Category(t.Topic, ""), app.AuthorLink(t.AuthorID, t.Author), app.Timestamp(t.CreatedAt), deleteBtn, factCheckBtn),
		contentHTML,
		renderCommunityNote(t.Note)))

	// Replies - render as a threaded tree
	if len(t.Replies) > 0 {
		sb.WriteString(app.Section("Replies"))
		// Build a map of parent -> children for threading
		childMap := map[string][]*Reply{}
		for _, reply := range t.Replies {
			childMap[reply.ParentID] = append(childMap[reply.ParentID], reply)
		}
		// Render top-level replies (ParentID == "") and their children recursively
		var renderReplies func(parentID string, depth int)
		renderReplies = func(parentID string, depth int) {
			children := childMap[parentID]
			for _, reply := range children {
				replyHTML := string(app.Render([]byte(reply.Content)))
				var replyDelete string
				if acc != nil && (acc.ID == reply.AuthorID || acc.Admin) {
					replyDelete = app.DeleteButton(
						fmt.Sprintf("/social?id=%s&reply=%s", t.ID, reply.ID),
						"Delete", "Delete this reply?")
				}
				var replyBtn string
				if acc != nil {
					replyBtn = app.ReplyLink(reply.ID)
				}
				indent := ""
				if depth > 0 {
					px := depth * 16
					if px > 64 {
						px = 64 // cap nesting depth visually
					}
					indent = fmt.Sprintf("margin-left:%dpx;", px)
				}
				sb.WriteString(fmt.Sprintf(`<div id="r-%s" class="card" style="padding:10px 16px;%s">`, reply.ID, indent))
				sb.WriteString(app.ItemMeta(app.AuthorLink(reply.AuthorID, reply.Author), app.Timestamp(reply.CreatedAt), replyBtn, replyDelete))
				sb.WriteString(fmt.Sprintf(`<div>%s</div>`, replyHTML))
				sb.WriteString(renderCommunityNote(reply.Note))
				if acc != nil {
					sb.WriteString(app.InlineReplyForm(reply.ID, "/social?id="+t.ID, "parent_id", reply.ID))
				}
				sb.WriteString(`</div>`)
				renderReplies(reply.ID, depth+1)
			}
		}
		renderReplies("", 0)
	}

	// Reply form for top-level replies
	if acc != nil {
		sb.WriteString(app.ReplyForm("/social?id="+t.ID, "Be respectful and stay on topic...", "", ""))
	} else {
		sb.WriteString(app.LoginPrompt("reply", "/social?id="+t.ID))
	}

	sb.WriteString(app.BackLink("Back to discussions", "/social"))

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
	go moderation.CheckContent("thread", thread.ID, thread.Title, thread.Content)

	// Fact-check in background
	go factCheckThread(thread.ID)

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

	var content, parentID string

	if app.SendsJSON(r) {
		var req struct {
			Content  string `json:"content"`
			ParentID string `json:"parent_id"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.BadRequest(w, r, "invalid json")
			return
		}
		content = strings.TrimSpace(req.Content)
		parentID = strings.TrimSpace(req.ParentID)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		content = strings.TrimSpace(r.FormValue("content"))
		parentID = strings.TrimSpace(r.FormValue("parent_id"))
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
		ParentID:  parentID,
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
	go moderation.CheckContent("thread", reply.ID, "", reply.Content)

	// Fact-check in background
	go factCheckReply(threadID, reply.ID)

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

// threadDeleter implements moderation.ContentDeleter for threads
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
