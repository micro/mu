package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"
)

// Import blog to get new account blog posts - will be set by blog package to avoid circular import
var GetNewAccountBlog func() []PostContent

// ============================================
// DATA STRUCTURES
// ============================================

type FlaggedItem struct {
	ContentType string    `json:"content_type"` // "post", "news", "video"
	ContentID   string    `json:"content_id"`
	FlagCount   int       `json:"flag_count"`
	Flagged     bool      `json:"flagged"`    // Hidden from public view
	FlaggedBy   []string  `json:"flagged_by"` // Usernames who flagged
	FlaggedAt   time.Time `json:"flagged_at"` // First flag timestamp
}

var (
	mutex sync.RWMutex
	flags = make(map[string]*FlaggedItem) // key: contentType:contentID
)

// ContentDeleter interface - each content type implements this
type ContentDeleter interface {
	Delete(id string) error
	Get(id string) interface{}
	RefreshCache()
}

var deleters = make(map[string]ContentDeleter)

// LLMAnalyzer interface for content moderation
type LLMAnalyzer interface {
	Analyze(prompt, question string) (string, error)
}

var analyzer LLMAnalyzer

// ============================================
// INITIALIZATION
// ============================================

func Load() {
	b, err := data.LoadFile("flags.json")
	if err != nil {
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	json.Unmarshal(b, &flags)
}

func saveUnlocked() error {
	// Caller must hold mutex lock
	return data.SaveJSON("flags.json", flags)
}

// RegisterDeleter registers a content type handler
func RegisterDeleter(contentType string, deleter ContentDeleter) {
	deleters[contentType] = deleter
}

// SetAnalyzer sets the LLM analyzer for content moderation
func SetAnalyzer(a LLMAnalyzer) {
	analyzer = a
}

// CheckContent analyzes content using LLM and flags if suspicious
func CheckContent(contentType, itemID, title, content string) {
	if analyzer == nil {
		return
	}

	prompt := `You are a content moderator. Analyze the following content and respond with ONLY ONE WORD:
- SPAM (if it's promotional spam or unwanted advertising)
- TEST (if it's clearly a test post like "test", "hello world", etc.)
- LOW_QUALITY (if it's very short, nonsensical, or has no value)
- OK (if the content is fine)

Respond with just the single word classification.`

	question := fmt.Sprintf("Title: %s\n\nContent: %s", title, content)

	resp, err := analyzer.Analyze(prompt, question)
	if err != nil {
		fmt.Printf("Moderation analysis error: %v\n", err)
		return
	}

	resp = strings.TrimSpace(strings.ToUpper(resp))
	fmt.Printf("Content moderation: %s %s -> %s\n", contentType, itemID, resp)

	if resp == "SPAM" || resp == "TEST" || resp == "LOW_QUALITY" {
		// Auto-flag by system (use "system" as username)
		Add(contentType, itemID, "system")
		fmt.Printf("Auto-flagged %s: %s (reason: %s)\n", contentType, itemID, resp)
	}
}

// ============================================
// FLAGGING OPERATIONS
// ============================================

// Add adds a flag to content (returns new flag count, already flagged bool, error)
func Add(contentType, contentID, username string) (int, bool, error) {
	key := contentType + ":" + contentID

	mutex.Lock()
	defer mutex.Unlock()

	item, exists := flags[key]
	if !exists {
		item = &FlaggedItem{
			ContentType: contentType,
			ContentID:   contentID,
			FlagCount:   0,
			Flagged:     false,
			FlaggedBy:   []string{},
			FlaggedAt:   time.Now(),
		}
		flags[key] = item
	}

	// Check if user already flagged
	for _, flagger := range item.FlaggedBy {
		if flagger == username {
			return item.FlagCount, true, nil
		}
	}

	// Add flag
	item.FlaggedBy = append(item.FlaggedBy, username)
	item.FlagCount++

	// Auto-hide after 3 flags
	if item.FlagCount >= 3 {
		item.Flagged = true
	}

	saveUnlocked()
	return item.FlagCount, false, nil
}

// GetCount returns flag count for content
func GetCount(contentType, contentID string) int {
	count, _ := GetFlags(contentType, contentID)
	return count
}

// GetFlags returns flag info for content (flagCount, isFlagged)
func GetFlags(contentType, contentID string) (int, bool) {
	key := contentType + ":" + contentID

	mutex.RLock()
	defer mutex.RUnlock()

	if item, exists := flags[key]; exists {
		return item.FlagCount, item.Flagged
	}
	return 0, false
}

// GetItem returns full flag details
func GetItem(contentType, contentID string) *FlaggedItem {
	key := contentType + ":" + contentID

	mutex.RLock()
	defer mutex.RUnlock()

	if item, exists := flags[key]; exists {
		return item
	}
	return nil
}

// GetAll returns all flagged items
func GetAll() []*FlaggedItem {
	mutex.RLock()
	defer mutex.RUnlock()

	var items []*FlaggedItem
	for _, item := range flags {
		if item.FlagCount > 0 {
			items = append(items, item)
		}
	}
	return items
}

// Approve clears flags for content
func Approve(contentType, contentID string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	delete(flags, key)
	err := saveUnlocked()
	mutex.Unlock()

	if err != nil {
		return err
	}

	// Refresh the content cache after unlocking to avoid deadlock
	// (RefreshCache may call back into admin.IsHidden which needs a lock)
	if deleter, ok := deleters[contentType]; ok {
		deleter.RefreshCache()
	}

	// Force home page refresh
	go func() {
		// Dynamically import to avoid circular dependency
		// This will be handled by the deleter's RefreshCache already
	}()

	return nil
}

// IsHidden checks if content is flagged/hidden
func IsHidden(contentType, contentID string) bool {
	_, flagged := GetFlags(contentType, contentID)
	return flagged
}

// AdminFlag immediately hides content (for admin use)
func AdminFlag(contentType, contentID, username string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	if item, exists := flags[key]; exists {
		item.FlagCount = 3
		item.Flagged = true
		if !contains(item.FlaggedBy, username) {
			item.FlaggedBy = append(item.FlaggedBy, username+" (admin)")
		}
	} else {
		flags[key] = &FlaggedItem{
			ContentType: contentType,
			ContentID:   contentID,
			FlagCount:   3,
			Flagged:     true,
			FlaggedBy:   []string{username + " (admin)"},
			FlaggedAt:   time.Now(),
		}
	}
	err := saveUnlocked()
	mutex.Unlock()

	if err != nil {
		return err
	}

	// Refresh cache immediately
	if deleter, ok := deleters[contentType]; ok {
		go deleter.RefreshCache()
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Delete removes both the flag and the content
func Delete(contentType, contentID string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	delete(flags, key)
	err := saveUnlocked()
	mutex.Unlock()

	if err != nil {
		return err
	}

	// Delete the actual content
	if deleter, ok := deleters[contentType]; ok {
		if err := deleter.Delete(contentID); err != nil {
			return err
		}
		// Refresh cache immediately after deletion
		go deleter.RefreshCache()
	}

	return nil
}

// ============================================
// HTTP HANDLERS
// ============================================

// FlagHandler handles flag submissions
func FlagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var contentType, contentID string

	if app.SendsJSON(r) {
		var req struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.RespondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		contentType = req.Type
		contentID = req.ID
	} else {
		contentType = r.FormValue("type")
		contentID = r.FormValue("id")
	}

	if contentID == "" || contentType == "" {
		http.Error(w, "Content ID and type required", http.StatusBadRequest)
		return
	}

	// Get the authenticated user
	flagger := "Anonymous"
	_, acc := auth.TrySession(r)
	if acc != nil {
		flagger = acc.Name
	}

	// Add flag
	count, alreadyFlagged, err := Add(contentType, contentID, flagger)
	if err != nil {
		http.Error(w, "Failed to flag content", http.StatusInternalServerError)
		return
	}

	if alreadyFlagged {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": false, "message": "Already flagged"}`))
		return
	}

	// Refresh cache if content was hidden
	if count >= 3 {
		if deleter, ok := deleters[contentType]; ok {
			deleter.RefreshCache()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true, "count": ` + fmt.Sprintf("%d", count) + `}`))
}

// ModerateHandler shows all flagged content
func ModerateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		handleModeration(w, r)
		return
	}

	// Check if user is admin
	_, acc, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	_ = acc // acc.Admin is always true here

	flaggedItems := GetAll()

	var itemsList []string
	for _, item := range flaggedItems {
		var contentHTML string
		var title string
		var author string
		var createdAt string

		// Get content from the appropriate handler
		if deleter, ok := deleters[item.ContentType]; ok {
			content := deleter.Get(item.ContentID)
			switch item.ContentType {
			case "post":
				if post, ok := content.(PostContent); ok {
					title = post.Title
					if title == "" {
						title = "Untitled"
					}
					text := post.Content
					if len(text) > 300 {
						text = text[:300] + "..."
					}
					contentHTML = fmt.Sprintf(`<p class="whitespace-pre-wrap">%s</p>`, text)
					author = post.Author
					createdAt = app.TimeAgo(post.CreatedAt)
				}
			case "news":
				// TODO: Implement news content display
				title = "News Article"
				contentHTML = `<p>[News content]</p>`
			case "video":
				// TODO: Implement video content display
				title = "Video"
				contentHTML = `<p>[Video content]</p>`
			}
		}

		status := "Under review"
		if item.Flagged {
			status = "Hidden"
		}

		// Build action buttons HTML (admin only - we're already admin here)
		actionButtons := ""
			actionButtons = fmt.Sprintf(`
				<form method="POST" action="/admin/moderate">
					<input type="hidden" name="action" value="approve">
					<input type="hidden" name="type" value="%s">
					<input type="hidden" name="id" value="%s">
					<button type="submit" class="btn-approve">Approve</button>
				</form>
				<form method="POST" action="/admin/moderate">
					<input type="hidden" name="action" value="delete">
					<input type="hidden" name="type" value="%s">
					<input type="hidden" name="id" value="%s">
					<button type="submit" class="btn-delete">Delete</button>
				</form>`,
				item.ContentType, item.ContentID,
				item.ContentType, item.ContentID)

		html := fmt.Sprintf(`<div class="flagged-item">
			<div>
				<span class="content-type-badge">%s</span>
				<h3>%s</h3>
			</div>
			%s
			<div class="info">
				%s by %s · Flags: %d · Status: %s<br>
				Flagged by: %s
			</div>
			<div class="actions">
				%s
				<a href="/%s?id=%s" target="_blank">view</a>
			</div>
		</div>`,
			item.ContentType,
			title,
			contentHTML,
			createdAt,
			author,
			item.FlagCount,
			status,
			strings.Join(item.FlaggedBy, ", "),
			actionButtons,
			getViewPath(item.ContentType),
			item.ContentID)

		itemsList = append(itemsList, html)
	}

	listHTML := "<p style='color: #666;'>No flagged content</p>"
	if len(itemsList) > 0 {
		listHTML = strings.Join(itemsList, "\n")
	}

	// Get new account blog posts section
	newAccountPostsHTML := ""
	if GetNewAccountBlog != nil {
		newPosts := GetNewAccountBlog()
		if len(newPosts) > 0 {
			var newPostsList []string
			for _, post := range newPosts {
				title := post.Title
				if title == "" {
					title = "Untitled"
				}

				content := post.Content
				if len(content) > 300 {
					content = content[:300] + "..."
				}

				item := fmt.Sprintf(`<div class="flagged-item">
				<div>
					<span class="content-type-badge">post</span>
					<h3>%s</h3>
				</div>
				<p class="whitespace-pre-wrap">%s</p>
				<div class="info">
					%s by %s · New Account (< 24h) · Hidden from homepage
				</div>
				<div class="actions">
					<a href="/post?id=%s" target="_blank">view</a> · 
					<a href="/flag?type=post&id=%s" class="text-error">flag</a>
				</div>
			</div>`,
					title,
					content,
					app.TimeAgo(post.CreatedAt),
					post.Author,
					post.ID,
					post.ID)

				newPostsList = append(newPostsList, item)
			}

			newAccountPostsHTML = fmt.Sprintf(`
				<h2 class="mt-6">New Account Blog Posts (< 24 hours old)</h2>
				<p class="text-muted mb-4">These blog posts are hidden from the public homepage but can still be flagged if inappropriate.</p>
				<div id="new-account-blog">
					%s
				</div>`, strings.Join(newPostsList, "\n"))
		}
	}

	content := fmt.Sprintf(`<div id="moderation">
		<div class="info-banner">
			<strong>Community Moderation</strong><br>
			Review content that has been flagged by users. Content is automatically hidden after 3 flags. 
			You can approve (clear flags) or delete the content permanently.
		</div>
		<h2>Flagged Content</h2>
		<div id="flagged-content">
			%s
		</div>
		%s
	</div>`, listHTML, newAccountPostsHTML)

	html := app.RenderHTMLForRequest("Moderate", "Review flagged content", content, r)
	w.Write([]byte(html))
}

func getViewPath(contentType string) string {
	switch contentType {
	case "post":
		return "post"
	case "news":
		return "news"
	case "video":
		return "video"
	default:
		return ""
	}
}

func handleModeration(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("action")
	contentType := r.FormValue("type")
	contentID := r.FormValue("id")

	if contentID == "" || contentType == "" {
		app.BadRequest(w, r, "Content ID and type required")
		return
	}

	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	switch action {
	case "approve":
		Approve(contentType, contentID)
		http.Redirect(w, r, "/admin/moderate", http.StatusSeeOther)

	case "delete":
		Delete(contentType, contentID)
		http.Redirect(w, r, "/admin/moderate", http.StatusSeeOther)

	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}

// ============================================
// CONTENT INTERFACES
// ============================================

// PostContent represents post data for display
type PostContent struct {
	ID        string
	Title     string
	Content   string
	Author    string
	CreatedAt time.Time
}
