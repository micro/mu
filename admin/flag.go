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

// ============================================
// DATA STRUCTURES
// ============================================

type FlaggedItem struct {
	ContentType string    `json:"content_type"` // "post", "news", "video"
	ContentID   string    `json:"content_id"`
	FlagCount   int       `json:"flag_count"`
	Flagged     bool      `json:"flagged"`     // Hidden from public view
	FlaggedBy   []string  `json:"flagged_by"`  // Usernames who flagged
	FlaggedAt   time.Time `json:"flagged_at"`  // First flag timestamp
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
	
	return nil
}

// IsHidden checks if content is flagged/hidden
func IsHidden(contentType, contentID string) bool {
	_, flagged := GetFlags(contentType, contentID)
	return flagged
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

	contentType := r.FormValue("type")
	contentID := r.FormValue("id")

	if contentID == "" || contentType == "" {
		http.Error(w, "Content ID and type required", http.StatusBadRequest)
		return
	}

	// Get the authenticated user
	flagger := "Anonymous"
	sess, err := auth.GetSession(r)
	if err == nil {
		acc, err := auth.GetAccount(sess.Account)
		if err == nil {
			flagger = acc.Name
		}
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
	isAdmin := false
	sess, err := auth.GetSession(r)
	if err == nil {
		acc, err := auth.GetAccount(sess.Account)
		if err == nil && acc.Admin {
			isAdmin = true
		}
	}

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
					contentHTML = fmt.Sprintf(`<p style="white-space: pre-wrap;">%s</p>`, text)
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

		// Build action buttons HTML (only for admins)
		actionButtons := ""
		if isAdmin {
			actionButtons = fmt.Sprintf(`
				<form method="POST" action="/moderate" style="display: inline;">
					<input type="hidden" name="action" value="approve">
					<input type="hidden" name="type" value="%s">
					<input type="hidden" name="id" value="%s">
					<button type="submit" style="padding: 4px 10px; background: #28a745; color: white; border: none; border-radius: 3px; cursor: pointer; font-size: 12px;">Approve</button>
				</form>
				<form method="POST" action="/moderate" style="display: inline;">
					<input type="hidden" name="action" value="delete">
					<input type="hidden" name="type" value="%s">
					<input type="hidden" name="id" value="%s">
					<button type="submit" style="padding: 4px 10px; background: #dc3545; color: white; border: none; border-radius: 3px; cursor: pointer; font-size: 12px;">Delete</button>
				</form>`,
				item.ContentType, item.ContentID,
				item.ContentType, item.ContentID)
		}

		html := fmt.Sprintf(`<div class="post-item" style="background: #fff3cd; padding: 15px; border-radius: 5px; margin-bottom: 20px;">
			<div style="display: flex; justify-content: space-between; align-items: start; margin-bottom: 10px;">
				<div>
					<span style="background: #666; color: white; padding: 2px 8px; border-radius: 3px; font-size: 12px; text-transform: uppercase;">%s</span>
					<h3 style="margin: 10px 0;">%s</h3>
				</div>
			</div>
			%s
			<div class="info" style="color: #666; font-size: small; margin-top: 10px;">
				%s by %s · Flags: %d · Status: %s<br>
				Flagged by: %s
			</div>
			<div style="margin-top: 15px; display: flex; gap: 10px;">
				%s
				<a href="/%s?id=%s" target="_blank" style="padding: 4px 10px; background: #6c757d; color: white; border: none; border-radius: 3px; text-decoration: none; font-size: 12px; display: inline-block;">View</a>
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

	listHTML := "<p style='color: #666;'>No flagged content. The community is behaving well! 🎉</p>"
	if len(itemsList) > 0 {
		listHTML = strings.Join(itemsList, "\n")
	}

	content := fmt.Sprintf(`<div id="moderation">
		<p style="color: #666; margin-bottom: 20px; padding: 15px; background: #e7f3ff; border-left: 4px solid #007bff; border-radius: 3px;">
			<strong>Community Moderation</strong><br>
			Review content that has been flagged by users. Content is automatically hidden after 3 flags. 
			You can approve (clear flags) or delete the content permanently.
		</p>
		<div id="flagged-content">
			%s
		</div>
	</div>`, listHTML)

	html := app.RenderHTML("Moderate", "Review flagged content", content)
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
		http.Error(w, "Content ID and type required", http.StatusBadRequest)
		return
	}

	// Check if user is admin
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil || !acc.Admin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	switch action {
	case "approve":
		Approve(contentType, contentID)
		http.Redirect(w, r, "/moderate", http.StatusSeeOther)

	case "delete":
		Delete(contentType, contentID)
		http.Redirect(w, r, "/moderate", http.StatusSeeOther)

	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}

// ============================================
// CONTENT INTERFACES
// ============================================

// PostContent represents post data for display
type PostContent struct {
	Title     string
	Content   string
	Author    string
	CreatedAt time.Time
}
