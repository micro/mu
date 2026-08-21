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
	"mu/internal/imageproxy"
	"mu/internal/linkmeta"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/snapshot"
)

// cardSnap is the go-micro read-plane channel for the social card (store +
// broker); see internal/snapshot and docs/GO_MICRO_ARCHITECTURE.md.
var cardSnap *snapshot.Snapshot

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
	ID       string    `json:"id"`
	Author   string    `json:"author"`    // display name
	AuthorID string    `json:"author_id"` // account ID
	Content  string    `json:"content"`
	ReplyTo  string    `json:"reply_to,omitempty"` // parent thread ID (empty = thread starter)
	PostedAt time.Time `json:"posted_at"`
}

// addMessage adds a message to the feed (prepend, dedup, cap, save)
// addMessage stores a message and reports whether it was new. The dedupe is by
// id, so a story reaching two sources is one message — and callers that
// announce it need to know which of the two happened.
func addMessage(p *Message) bool {
	mutex.Lock()
	// Dedup by ID
	for _, existing := range messages {
		if existing.ID == p.ID {
			mutex.Unlock()
			return false
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
	return true
}

func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("social", "service register failed: %v", err)
	}

	// Read plane: start the snapshot channel before the cache is first built
	// (below) so the rebuild publishes to the go-micro store + broker.
	cardSnap = snapshot.New("social")

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

	loadedAt = time.Now()

	// Detect breaking stories — headlines reported by multiple sources

	app.Log("social", "Loaded %d messages", len(messages))
}

// SurfaceBreaking creates a system thread from external sources (e.g., breaking news).
// The category is used as the author name (e.g. "Politics", "Finance").
func SurfaceBreaking(category, title, link string) {
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

	// The link identifies the story, so the same story from two sources is one
	// message. When there is no link the title has to identify it — keying every
	// link-less item on the same empty string made them all one message, and
	// exactly one of them would ever have appeared. Nothing surfaced here was
	// ever link-less until the network watcher started passing posts that carry
	// only an image.
	key := link
	if key == "" {
		key = title
	}
	id := fmt.Sprintf("%x", md5.Sum([]byte("breaking:"+key)))[:16]

	if addMessage(&Message{
		ID:       id,
		Author:   category,
		AuthorID: "_system",
		Content:  content,
		PostedAt: time.Now(),
	}) {
		event.Announce("social", category+": "+title, link, "")
	}
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

	// Publish the rebuilt card snapshot to the go-micro store + broker; runs
	// under the caller's write lock (nil-safe before Load wires cardSnap).
	cardSnap.Publish(cardHTML)
}

// CardHTML returns cached dashboard card HTML
func CardHTML() string {
	// Serve the broker-fed snapshot mirror (go-micro read plane); fall back to
	// the locally-cached HTML if no snapshot has arrived yet.
	if s := cardSnap.Get(); s != "" {
		return s
	}
	mutex.RLock()
	defer mutex.RUnlock()
	return cardHTML
}

// CountSince returns the number of messages (threads + replies) posted
// after the given timestamp. Used by the /updates endpoint.
func CountSince(since time.Time) int {
	mutex.RLock()
	defer mutex.RUnlock()
	count := 0
	for _, p := range messages {
		if p.PostedAt.After(since) {
			count++
		}
	}
	return count
}

// Threads returns all cached messages (most recent first)
func Threads() []*Message {
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

// replyCounts is every thread's message count, in one pass.
//
// The feed asked replyCount per row, and replyCount walks every message there
// is — so drawing the feed was rows × messages, both of which only ever grow,
// and it took the lock once per row to do it. One pass, one lock.
//
// Caller must hold the read lock.
func replyCounts() map[string]int {
	counts := make(map[string]int)
	for _, p := range messages {
		if p.ReplyTo != "" {
			counts[p.ReplyTo]++
		}
	}
	return counts
}

// feedPerPage is how many threads one page of the feed is.
const feedPerPage = 25

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

	// Support _method=DELETE from POST forms
	if r.Method == "POST" && r.FormValue("_method") == "DELETE" {
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
	counts := replyCounts()
	mutex.RUnlock()

	// Filter out flagged/banned messages and replies (only show threads in feed)
	var visible []*Message
	for _, p := range all {
		if p.ReplyTo != "" {
			continue
		}
		if flag.IsHidden("social", p.ID) || auth.IsBanned(p.AuthorID) {
			continue
		}
		visible = append(visible, p)
	}

	// And what this viewer has hidden or blocked, before the page is cut rather
	// than after — otherwise a page of twenty-five shows however many of them
	// survive, and the count on the pager is a number about somebody else's
	// feed. This was inside the rendering loop, which is why it had to be.
	if _, acc := auth.TrySession(r); acc != nil {
		var kept []*Message
		for _, p := range visible {
			if app.IsBlocked(acc.ID, p.AuthorID) || app.IsDismissed(acc.ID, "social", p.ID) {
				continue
			}
			kept = append(kept, p)
		}
		visible = kept
	}

	pager := app.Paginate(r, len(visible), feedPerPage)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"threads": visible[pager.From:pager.To]})
		return
	}

	body := generatePageHTML(visible[pager.From:pager.To], counts, pager.Nav("/social"), r)

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
	sb.WriteString(`<div class="col-narrow">`)

	// Back link
	sb.WriteString(`<div class="mb-4"><a href="/social" class="text-muted no-underline">&larr; Back to threads</a></div>`)

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
	var threadUserID string
	var threadIsAdmin bool
	if acc != nil {
		threadUserID = acc.ID
		threadIsAdmin = acc.Admin
	}
	controls := app.ItemControls(threadUserID, threadIsAdmin, "social", p.ID, p.AuthorID, "", "/social?id="+p.ID)

	ts := p.PostedAt.Unix()
	threadAuthorHTML := fmt.Sprintf(`<b>%s</b>`, htmlpkg.EscapeString(p.Author))
	if p.AuthorID == "_system" {
		threadAuthorHTML = fmt.Sprintf(`<span class="category">%s</span>`, htmlpkg.EscapeString(p.Author))
	}
	sb.WriteString(fmt.Sprintf(`<div class="headline so-rule">
  %s
  <div class="d-flex between so-head">
    <div>%s</div>
    <div><span data-timestamp="%d" class="text-muted text-sm">%s</span></div>
  </div>
  <div class="mt-2 so-body breakable">%s</div>%s
</div>`,
		controls,
		threadAuthorHTML,
		ts,
		app.TimeAgo(p.PostedAt),
		content,
		linkCard,
	))

	// Message count
	msgLabel := "messages"
	if len(replies) == 1 {
		msgLabel = "message"
	}
	if len(replies) > 0 {
		sb.WriteString(fmt.Sprintf(`<div class="feed-row text-muted text-sm">%d %s</div>`, len(replies), msgLabel))
	}

	// Reply form (for logged-in users)
	if acc != nil {
		sb.WriteString(fmt.Sprintf(`<div class="my-4">
  <form method="POST" action="/social/thread" id="reply-form">
    <input type="hidden" name="reply_to" value="%s">
    <textarea name="content" id="reply-content" rows="2" placeholder="Write a message..." required
      class="form-area"></textarea>
    <div class="d-flex between items-center mt-2">
      <span id="reply-char-count" class="text-sm text-muted">0/500</span>
      <button type="submit" class="btn">Send</button>
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
		sb.WriteString(`<div class="panel">
  <a href="/login" class="so-name">Log in</a> to join the conversation
</div>`)
	}

	// Messages (chronological — oldest first, so conversation reads naturally)
	for _, reply := range replies {
		if flag.IsHidden("social", reply.ID) || auth.IsBanned(reply.AuthorID) {
			continue
		}
		rc := htmlpkg.EscapeString(reply.Content)
		rc = linkifyURLs(rc)

		replyControls := app.ItemControls(threadUserID, threadIsAdmin, "social", reply.ID, reply.AuthorID, "", "/social?id="+reply.ID)

		rts := reply.PostedAt.Unix()
		sb.WriteString(fmt.Sprintf(`<div class="feed-row">
  <div class="d-flex between so-head">
    <div class="text-sm"><b>%s</b></div>
    <div><span data-timestamp="%d" class="text-muted text-sm">%s</span>%s</div>
  </div>
  <div class="mt-1 breakable">%s</div>
</div>`,
			htmlpkg.EscapeString(reply.Author),
			rts,
			app.TimeAgo(reply.PostedAt),
			replyControls,
			rc,
		))
	}

	sb.WriteString(`</div>`)
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

	canProceed, _, cost, _ := quota.CheckQuota(sess.Account, quota.OpSocialSearch)
	if !canProceed {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":          "insufficient credits",
			"credits_needed": cost,
		})
		return
	}

	quota.Charge(sess.Account, quota.OpSocialSearch, nil)

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

	canProceed, _, cost, _ := quota.CheckQuota(sess.Account, quota.OpSocialSearch)
	if !canProceed {
		content := quota.ExceededPage(cost)
		app.Respond(w, r, app.Response{
			Title: "Social - Search",
			HTML:  content,
		})
		return
	}

	quota.Charge(sess.Account, quota.OpSocialSearch, nil)

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
  <div class="mt-1 text-sm">%s</div>
</div>`, htmlpkg.EscapeString(entry.Title), htmlpkg.EscapeString(content)))
	}

	if count == 0 {
		sb.WriteString(`<p class="text-muted">No results found</p>`)
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
		return `<p class="text-muted">No threads yet. Be the first to start one.</p>`
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
		if flag.IsHidden("social", p.ID) || auth.IsBanned(p.AuthorID) {
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
			replyInfo = fmt.Sprintf(` · <a href="/social/thread?id=%s" class="text-muted no-underline">%d %s</a>`, p.ID, rc, noun)
		}

		ts := p.PostedAt.Unix()
		authorHTML := htmlpkg.EscapeString(p.Author)
		if p.AuthorID == "_system" {
			authorHTML = fmt.Sprintf(`<span class="category">%s</span>`, authorHTML)
		}
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  <a href="/social/thread?id=%s">
    <span class="title">%s</span>
  </a>
  <span class="description breakable">%s</span>%s
  <div class="summary"><span data-timestamp="%d">%s</span>%s</div>
</div>`,
			p.ID,
			authorHTML,
			content,
			linkCard,
			ts,
			app.TimeAgo(p.PostedAt),
			replyInfo,
		))
	}

	return sb.String()
}

func generatePageHTML(visible []*Message, counts map[string]int, nav string, r *http.Request) string {
	var sb strings.Builder
	sb.WriteString(`<div class="col-narrow">`)

	// Compose box (shown to logged-in users)
	_, acc := auth.TrySession(r)
	if acc != nil {
		sb.WriteString(`<div class="mb-5">
  <form method="POST" action="/social" id="social-form">
    <textarea name="content" id="social-content" rows="3" placeholder="Start a thread..." required
      class="form-area"></textarea>
    <div class="d-flex between items-center mt-2">
      <span id="social-char-count" class="text-sm text-muted">0/500</span>
      <button type="submit" class="btn">Start Thread</button>
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
		sb.WriteString(`<div class="panel mb-5">
  <a href="/login" class="so-name">Log in</a> to start a thread
</div>`)
	}

	if len(visible) == 0 {
		sb.WriteString(`<p class="text-muted">No threads yet. Be the first to start one.</p>`)
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

		var userID string
		var isAdmin bool
		if acc != nil {
			userID = acc.ID
			isAdmin = acc.Admin
		}
		controls := app.ItemControls(userID, isAdmin, "social", p.ID, p.AuthorID, "", "/social?id="+p.ID)

		// Message count, counted once for the whole page rather than per row.
		rc := counts[p.ID]
		replyLink := fmt.Sprintf(`<a href="/social/thread?id=%s" class="text-muted no-underline text-sm">open thread</a>`, p.ID)
		if rc > 0 {
			noun := "messages"
			if rc == 1 {
				noun = "message"
			}
			replyLink = fmt.Sprintf(`<a href="/social/thread?id=%s" class="text-muted no-underline text-sm">%d %s</a>`, p.ID, rc, noun)
		}

		ts := p.PostedAt.Unix()
		authorHTML := fmt.Sprintf(`<b>%s</b>`, htmlpkg.EscapeString(p.Author))
		if p.AuthorID == "_system" {
			authorHTML = fmt.Sprintf(`<span class="category">%s</span>`, htmlpkg.EscapeString(p.Author))
		}
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  %s
  <div class="d-flex between so-head">
    <div>%s</div>
    <div><span data-timestamp="%d" class="text-muted text-sm">%s</span></div>
  </div>
  <div class="mt-1 breakable">%s</div>%s
  <div class="mt-1">%s</div>
</div>`,
			controls,
			authorHTML,
			ts,
			app.TimeAgo(p.PostedAt),
			content,
			linkCard,
			replyLink,
		))
	}

	sb.WriteString(nav)
	sb.WriteString(`</div>`)
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
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="so-url">%s</a>`, htmlpkg.EscapeString(href), htmlpkg.EscapeString(display))
	})
}

// renderLinkCard renders a Twitter-style embed card for a URL using cached OG metadata
func renderLinkCard(rawURL string) string {
	md, ok := linkmeta.Lookup(rawURL)
	if !ok || (md.Title == "" && md.Description == "") {
		// Fallback: simple domain card
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return ""
		}
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="link-card link-card-pad">
  <div class="text-sm text-muted">%s</div>
</a>`, htmlpkg.EscapeString(rawURL), htmlpkg.EscapeString(parsed.Hostname()))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="link-card">`, htmlpkg.EscapeString(rawURL)))

	if md.Image != "" {
		// Through the proxy: the publisher's CDN gets asked once by us rather
		// than once per reader, and the card stops depending on whether that
		// CDN, or the reader's blocker, feels like allowing a cross-origin
		// embed today. See internal/imageproxy.
		sb.WriteString(fmt.Sprintf(`<div class="w-full bg-soft"><img src="%s" class="so-image" loading="lazy" onerror="this.parentElement.style.display='none'"></div>`, htmlpkg.EscapeString(imageproxy.URL(md.Image))))
	}

	sb.WriteString(`<div class="so-pad">`)

	site := md.Site
	if site == "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			site = parsed.Hostname()
		}
	}
	if site != "" {
		sb.WriteString(fmt.Sprintf(`<div class="text-sm text-muted so-sub">%s</div>`, htmlpkg.EscapeString(site)))
	}

	if md.Title != "" {
		title := md.Title
		if len(title) > 100 {
			title = title[:97] + "..."
		}
		sb.WriteString(fmt.Sprintf(`<div class="text-base semibold so-title">%s</div>`, htmlpkg.EscapeString(title)))
	}

	if md.Description != "" {
		desc := md.Description
		if len(desc) > 150 {
			desc = desc[:147] + "..."
		}
		sb.WriteString(fmt.Sprintf(`<div class="text-sm text-secondary mt-1 so-desc">%s</div>`, htmlpkg.EscapeString(desc)))
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

// stripHTML removes HTML tags from a string
func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(s, " ")
	re2 := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re2.ReplaceAllString(text, " "))
}

// DetectSocialURLs finds social media URLs in text content
func DetectSocialURLs(content string) []string {
	re := regexp.MustCompile(`https?://(?:(?:(?:www|mobile)\.)?(?:twitter\.com|x\.com)|(?:(?:www\.)?truthsocial\.com))/[^\s"'<>\])+]+`)
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

// DeleteByAuthor removes all messages by a user.
func DeleteByAuthor(authorID string) {
	mutex.Lock()
	var kept []*Message
	for _, m := range messages {
		if m.AuthorID != authorID {
			kept = append(kept, m)
		}
	}
	messages = kept
	updateCacheLocked()
	mutex.Unlock()
	save()
}
