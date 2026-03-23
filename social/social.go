package social

import (
	"crypto/md5"
	"embed"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/wallet"
)

//go:embed accounts.json
var f embed.FS

var mutex sync.RWMutex

// accounts maps category to list of accounts
var accounts = map[string][]Account{}

// cached posts (most recent first)
var posts []*Post

// cached HTML
var cardHTML string
var pageBodyHTML string

// nitterInstance for fetching X/Twitter posts via RSS
var nitterInstance = "nitter.poast.org"

// Account represents a social media account to follow
type Account struct {
	Handle   string `json:"handle"`
	Platform string `json:"platform"` // "x", "truthsocial"
}

// Post represents a social media post
type Post struct {
	ID       string    `json:"id"`
	Author   string    `json:"author"`
	Handle   string    `json:"handle"`
	Platform string    `json:"platform"`
	Content  string    `json:"content"`
	URL      string    `json:"url"`
	PostedAt time.Time `json:"posted_at"`
	Category string    `json:"category"`
	Image    string    `json:"image,omitempty"`
	Source   string    `json:"source"` // "feed" (fetched) or "self" (user-posted)
}

func Load() {
	// Load embedded accounts config
	b, err := f.ReadFile("accounts.json")
	if err != nil {
		app.Log("social", "Failed to read accounts.json: %v", err)
		return
	}
	if err := json.Unmarshal(b, &accounts); err != nil {
		app.Log("social", "Failed to parse accounts.json: %v", err)
		return
	}

	app.Log("social", "Loaded %d categories", len(accounts))

	// Load cached posts
	b, err = data.LoadFile("social_posts.json")
	if err == nil {
		var cached []*Post
		if json.Unmarshal(b, &cached) == nil {
			mutex.Lock()
			posts = cached
			cardHTML = generateCardHTML(cached)
			pageBodyHTML = generatePageHTML(cached)
			mutex.Unlock()
		}
	}

	// Load cached HTML
	b, err = data.LoadFile("social_card.html")
	if err == nil && len(b) > 0 {
		mutex.Lock()
		cardHTML = string(b)
		mutex.Unlock()
	}

	// Start background refresh
	go refreshPosts()
}

func refreshPosts() {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					app.Log("social", "Panic in refreshPosts: %v\n%s", r, debug.Stack())
				}
			}()

			newPosts := fetchAllAccounts()
			if len(newPosts) > 0 {
				// Merge with existing posts, deduplicate by ID
				merged := mergePosts(newPosts)

				mutex.Lock()
				posts = merged
				cardHTML = generateCardHTML(merged)
				pageBodyHTML = generatePageHTML(merged)
				mutex.Unlock()

				indexPosts(merged)
				data.SaveJSON("social_posts.json", merged)
				data.SaveFile("social_card.html", cardHTML)

				app.Log("social", "Refreshed %d posts", len(merged))
			}
		}()

		time.Sleep(15 * time.Minute)
	}
}

func fetchAllAccounts() []*Post {
	var allPosts []*Post
	var mu sync.Mutex
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 15 * time.Second}
	parser := gofeed.NewParser()

	for category, accs := range accounts {
		for _, acc := range accs {
			wg.Add(1)
			go func(cat string, acc Account) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						app.Log("social", "Panic fetching @%s: %v", acc.Handle, r)
					}
				}()

				feedURL := feedURLFor(acc)
				if feedURL == "" {
					return
				}

				resp, err := client.Get(feedURL)
				if err != nil {
					app.Log("social", "Error fetching @%s: %v", acc.Handle, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != 200 {
					app.Log("social", "Error fetching @%s: status %d", acc.Handle, resp.StatusCode)
					return
				}

				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return
				}

				feed, err := parser.ParseString(string(body))
				if err != nil {
					app.Log("social", "Error parsing @%s feed: %v", acc.Handle, err)
					return
				}

				for _, item := range feed.Items {
					if item == nil {
						continue
					}

					postedAt := time.Now()
					if item.PublishedParsed != nil {
						postedAt = *item.PublishedParsed
					} else if item.UpdatedParsed != nil {
						postedAt = *item.UpdatedParsed
					}

					// Only keep posts from last 7 days
					if time.Since(postedAt) > 7*24*time.Hour {
						continue
					}

					content := item.Description
					if content == "" {
						content = item.Title
					}
					// Strip HTML tags for clean text
					content = stripHTML(content)

					// Truncate very long posts
					if len(content) > 1000 {
						content = content[:1000] + "…"
					}

					postURL := item.Link
					if postURL == "" && item.GUID != "" {
						postURL = item.GUID
					}

					// Generate stable ID from URL
					id := fmt.Sprintf("%x", md5.Sum([]byte(postURL)))[:16]

					// Derive display name from handle
					author := acc.Handle

					p := &Post{
						ID:       id,
						Author:   author,
						Handle:   acc.Handle,
						Platform: acc.Platform,
						Content:  content,
						URL:      postURL,
						PostedAt: postedAt,
						Category: cat,
						Source:   "feed",
					}

					// Extract first image if present
					if item.Image != nil && item.Image.URL != "" {
						p.Image = item.Image.URL
					}

					mu.Lock()
					allPosts = append(allPosts, p)
					mu.Unlock()
				}
			}(category, acc)
		}
	}

	wg.Wait()

	// Sort by most recent first
	sort.Slice(allPosts, func(i, j int) bool {
		return allPosts[i].PostedAt.After(allPosts[j].PostedAt)
	})

	return allPosts
}

func feedURLFor(acc Account) string {
	switch acc.Platform {
	case "x":
		return fmt.Sprintf("https://%s/%s/rss", nitterInstance, acc.Handle)
	case "truthsocial":
		return fmt.Sprintf("https://truthsocial.com/@%s.rss", acc.Handle)
	default:
		return ""
	}
}

func mergePosts(newPosts []*Post) []*Post {
	mutex.RLock()
	existing := posts
	mutex.RUnlock()

	seen := map[string]bool{}
	var merged []*Post

	// New posts take priority
	for _, p := range newPosts {
		if !seen[p.ID] {
			seen[p.ID] = true
			merged = append(merged, p)
		}
	}

	// Keep old posts not in new set (e.g. self-posted)
	for _, p := range existing {
		if !seen[p.ID] {
			seen[p.ID] = true
			// Only keep old feed posts for 7 days
			if p.Source == "feed" && time.Since(p.PostedAt) > 7*24*time.Hour {
				continue
			}
			merged = append(merged, p)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].PostedAt.After(merged[j].PostedAt)
	})

	// Cap at 200 posts
	if len(merged) > 200 {
		merged = merged[:200]
	}

	return merged
}

func indexPosts(posts []*Post) {
	for _, p := range posts {
		data.Index(
			"social_"+p.ID,
			"social",
			p.Author+" (@"+p.Handle+")",
			p.Content,
			map[string]interface{}{
				"url":       p.URL,
				"handle":    p.Handle,
				"platform":  p.Platform,
				"category":  p.Category,
				"posted_at": p.PostedAt,
			},
		)
	}
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

// GetPostsByHandle returns posts from a specific handle
func GetPostsByHandle(handle string) []*Post {
	mutex.RLock()
	defer mutex.RUnlock()
	var result []*Post
	for _, p := range posts {
		if strings.EqualFold(p.Handle, handle) {
			result = append(result, p)
		}
	}
	return result
}

func generateCardHTML(posts []*Post) string {
	if len(posts) == 0 {
		return `<p style="color:#888;">No posts yet</p>`
	}

	// Show latest posts, one per account for variety
	seen := map[string]bool{}
	var selected []*Post
	for _, p := range posts {
		if seen[p.Handle] {
			continue
		}
		seen[p.Handle] = true
		selected = append(selected, p)
		if len(selected) >= 4 {
			break
		}
	}

	var sb strings.Builder
	for _, p := range selected {
		content := p.Content
		if len(content) > 120 {
			content = content[:120] + "…"
		}
		sb.WriteString(fmt.Sprintf(`<div class="headline" style="border:none;border-bottom:1px solid #f0f0f0;border-radius:0;padding:8px 0;">
  <a href="%s" target="_blank" rel="noopener noreferrer" style="text-decoration:none;color:inherit;">
    <div style="font-size:13px;"><b>@%s</b> <span style="color:#888;font-size:12px;">· %s · %s</span></div>
    <div style="font-size:13px;margin-top:2px;color:#333;">%s</div>
  </a>
</div>`,
			htmlpkg.EscapeString(p.URL),
			htmlpkg.EscapeString(p.Handle),
			htmlpkg.EscapeString(platformLabel(p.Platform)),
			app.TimeAgo(p.PostedAt),
			htmlpkg.EscapeString(content),
		))
	}

	return sb.String()
}

func generatePageHTML(posts []*Post) string {
	if len(posts) == 0 {
		return `<p style="color:#888;">No posts yet. Accounts are being fetched.</p>`
	}

	// Group by category
	categories := map[string][]*Post{}
	var catOrder []string
	for _, p := range posts {
		if _, exists := categories[p.Category]; !exists {
			catOrder = append(catOrder, p.Category)
		}
		categories[p.Category] = append(categories[p.Category], p)
	}
	sort.Strings(catOrder)

	// Build category tabs
	var sb strings.Builder
	sb.WriteString(app.Head("social", catOrder))

	// Show all posts with most recent first
	for _, p := range posts {
		content := p.Content
		if len(content) > 500 {
			content = content[:500] + "…"
		}
		sb.WriteString(fmt.Sprintf(`<div class="headline" data-ref="%s">
  <a href="%s" target="_blank" rel="noopener noreferrer" style="text-decoration:none;color:inherit;">
    <div style="display:flex;justify-content:space-between;align-items:baseline;">
      <div><b>@%s</b> <span style="color:#888;font-size:12px;">%s</span></div>
      <span style="color:#888;font-size:12px;">%s</span>
    </div>
    <div style="margin-top:4px;">%s</div>
  </a>
</div>`,
			htmlpkg.EscapeString(strings.ToLower(p.Category)),
			htmlpkg.EscapeString(p.URL),
			htmlpkg.EscapeString(p.Handle),
			htmlpkg.EscapeString(platformLabel(p.Platform)),
			app.TimeAgo(p.PostedAt),
			htmlpkg.EscapeString(content),
		))
	}

	return sb.String()
}

func platformLabel(platform string) string {
	switch platform {
	case "x":
		return "X"
	case "truthsocial":
		return "Truth Social"
	default:
		return platform
	}
}

// Handler serves the /social endpoint
func Handler(w http.ResponseWriter, r *http.Request) {
	// Handle POST with JSON (API search)
	if r.Method == "POST" && app.SendsJSON(r) {
		handleAPISearch(w, r)
		return
	}

	// Handle search query (HTML)
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

	// Handle filter by handle
	if handle := r.URL.Query().Get("handle"); handle != "" {
		filtered := GetPostsByHandle(handle)
		if app.WantsJSON(r) {
			app.RespondJSON(w, map[string]interface{}{
				"handle": handle,
				"posts":  filtered,
			})
			return
		}
	}

	// GET social feed
	handleGetFeed(w, r)
}

func handleGetFeed(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	currentPosts := posts
	mutex.RUnlock()

	// JSON response
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"posts": currentPosts,
		})
		return
	}

	// HTML response
	mutex.RLock()
	body := pageBodyHTML
	mutex.RUnlock()

	if len(currentPosts) > 0 {
		body = generatePageHTML(currentPosts)
	}

	app.Respond(w, r, app.Response{
		Title:       "Social",
		Description: "Keep track of what's happening",
		HTML:        body,
	})
}

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var reqData map[string]interface{}
	b, _ := ioutil.ReadAll(r.Body)
	json.Unmarshal(b, &reqData)

	query := ""
	if v := reqData["query"]; v != nil {
		query = fmt.Sprintf("%v", v)
	}
	if query == "" {
		http.Error(w, "query required", 400)
		return
	}

	// Check quota
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

	// Search indexed social posts
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

	// Check quota
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
		postURL := ""
		if entry.Metadata != nil {
			if u, ok := entry.Metadata["url"].(string); ok {
				postURL = u
			}
		}
		content := entry.Content
		if len(content) > 300 {
			content = content[:300] + "…"
		}
		sb.WriteString(fmt.Sprintf(`<div class="headline">
  <a href="%s" target="_blank" rel="noopener noreferrer" style="text-decoration:none;color:inherit;">
    <div><b>%s</b></div>
    <div style="margin-top:4px;font-size:13px;">%s</div>
  </a>
</div>`, htmlpkg.EscapeString(postURL), htmlpkg.EscapeString(entry.Title), htmlpkg.EscapeString(content)))
	}

	if count == 0 {
		sb.WriteString(`<p style="color:#888;">No results found</p>`)
	}

	app.Respond(w, r, app.Response{
		Title: "Social - Search",
		HTML:  sb.String(),
	})
}

// stripHTML removes HTML tags from a string
func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(s, " ")
	// Collapse whitespace
	re2 := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re2.ReplaceAllString(text, " "))
}

// DetectSocialURLs finds social media URLs in text content
func DetectSocialURLs(content string) []string {
	re := regexp.MustCompile(`https?://(?:(?:www\.)?(?:twitter\.com|x\.com)|(?:truthsocial\.com))/[^\s"'<>\])+]+`)
	matches := re.FindAllString(content, -1)

	// Deduplicate
	seen := map[string]bool{}
	var unique []string
	for _, m := range matches {
		// Clean trailing punctuation
		m = strings.TrimRight(m, ".,;:!?)")
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

// FetchPost fetches a single social post by URL and returns it
func FetchPost(rawURL string) (*Post, error) {
	// Rewrite Twitter/X URLs to Nitter
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

	// Extract post content from HTML
	text := stripHTML(string(body))
	if len(text) > 1000 {
		text = text[:1000] + "…"
	}

	// Try to extract handle from URL path
	handle := ""
	if parsed != nil && len(parsed.Path) > 1 {
		parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(parts) > 0 {
			handle = strings.TrimPrefix(parts[0], "@")
		}
	}

	// Detect platform
	platform := "x"
	if strings.Contains(rawURL, "truthsocial.com") {
		platform = "truthsocial"
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte(rawURL)))[:16]

	return &Post{
		ID:       id,
		Author:   handle,
		Handle:   handle,
		Platform: platform,
		Content:  text,
		URL:      rawURL,
		PostedAt: time.Now(),
		Source:   "fetch",
	}, nil
}
