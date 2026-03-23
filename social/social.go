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

// posts stored newest first
var posts []*Post

// cached HTML
var cardHTML string
var pageBodyHTML string

// startup throttle: suppress breaking posts for first 30 seconds after load
var loadedAt time.Time

// nitterInstance for fetching X/Twitter posts via Nitter (used by FetchPost/context)
var nitterInstance = "nitter.poast.org"

// Post represents a social post by a user
type Post struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`     // display name
	AuthorID  string    `json:"author_id"`  // account ID
	Content   string    `json:"content"`
	PostedAt  time.Time `json:"posted_at"`
}

// addPost adds a post to the feed (prepend, dedup, cap, save)
func addPost(p *Post) {
	mutex.Lock()
	// Dedup by ID
	for _, existing := range posts {
		if existing.ID == p.ID {
			mutex.Unlock()
			return
		}
	}
	posts = append([]*Post{p}, posts...)
	if len(posts) > 500 {
		posts = posts[:500]
	}
	updateCacheLocked()
	mutex.Unlock()

	indexPosts([]*Post{p})
	save()
}

func Load() {
	// Load saved posts
	b, err := data.LoadFile("social_posts.json")
	if err == nil {
		var cached []*Post
		if json.Unmarshal(b, &cached) == nil {
			mutex.Lock()
			posts = cached
			updateCacheLocked()
			mutex.Unlock()
			indexPosts(cached)
		}
	}

	loadedAt = time.Now()

	// Subscribe to news summaries — surface breaking stories as social posts
	go func() {
		sub := event.Subscribe(event.EventSummaryGenerated)
		startupPostCount := 0
		for evt := range sub.Chan {
			contentType, _ := evt.Data["type"].(string)
			if contentType != "news" {
				continue
			}
			summary, _ := evt.Data["summary"].(string)
			uri, _ := evt.Data["uri"].(string)

			if summary == "" || uri == "" {
				continue
			}

			// Throttle during startup: skip most breaking posts in first 30s
			if time.Since(loadedAt) < 30*time.Second {
				startupPostCount++
				if startupPostCount > 2 {
					app.Log("social", "Throttled breaking during startup: %s", uri)
					continue
				}
			}

			// Take the first sentence or two of the summary as the social post
			content := firstSentences(summary, 2)
			if content == "" {
				continue
			}
			if uri != "" {
				content += " " + uri
			}
			if len(content) > 500 {
				content = content[:497] + "..."
			}

			id := fmt.Sprintf("%x", md5.Sum([]byte("news:"+uri)))[:16]

			addPost(&Post{
				ID:       id,
				Author:   "Breaking",
				AuthorID: "_system",
				Content:  content,
				PostedAt: time.Now(),
			})

			app.Log("social", "Surfaced breaking: %s", content[:min(80, len(content))])
		}
	}()

	app.Log("social", "Loaded %d posts", len(posts))
}

func save() error {
	mutex.RLock()
	p := make([]*Post, len(posts))
	copy(p, posts)
	mutex.RUnlock()
	return data.SaveJSON("social_posts.json", p)
}

// updateCacheLocked regenerates cached HTML. Caller must hold mutex write lock.
func updateCacheLocked() {
	cardHTML = generateCardHTML(posts)
	pageBodyHTML = "" // invalidate, regenerated on next request
}

// CardHTML returns cached dashboard card HTML
func CardHTML() string {
	mutex.RLock()
	defer mutex.RUnlock()
	return cardHTML
}

// GetPosts returns all cached posts (most recent first)
func GetPosts() []*Post {
	mutex.RLock()
	defer mutex.RUnlock()
	result := make([]*Post, len(posts))
	copy(result, posts)
	return result
}

// Handler serves the /social endpoint
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if app.SendsJSON(r) {
			// JSON POST could be search or create
			handleJSONPost(w, r)
			return
		}
		handleCreatePost(w, r)
		return
	case "DELETE":
		handleDeletePost(w, r)
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

func handleCreatePost(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	if !auth.CanPost(acc.ID) {
		app.BadRequest(w, r, "Your account is too new to post. Please wait a bit.")
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
		app.BadRequest(w, r, "Posts must be 500 characters or less")
		return
	}
	if len(strings.Fields(content)) < 2 {
		app.BadRequest(w, r, "Post must contain at least 2 words")
		return
	}

	postID := fmt.Sprintf("%d", time.Now().UnixNano())

	p := &Post{
		ID:       postID,
		Author:   acc.Name,
		AuthorID: acc.ID,
		Content:  content,
		PostedAt: time.Now(),
	}

	addPost(p)

	// Async content moderation
	go flag.CheckContent("social", postID, "", content)

	app.Log("social", "New post by %s (%s)", acc.Name, acc.ID)

	if app.SendsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"success": true, "id": postID})
		return
	}
	http.Redirect(w, r, "/social", http.StatusSeeOther)
}

func handleJSONPost(w http.ResponseWriter, r *http.Request) {
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

	// Otherwise it's a create post
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
		http.Error(w, "Account too new to post", http.StatusForbidden)
		return
	}

	if len(content) > 500 {
		http.Error(w, "Posts must be 500 characters or less", 400)
		return
	}

	postID := fmt.Sprintf("%d", time.Now().UnixNano())
	p := &Post{
		ID:       postID,
		Author:   acc.Name,
		AuthorID: acc.ID,
		Content:  content,
		PostedAt: time.Now(),
	}

	addPost(p)

	go flag.CheckContent("social", postID, "", content)

	app.RespondJSON(w, map[string]interface{}{"success": true, "id": postID})
}

func handleDeletePost(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	postID := r.URL.Query().Get("id")
	if postID == "" {
		app.BadRequest(w, r, "Post ID required")
		return
	}

	mutex.Lock()
	found := false
	for i, p := range posts {
		if p.ID == postID {
			// Only author or admin can delete
			if p.AuthorID != acc.ID && !acc.Admin {
				mutex.Unlock()
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			posts = append(posts[:i], posts[i+1:]...)
			found = true
			break
		}
	}
	if found {
		updateCacheLocked()
	}
	mutex.Unlock()

	if !found {
		http.Error(w, "Post not found", 404)
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
	currentPosts := make([]*Post, len(posts))
	copy(currentPosts, posts)
	mutex.RUnlock()

	// Filter out flagged posts
	var visible []*Post
	for _, p := range currentPosts {
		if !flag.IsHidden("social", p.ID) {
			visible = append(visible, p)
		}
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"posts": visible})
		return
	}

	body := generatePageHTML(visible, r)

	app.Respond(w, r, app.Response{
		Title:       "Social",
		Description: "Share what's on your mind",
		HTML:        body,
	})
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

func indexPosts(toIndex []*Post) {
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

func generateCardHTML(allPosts []*Post) string {
	if len(allPosts) == 0 {
		return `<p style="color:#888;">No posts yet. Be the first to share something.</p>`
	}

	// Show up to 4 latest posts, one per author for variety
	// Limit breaking posts to at most 1 on the home card
	seen := map[string]bool{}
	breakingCount := 0
	var selected []*Post
	for _, p := range allPosts {
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

		ts := p.PostedAt.Unix()
		sb.WriteString(fmt.Sprintf(`<div class="headline" style="border:none;border-bottom:1px solid #f0f0f0;border-radius:0;padding:8px 0;">
  <div style="font-size:13px;"><b>%s</b> <span data-timestamp="%d" style="color:#888;font-size:12px;">%s</span></div>
  <div style="font-size:13px;margin-top:2px;color:#333;overflow-wrap:break-word;word-break:break-word;">%s</div>%s
</div>`,
			htmlpkg.EscapeString(p.Author),
			ts,
			app.TimeAgo(p.PostedAt),
			content,
			linkCard,
		))
	}

	return sb.String()
}

func generatePageHTML(visible []*Post, r *http.Request) string {
	var sb strings.Builder

	// Compose box (shown to logged-in users)
	_, acc := auth.TrySession(r)
	if acc != nil {
		sb.WriteString(`<div style="margin-bottom:20px;">
  <form method="POST" action="/social" id="social-form">
    <textarea name="content" id="social-content" rows="3" placeholder="What's on your mind?" required
      style="width:100%;box-sizing:border-box;padding:10px;border:1px solid #ddd;border-radius:8px;font-family:inherit;font-size:14px;resize:vertical;"></textarea>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-top:8px;">
      <span id="social-char-count" style="font-size:12px;color:#888;">0/500</span>
      <button type="submit" style="padding:8px 20px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-family:inherit;">Post</button>
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
  <a href="/login" style="color:#000;font-weight:bold;">Log in</a> to share a post
</div>`)
	}

	if len(visible) == 0 {
		sb.WriteString(`<p style="color:#888;">No posts yet. Be the first to share something.</p>`)
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
			deleteBtn = fmt.Sprintf(` <button onclick="if(confirm('Delete this post?')){fetch('/social?id=%s',{method:'DELETE'}).then(()=>location.reload())}" style="background:none;border:none;color:#ccc;cursor:pointer;font-size:12px;padding:0;" title="Delete">x</button>`, p.ID)
		}

		ts := p.PostedAt.Unix()
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  <div style="display:flex;justify-content:space-between;align-items:baseline;">
    <div><b>%s</b></div>
    <div><span data-timestamp="%d" style="color:#888;font-size:12px;">%s</span>%s</div>
  </div>
  <div style="margin-top:4px;overflow-wrap:break-word;word-break:break-word;">%s</div>%s
</div>`,
			htmlpkg.EscapeString(p.Author),
			ts,
			app.TimeAgo(p.PostedAt),
			deleteBtn,
			content,
			linkCard,
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

// SurfaceBreaking creates a system post from external sources (e.g., news headlines)
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

	addPost(&Post{
		ID:       id,
		Author:   "Breaking",
		AuthorID: "_system",
		Content:  content,
		PostedAt: time.Now(),
	})
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

// FetchPost fetches a single social post by URL and returns it (used by context.go for news)
func FetchPost(rawURL string) (*Post, error) {
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

	return &Post{
		ID:       id,
		Author:   handle,
		AuthorID: handle,
		Content:  text,
		PostedAt: time.Now(),
	}, nil
}
