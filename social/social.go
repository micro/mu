package social

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/flag"
	"mu/news"
	"mu/wallet"
)

var mutex sync.RWMutex

// messages stored newest first
var messages []*Message

// cached HTML
var cardHTML string
var pageBodyHTML string

// startup throttle: suppress breaking threads for first 30 seconds after load
var loadedAt time.Time

// nitterInstance for fetching X/Twitter posts via Nitter (used by FetchExternalPost/context)
var nitterInstance = "nitter.poast.org"

// Message represents a message in a thread (or the thread-starting message itself)
type Message struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`     // display name
	AuthorID  string    `json:"author_id"`  // account ID
	Content   string    `json:"content"`
	ReplyTo   string    `json:"reply_to,omitempty"` // parent thread ID (empty = thread starter)
	PostedAt  time.Time `json:"posted_at"`
}

// addMessage adds a message to the feed (prepend, dedup, cap, save)
func addMessage(p *Message) {
	mutex.Lock()
	// Dedup by ID
	for _, existing := range messages {
		if existing.ID == p.ID {
			mutex.Unlock()
			return
		}
	}
	messages = append([]*Message{p}, messages...)
	if len(messages) > 500 {
		messages = messages[:500]
	}
	updateCacheLocked()
	mutex.Unlock()

	indexMessages([]*Message{p})
	save()

	event.Publish(event.Event{Type: "social_updated"})
}

func Load() {
	// Load saved messages (migrate from social_posts.json if needed)
	b, err := data.LoadFile("social.json")
	if err != nil {
		b, err = data.LoadFile("social_posts.json")
	}
	if err == nil {
		var cached []*Message
		if json.Unmarshal(b, &cached) == nil {
			mutex.Lock()
			messages = cached
			updateCacheLocked()
			mutex.Unlock()
			indexMessages(cached)
		}
	}

	// Remove system-generated news summary threads (not breaking news)
	cleaned := false
	mutex.Lock()
	var kept []*Message
	for _, m := range messages {
		if m.AuthorID == "_system" && m.Author != "Breaking" {
			cleaned = true
			continue
		}
		kept = append(kept, m)
	}
	if cleaned {
		messages = kept
		updateCacheLocked()
	}
	mutex.Unlock()
	if cleaned {
		save()
		app.Log("social", "Removed system-generated news summary threads")
	}

	loadedAt = time.Now()

	// Detect breaking stories — headlines reported by multiple sources
	go detectBreakingStories()

	app.Log("social", "Loaded %d messages", len(messages))
}

// detectBreakingStories checks the news feed for stories covered by multiple
// categories/sources. If the same story appears across 2+ sources, it's
// significant enough to surface as a social thread.
func detectBreakingStories() {
	// Wait for news to load first
	time.Sleep(3 * time.Minute)

	for {
		surfaceBreakingFromNews()
		time.Sleep(time.Hour)
	}
}

func surfaceBreakingFromNews() {
	feed := news.GetFeed()
	if len(feed) == 0 {
		return
	}

	// Only consider articles from the last 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)

	// Extract keywords from each recent article
	type article struct {
		title    string
		url      string
		category string
		words    map[string]bool
	}
	var recent []article

	for _, p := range feed {
		if p.PostedAt.Before(cutoff) {
			continue
		}
		words := extractKeywords(p.Title)
		if len(words) < 2 {
			continue
		}
		recent = append(recent, article{
			title:    p.Title,
			url:      p.URL,
			category: p.Category,
			words:    words,
		})
	}

	// Find articles from different categories that share 2+ keywords
	surfaced := map[string]bool{}
	for i, a := range recent {
		for j := i + 1; j < len(recent); j++ {
			b := recent[j]
			if a.category == b.category {
				continue
			}
			// Count shared keywords
			shared := 0
			for w := range a.words {
				if b.words[w] {
					shared++
				}
			}
			if shared >= 2 {
				// Surface the first one (use URL as dedup key)
				if !surfaced[a.url] {
					surfaced[a.url] = true
					SurfaceBreaking(a.title, a.url)
					app.Log("social", "Breaking: %q matched across %s and %s", a.title, a.category, b.category)
				}
			}
		}
	}
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"has": true, "have": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "its": true, "it": true,
	"that": true, "this": true, "as": true, "not": true, "no": true,
	"new": true, "says": true, "said": true, "after": true, "over": true,
	"into": true, "up": true, "out": true, "about": true, "than": true,
	"how": true, "what": true, "when": true, "where": true, "who": true,
	"why": true, "more": true, "been": true, "being": true, "just": true,
}

// extractKeywords pulls significant words from a headline
func extractKeywords(title string) map[string]bool {
	title = strings.ToLower(title)
	title = regexp.MustCompile(`[^a-z0-9\s]`).ReplaceAllString(title, "")

	words := map[string]bool{}
	for _, w := range strings.Fields(title) {
		if !stopWords[w] && len(w) > 2 {
			words[w] = true
		}
	}
	return words
}

// SurfaceBreaking creates a system thread from external sources (e.g., breaking news)
func SurfaceBreaking(title, link string) {
	if title == "" {
		return
	}
	content := title
	if link != "" {
		content += " " + link
	}
	if len(content) > 500 {
		content = content[:497] + "..."
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte("breaking:"+link)))[:16]

	addMessage(&Message{
		ID:       id,
		Author:   "Breaking",
		AuthorID: "_system",
		Content:  content,
		PostedAt: time.Now(),
	})
}

func save() error {
	mutex.RLock()
	p := make([]*Message, len(messages))
	copy(p, messages)
	mutex.RUnlock()
	return data.SaveJSON("social.json", p)
}

// updateCacheLocked regenerates cached HTML. Caller must hold mutex write lock.
func updateCacheLocked() {
	cardHTML = generateCardHTML(messages)
	pageBodyHTML = "" // invalidate, regenerated on next request
}

// CardHTML returns cached dashboard card HTML
func CardHTML() string {
	mutex.RLock()
	defer mutex.RUnlock()
	return cardHTML
}

// GetThreads returns all cached messages (most recent first)
func GetThreads() []*Message {
	mutex.RLock()
	defer mutex.RUnlock()
	result := make([]*Message, len(messages))
	copy(result, messages)
	return result
}

// getMessage returns a message by ID. Caller must hold read lock.
func getMessage(id string) *Message {
	for _, p := range messages {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// replyCount returns the number of messages in a thread. Caller must hold read lock.
func replyCount(threadID string) int {
	count := 0
	for _, p := range messages {
		if p.ReplyTo == threadID {
			count++
		}
	}
	return count
}

// getReplies returns messages in a thread in chronological order (oldest first). Caller must hold read lock.
func getReplies(threadID string) []*Message {
	var replies []*Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ReplyTo == threadID {
			replies = append(replies, messages[i])
		}
	}
	return replies
}

// Handler serves the /social endpoint
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if app.SendsJSON(r) {
			// JSON POST could be search or create
			handleJSONRequest(w, r)
			return
		}
		handleCreateThread(w, r)
		return
	case "DELETE":
		handleDeleteMessage(w, r)
		return
	}

	// GET
	if query := r.URL.Query().Get("query"); query != "" {
		_, acc := auth.TrySession(r)
		if acc == nil {
			app.Unauthorized(w, r)
			return
		}
		if len(query) > 256 {
			app.BadRequest(w, r, "Search query must not exceed 256 characters")
			return
		}
		handleSearch(w, r, query)
		return
	}

	handleGetFeed(w, r)
}

func handleCreateThread(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	if !auth.CanPost(acc.ID) {
		app.BadRequest(w, r, "Your account is too new to start a thread. Please wait a bit.")
		return
	}

	if err := r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Failed to parse form")
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		app.BadRequest(w, r, "Content is required")
		return
	}
	if len(content) > 500 {
		app.BadRequest(w, r, "Messages must be 500 characters or less")
		return
	}
	if len(strings.Fields(content)) < 2 {
		app.BadRequest(w, r, "Message must contain at least 2 words")
		return
	}

	threadID := fmt.Sprintf("%d", time.Now().UnixNano())

	p := &Message{
		ID:       threadID,
		Author:   acc.Name,
		AuthorID: acc.ID,
		Content:  content,
		PostedAt: time.Now(),
	}

	addMessage(p)

	// Async content moderation
	go flag.CheckContent("social", threadID, "", content)

	app.Log("social", "New thread by %s (%s)", acc.Name, acc.ID)

	if app.SendsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true, "id": threadID})
		return
	}
	http.Redirect(w, r, "/social", http.StatusSeeOther)
}

func handleJSONRequest(w http.ResponseWriter, r *http.Request) {
	var reqData map[string]interface{}
	b, _ := ioutil.ReadAll(r.Body)
	json.Unmarshal(b, &reqData)

	// If it has a "query" field, it's a search
	if q, ok := reqData["query"]; ok && q != nil {
		query := fmt.Sprintf("%v", q)
		if query == "" {
			http.Error(w, "query required", 400)
			return
		}
		handleAPISearch(w, r, query)
		return
	}

	// Otherwise it's a create thread
	content := ""
	if v, ok := reqData["content"]; ok && v != nil {
		content = strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	if content == "" {
		http.Error(w, "content required", 400)
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	if !auth.CanPost(acc.ID) {
		http.Error(w, "Account too new to start threads", http.StatusForbidden)
		return
	}

	if len(content) > 500 {
		http.Error(w, "Messages must be 500 characters or less", 400)
		return
	}

	threadID := fmt.Sprintf("%d", time.Now().UnixNano())
	p := &Message{
		ID:       threadID,
		Author:   acc.Name,
		AuthorID: acc.ID,
		Content:  content,
		PostedAt: time.Now(),
	}

	addMessage(p)

	go flag.CheckContent("social", threadID, "", content)

	app.RespondJSON(w, map[string]interface{}{"success": true, "id": threadID})
}

func handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	threadID := r.URL.Query().Get("id")
	if threadID == "" {
		app.BadRequest(w, r, "Thread ID required")
		return
	}

	mutex.Lock()
	found := false
	for i, p := range messages {
		if p.ID == threadID {
			// Only author or admin can delete
			if p.AuthorID != acc.ID && !acc.Admin {
				mutex.Unlock()
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			messages = append(messages[:i], messages[i+1:]...)
			found = true
			break
		}
	}
	if found {
		updateCacheLocked()
	}
	mutex.Unlock()

	if !found {
		http.Error(w, "Thread not found", 404)
		return
	}

	save()

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true})
		return
	}
	http.Redirect(w, r, "/social", http.StatusSeeOther)
}

func handleGetFeed(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	all := make([]*Message, len(messages))
	copy(all, messages)
	mutex.RUnlock()

	// Filter out flagged messages and replies (only show threads in feed)
	var visible []*Message
	for _, p := range all {
		if p.ReplyTo != "" {
			continue
		}
		if !flag.IsHidden("social", p.ID) {
			visible = append(visible, p)
		}
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"threads": visible})
		return
	}

	body := generatePageHTML(visible, r)

	app.Respond(w, r, app.Response{
		Title:       "Social",
		Description: "Threads and conversations",
		HTML:        body,
	})
}

// ThreadHandler serves the /social/thread endpoint — shows a thread and its messages
func ThreadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		handleCreateReply(w, r)
		return
	}

	threadID := r.URL.Query().Get("id")
	if threadID == "" {
		http.Redirect(w, r, "/social", http.StatusFound)
		return
	}

	mutex.RLock()
	p := getMessage(threadID)
	if p == nil {
		mutex.RUnlock()
		http.Error(w, "Thread not found", 404)
		return
	}
	// If this is a reply, redirect to the parent thread
	if p.ReplyTo != "" {
		mutex.RUnlock()
		http.Redirect(w, r, "/social/thread?id="+p.ReplyTo, http.StatusFound)
		return
	}
	replies := getReplies(threadID)
	mutex.RUnlock()

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"thread": p, "messages": replies})
		return
	}

	body := generateThreadHTML(p, replies, r)

	app.Respond(w, r, app.Response{
		Title:       "Thread by " + p.Author,
		Description: truncate(p.Content, 160),
		HTML:        body,
	})
}

func handleCreateReply(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	if !auth.CanPost(acc.ID) {
		app.BadRequest(w, r, "Your account is too new to send messages. Please wait a bit.")
		return
	}

	if err := r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Failed to parse form")
		return
	}

	parentID := r.FormValue("reply_to")
	content := strings.TrimSpace(r.FormValue("content"))

	if parentID == "" {
		app.BadRequest(w, r, "Missing thread")
		return
	}
	if content == "" {
		app.BadRequest(w, r, "Message cannot be empty")
		return
	}
	if len(content) > 500 {
		app.BadRequest(w, r, "Messages must be 500 characters or less")
		return
	}

	// Verify parent exists
	mutex.RLock()
	parent := getMessage(parentID)
	mutex.RUnlock()
	if parent == nil {
		app.BadRequest(w, r, "Thread not found")
		return
	}

	replyID := fmt.Sprintf("%d", time.Now().UnixNano())
	reply := &Message{
		ID:       replyID,
		Author:   acc.Name,
		AuthorID: acc.ID,
		Content:  content,
		ReplyTo:  parentID,
		PostedAt: time.Now(),
	}

	addMessage(reply)

	go flag.CheckContent("social", replyID, "", content)

	app.Log("social", "Message by %s in thread %s", acc.Name, parentID)

	if app.SendsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true, "id": replyID})
		return
	}
	http.Redirect(w, r, "/social/thread?id="+parentID, http.StatusSeeOther)
}

func generateThreadHTML(p *Message, replies []*Message, r *http.Request) string {
	var sb strings.Builder

	// Back link
	sb.WriteString(`<div style="margin-bottom:16px;"><a href="/social" style="color:#888;text-decoration:none;">&larr; Back to threads</a></div>`)

	// Original message (full, no truncation)
	content := htmlpkg.EscapeString(p.Content)
	firstURL := extractFirstURL(content)
	linkCard := ""
	if firstURL != "" {
		linkCard = renderLinkCard(firstURL)
		if linkCard != "" {
			escapedURL := htmlpkg.EscapeString(firstURL)
			content = strings.TrimSpace(strings.Replace(content, escapedURL, "", 1))
		}
	}
	content = linkifyURLs(content)

	_, acc := auth.TrySession(r)

	canDelete := acc != nil && (acc.ID == p.AuthorID || acc.Admin)
	deleteBtn := ""
	if canDelete {
		deleteBtn = fmt.Sprintf(` <button onclick="if(confirm('Delete this thread?')){fetch('/social?id=%s',{method:'DELETE'}).then(()=>location.href='/social')}" style="background:none;border:none;color:#ccc;cursor:pointer;font-size:12px;padding:0;" title="Delete">x</button>`, p.ID)
	}

	ts := p.PostedAt.Unix()
	sb.WriteString(fmt.Sprintf(`<div class="headline" style="border-bottom:2px solid #eee;">
  <div style="display:flex;justify-content:space-between;align-items:baseline;">
    <div><b>%s</b></div>
    <div><span data-timestamp="%d" style="color:#888;font-size:12px;">%s</span>%s</div>
  </div>
  <div style="margin-top:8px;font-size:15px;line-height:1.5;overflow-wrap:break-word;word-break:break-word;">%s</div>%s
</div>`,
		htmlpkg.EscapeString(p.Author),
		ts,
		app.TimeAgo(p.PostedAt),
		deleteBtn,
		content,
		linkCard,
	))

	// Message count
	msgLabel := "messages"
	if len(replies) == 1 {
		msgLabel = "message"
	}
	if len(replies) > 0 {
		sb.WriteString(fmt.Sprintf(`<div style="padding:12px 0;color:#888;font-size:13px;border-bottom:1px solid #f0f0f0;">%d %s</div>`, len(replies), msgLabel))
	}

	// Reply form (for logged-in users)
	if acc != nil {
		sb.WriteString(fmt.Sprintf(`<div style="margin:16px 0;">
  <form method="POST" action="/social/thread" id="reply-form">
    <input type="hidden" name="reply_to" value="%s">
    <textarea name="content" id="reply-content" rows="2" placeholder="Write a message..." required
      style="width:100%%;box-sizing:border-box;padding:10px;border:1px solid #ddd;border-radius:8px;font-family:inherit;font-size:14px;resize:vertical;"></textarea>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-top:8px;">
      <span id="reply-char-count" style="font-size:12px;color:#888;">0/500</span>
      <button type="submit" style="padding:6px 16px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-family:inherit;">Send</button>
    </div>
  </form>
  <script>
    var ta=document.getElementById('reply-content'),cc=document.getElementById('reply-char-count');
    ta.addEventListener('input',function(){
      var n=ta.value.length;
      cc.textContent=n+'/500';
      cc.style.color=n>500?'red':'#888';
    });
  </script>
</div>`, p.ID))
	} else {
		sb.WriteString(`<div style="margin:16px 0;padding:12px;background:#f9f9f9;border-radius:8px;text-align:center;">
  <a href="/login" style="color:#000;font-weight:bold;">Log in</a> to join the conversation
</div>`)
	}

	// Messages (chronological — oldest first, so conversation reads naturally)
	for _, reply := range replies {
		if flag.IsHidden("social", reply.ID) {
			continue
		}
		rc := htmlpkg.EscapeString(reply.Content)
		rc = linkifyURLs(rc)

		canDeleteReply := acc != nil && (acc.ID == reply.AuthorID || acc.Admin)
		rDeleteBtn := ""
		if canDeleteReply {
			rDeleteBtn = fmt.Sprintf(` <button onclick="if(confirm('Delete this message?')){fetch('/social?id=%s',{method:'DELETE'}).then(()=>location.reload())}" style="background:none;border:none;color:#ccc;cursor:pointer;font-size:12px;padding:0;" title="Delete">x</button>`, reply.ID)
		}

		rts := reply.PostedAt.Unix()
		sb.WriteString(fmt.Sprintf(`<div style="padding:12px 0;border-bottom:1px solid #f5f5f5;">
  <div style="display:flex;justify-content:space-between;align-items:baseline;">
    <div style="font-size:13px;"><b>%s</b></div>
    <div><span data-timestamp="%d" style="color:#888;font-size:12px;">%s</span>%s</div>
  </div>
  <div style="margin-top:4px;overflow-wrap:break-word;word-break:break-word;">%s</div>
</div>`,
			htmlpkg.EscapeString(reply.Author),
			rts,
			app.TimeAgo(reply.PostedAt),
			rDeleteBtn,
			rc,
		))
	}

	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func handleAPISearch(w http.ResponseWriter, r *http.Request, query string) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpSocialSearch)
	if !canProceed {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":          "insufficient credits",
			"credits_needed": cost,
		})
		return
	}

	wallet.ConsumeQuota(sess.Account, wallet.OpSocialSearch)

	results := data.Search(query, 50)
	var socialResults []map[string]interface{}
	for _, entry := range results {
		if entry.Type == "social" {
			socialResults = append(socialResults, map[string]interface{}{
				"title":    entry.Title,
				"content":  entry.Content,
				"metadata": entry.Metadata,
			})
		}
	}

	app.RespondJSON(w, map[string]interface{}{
		"query":   query,
		"results": socialResults,
	})
}

func handleSearch(w http.ResponseWriter, r *http.Request, query string) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.Unauthorized(w, r)
		return
	}

	canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpSocialSearch)
	if !canProceed {
		content := wallet.QuotaExceededPage(wallet.OpSocialSearch, cost)
		app.Respond(w, r, app.Response{
			Title: "Social - Search",
			HTML:  content,
		})
		return
	}

	wallet.ConsumeQuota(sess.Account, wallet.OpSocialSearch)

	results := data.Search(query, 50)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<h4>Results for "%s"</h4>`, htmlpkg.EscapeString(query)))

	count := 0
	for _, entry := range results {
		if entry.Type != "social" {
			continue
		}
		count++
		content := entry.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  <div><b>%s</b></div>
  <div style="margin-top:4px;font-size:13px;">%s</div>
</div>`, htmlpkg.EscapeString(entry.Title), htmlpkg.EscapeString(content)))
	}

	if count == 0 {
		sb.WriteString(`<p style="color:#888;">No results found</p>`)
	}

	app.Respond(w, r, app.Response{
		Title: "Social - Search",
		HTML:  sb.String(),
	})
}

func indexMessages(toIndex []*Message) {
	for _, p := range toIndex {
		data.Index(
			"social_"+p.ID,
			"social",
			p.Author,
			p.Content,
			map[string]interface{}{
				"author_id": p.AuthorID,
				"posted_at": p.PostedAt,
			},
		)
	}
}

func generateCardHTML(allMessages []*Message) string {
	if len(allMessages) == 0 {
		return `<p style="color:#888;">No threads yet. Be the first to start one.</p>`
	}

	// Show up to 4 latest threads, one per author for variety
	// Limit breaking threads to at most 1 on the home card
	seen := map[string]bool{}
	breakingCount := 0
	var selected []*Message
	for _, p := range allMessages {
		if p.ReplyTo != "" {
			continue // skip replies in home card
		}
		if flag.IsHidden("social", p.ID) {
			continue
		}
		if p.AuthorID == "_system" {
			breakingCount++
			if breakingCount > 1 {
				continue
			}
		}
		if seen[p.AuthorID] && p.AuthorID != "_system" {
			continue
		}
		seen[p.AuthorID] = true
		selected = append(selected, p)
		if len(selected) >= 4 {
			break
		}
	}

	var sb strings.Builder
	for _, p := range selected {
		content := htmlpkg.EscapeString(p.Content)

		// Check for link card
		firstURL := extractFirstURL(content)
		linkCard := ""
		if firstURL != "" {
			linkCard = renderLinkCard(firstURL)
			// Remove the URL from displayed text if we have a card
			if linkCard != "" {
				escapedURL := htmlpkg.EscapeString(firstURL)
				content = strings.TrimSpace(strings.Replace(content, escapedURL, "", 1))
			}
		}

		if len(content) > 120 && linkCard != "" {
			content = content[:120] + "..."
		} else if len(content) > 200 {
			content = content[:200] + "..."
		}

		rc := replyCount(p.ID)
		replyInfo := ""
		if rc > 0 {
			noun := "messages"
			if rc == 1 {
				noun = "message"
			}
			replyInfo = fmt.Sprintf(` · <a href="/social/thread?id=%s" style="color:#888;text-decoration:none;">%d %s</a>`, p.ID, rc, noun)
		}

		ts := p.PostedAt.Unix()
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  <a href="/social/thread?id=%s">
    <span class="title">%s</span>
  </a>
  <span class="description" style="overflow-wrap:break-word;word-break:break-word;">%s</span>%s
  <div class="summary"><span data-timestamp="%d">%s</span>%s</div>
</div>`,
			p.ID,
			htmlpkg.EscapeString(p.Author),
			content,
			linkCard,
			ts,
			app.TimeAgo(p.PostedAt),
			replyInfo,
		))
	}

	return sb.String()
}

func generatePageHTML(visible []*Message, r *http.Request) string {
	var sb strings.Builder

	// Compose box (shown to logged-in users)
	_, acc := auth.TrySession(r)
	if acc != nil {
		sb.WriteString(`<div style="margin-bottom:20px;">
  <form method="POST" action="/social" id="social-form">
    <textarea name="content" id="social-content" rows="3" placeholder="Start a thread..." required
      style="width:100%;box-sizing:border-box;padding:10px;border:1px solid #ddd;border-radius:8px;font-family:inherit;font-size:14px;resize:vertical;"></textarea>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-top:8px;">
      <span id="social-char-count" style="font-size:12px;color:#888;">0/500</span>
      <button type="submit" style="padding:8px 20px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-family:inherit;">Start Thread</button>
    </div>
  </form>
  <script>
    var ta=document.getElementById('social-content'),cc=document.getElementById('social-char-count');
    ta.addEventListener('input',function(){
      var n=ta.value.length;
      cc.textContent=n+'/500';
      cc.style.color=n>500?'red':'#888';
    });
  </script>
</div>`)
	} else {
		sb.WriteString(`<div style="margin-bottom:20px;padding:16px;background:#f9f9f9;border-radius:8px;text-align:center;">
  <a href="/login" style="color:#000;font-weight:bold;">Log in</a> to start a thread
</div>`)
	}

	if len(visible) == 0 {
		sb.WriteString(`<p style="color:#888;">No threads yet. Be the first to start one.</p>`)
		return sb.String()
	}

	for _, p := range visible {
		content := htmlpkg.EscapeString(p.Content)

		// Extract first URL for card rendering, then linkify remaining
		firstURL := extractFirstURL(content)
		linkCard := ""
		if firstURL != "" {
			linkCard = renderLinkCard(firstURL)
			// If we have a rich card, remove the URL from text
			if linkCard != "" {
				escapedURL := htmlpkg.EscapeString(firstURL)
				content = strings.TrimSpace(strings.Replace(content, escapedURL, "", 1))
			}
		}

		// Linkify any remaining URLs in content
		content = linkifyURLs(content)

		canDelete := acc != nil && (acc.ID == p.AuthorID || acc.Admin)
		deleteBtn := ""
		if canDelete {
			deleteBtn = fmt.Sprintf(` <button onclick="if(confirm('Delete this thread?')){fetch('/social?id=%s',{method:'DELETE'}).then(()=>location.reload())}" style="background:none;border:none;color:#ccc;cursor:pointer;font-size:12px;padding:0;" title="Delete">x</button>`, p.ID)
		}

		// Message count
		mutex.RLock()
		rc := replyCount(p.ID)
		mutex.RUnlock()
		replyLink := fmt.Sprintf(`<a href="/social/thread?id=%s" style="color:#888;text-decoration:none;font-size:12px;">open thread</a>`, p.ID)
		if rc > 0 {
			noun := "messages"
			if rc == 1 {
				noun = "message"
			}
			replyLink = fmt.Sprintf(`<a href="/social/thread?id=%s" style="color:#888;text-decoration:none;font-size:12px;">%d %s</a>`, p.ID, rc, noun)
		}

		ts := p.PostedAt.Unix()
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  <div style="display:flex;justify-content:space-between;align-items:baseline;">
    <div><b>%s</b></div>
    <div><span data-timestamp="%d" style="color:#888;font-size:12px;">%s</span>%s</div>
  </div>
  <div style="margin-top:4px;overflow-wrap:break-word;word-break:break-word;">%s</div>%s
  <div style="margin-top:6px;">%s</div>
</div>`,
			htmlpkg.EscapeString(p.Author),
			ts,
			app.TimeAgo(p.PostedAt),
			deleteBtn,
			content,
			linkCard,
			replyLink,
		))
	}

	return sb.String()
}

var urlRegex = regexp.MustCompile(`https?://[^\s<>"]+`)

// extractURLFromEscaped pulls a URL from HTML-escaped text, unescaping &amp; back to &
func extractURLFromEscaped(u string) (href, display string) {
	href = strings.ReplaceAll(u, "&amp;", "&")
	parsed, err := url.Parse(href)
	if err != nil {
		return href, href
	}
	domain := parsed.Hostname()
	// Truncated display: domain + short path
	path := parsed.Path
	if len(path) > 30 {
		path = path[:27] + "..."
	}
	display = domain + path
	if parsed.RawQuery != "" {
		display = domain + path + "?..."
	}
	return href, display
}

func linkifyURLs(escaped string) string {
	return urlRegex.ReplaceAllStringFunc(escaped, func(u string) string {
		href, display := extractURLFromEscaped(u)
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" style="color:#06c;word-break:break-all;">%s</a>`, htmlpkg.EscapeString(href), htmlpkg.EscapeString(display))
	})
}

// renderLinkCard renders a Twitter-style embed card for a URL using cached OG metadata
func renderLinkCard(rawURL string) string {
	md, ok := news.LookupMetadata(rawURL)
	if !ok || (md.Title == "" && md.Description == "") {
		// Fallback: simple domain card
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return ""
		}
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" style="display:block;border:1px solid #e1e1e1;border-radius:12px;padding:12px;margin-top:8px;text-decoration:none;color:inherit;">
  <div style="font-size:12px;color:#888;">%s</div>
</a>`, htmlpkg.EscapeString(rawURL), htmlpkg.EscapeString(parsed.Hostname()))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" style="display:block;border:1px solid #e1e1e1;border-radius:12px;overflow:hidden;margin-top:8px;text-decoration:none;color:inherit;">`, htmlpkg.EscapeString(rawURL)))

	if md.Image != "" {
		sb.WriteString(fmt.Sprintf(`<div style="width:100%%;background:#f5f5f5;"><img src="%s" style="width:100%%;max-height:200px;object-fit:cover;display:block;" loading="lazy" onerror="this.parentElement.style.display='none'"></div>`, htmlpkg.EscapeString(md.Image)))
	}

	sb.WriteString(`<div style="padding:10px 12px;">`)

	site := md.Site
	if site == "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			site = parsed.Hostname()
		}
	}
	if site != "" {
		sb.WriteString(fmt.Sprintf(`<div style="font-size:12px;color:#888;margin-bottom:2px;">%s</div>`, htmlpkg.EscapeString(site)))
	}

	if md.Title != "" {
		title := md.Title
		if len(title) > 100 {
			title = title[:97] + "..."
		}
		sb.WriteString(fmt.Sprintf(`<div style="font-size:14px;font-weight:600;line-height:1.3;">%s</div>`, htmlpkg.EscapeString(title)))
	}

	if md.Description != "" {
		desc := md.Description
		if len(desc) > 150 {
			desc = desc[:147] + "..."
		}
		sb.WriteString(fmt.Sprintf(`<div style="font-size:13px;color:#666;margin-top:4px;line-height:1.4;">%s</div>`, htmlpkg.EscapeString(desc)))
	}

	sb.WriteString(`</div></a>`)
	return sb.String()
}

// extractFirstURL returns the first URL found in text (unescaped)
func extractFirstURL(text string) string {
	re := regexp.MustCompile(`https?://[^\s<>"]+`)
	match := re.FindString(text)
	return strings.ReplaceAll(match, "&amp;", "&")
}

// firstSentences extracts the first n sentences from text
func firstSentences(text string, n int) string {
	text = strings.TrimSpace(text)
	count := 0
	for i, r := range text {
		if r == '.' || r == '!' || r == '?' {
			count++
			if count >= n {
				return strings.TrimSpace(text[:i+1])
			}
		}
	}
	// If fewer sentences than n, return the whole text
	if len(text) > 280 {
		return text[:277] + "..."
	}
	return text
}


// stripHTML removes HTML tags from a string
func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(s, " ")
	re2 := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re2.ReplaceAllString(text, " "))
}

// DetectSocialURLs finds social media URLs in text content
func DetectSocialURLs(content string) []string {
	re := regexp.MustCompile(`https?://(?:(?:www\.)?(?:twitter\.com|x\.com)|(?:truthsocial\.com))/[^\s"'<>\])+]+`)
	matches := re.FindAllString(content, -1)

	seen := map[string]bool{}
	var unique []string
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;:!?)")
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

// FetchExternalPost fetches a single social media post by URL (used by context.go for news)
func FetchExternalPost(rawURL string) (*Message, error) {
	fetchURL := rawURL
	parsed, err := url.Parse(rawURL)
	if err == nil {
		host := strings.ToLower(parsed.Hostname())
		if host == "twitter.com" || host == "www.twitter.com" ||
			host == "x.com" || host == "www.x.com" ||
			host == "mobile.twitter.com" || host == "mobile.x.com" {
			parsed.Host = nitterInstance
			parsed.Scheme = "https"
			fetchURL = parsed.String()
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fetchURL)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	text := stripHTML(string(body))
	if len(text) > 1000 {
		text = text[:1000] + "..."
	}

	handle := ""
	if parsed != nil && len(parsed.Path) > 1 {
		parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(parts) > 0 {
			handle = strings.TrimPrefix(parts[0], "@")
		}
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte(rawURL)))[:16]

	return &Message{
		ID:       id,
		Author:   handle,
		AuthorID: handle,
		Content:  text,
		PostedAt: time.Now(),
	}, nil
}
