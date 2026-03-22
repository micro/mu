package admin

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/flag"
)

// Re-export types from moderation subsystem for backward compatibility
// in admin dashboard handlers.
type PostContent = flag.PostContent
type FlaggedItem = flag.FlaggedItem
type ContentDeleter = flag.ContentDeleter
type LLMAnalyzer = flag.LLMAnalyzer

// Import blog to get new account blog posts - will be set by blog package to avoid circular import
var GetNewAccountBlog func() []PostContent

// RefreshBlogCache is set by blog package to refresh cache after account approval
var RefreshBlogCache func()

// Delegated functions — building blocks should import internal/moderation directly.
// These exist only so admin's own handlers can call them.
var (
	RegisterDeleter = flag.RegisterDeleter
	SetAnalyzer     = flag.SetAnalyzer
	CheckContent    = flag.CheckContent
	IsHidden        = flag.IsHidden
	AdminFlag       = flag.AdminFlag
)

func Load() {
	flag.Load()
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
	count, alreadyFlagged, err := flag.Add(contentType, contentID, flagger)
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
		if deleter, ok := flag.GetDeleter(contentType); ok {
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

	flaggedItems := flag.GetAll()

	var itemsList []string
	for _, item := range flaggedItems {
		var contentHTML string
		var title string
		var author string
		var createdAt string

		// Get content from the appropriate handler
		if deleter, ok := flag.GetDeleter(item.ContentType); ok {
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
				title = "News Article"
				contentHTML = `<p>[News content]</p>`
			case "video":
				title = "Video"
				contentHTML = `<p>[Video content]</p>`
			}
		}

		status := "Under review"
		if item.Flagged {
			status = "Hidden"
		}

		actionButtons := fmt.Sprintf(`
				<form method="POST" action="/admin/moderate">
					<input type="hidden" name="action" value="approve">
					<input type="hidden" name="type" value="%s">
					<input type="hidden" name="id" value="%s">
					<button type="submit" class="btn-approve">Approve</button>
				</form>
				<form method="POST" action="/admin/moderate" onsubmit="event.preventDefault(); muConfirm('Permanently delete this content?').then(function(ok){if(ok)event.target.submit()})">
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
					%s by %s · New Account (&lt; 24h) · Hidden from homepage
				</div>
				<div class="actions">
					<form method="POST" action="/admin/moderate">
						<input type="hidden" name="action" value="approve_account">
						<input type="hidden" name="type" value="post">
						<input type="hidden" name="id" value="%s">
						<button type="submit" class="btn-approve">Approve</button>
					</form>
					<form method="POST" action="/admin/moderate" onsubmit="event.preventDefault(); muConfirm('Flag this post?').then(function(ok){if(ok){fetch('/admin/flag',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({type:'post',id:'%s'})}).then(r=>r.json()).then(d=>{if(d.success){location.reload()}else{alert(d.message||'Failed')}}).catch(()=>alert('Error'))}});return false;">
						<button type="submit" class="btn-delete">Flag</button>
					</form>
					<a href="/blog/post?id=%s" target="_blank">view</a>
				</div>
			</div>`,
					title,
					content,
					app.TimeAgo(post.CreatedAt),
					post.Author,
					post.AuthorID,
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

	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	switch action {
	case "approve":
		flag.Approve(contentType, contentID)
		http.Redirect(w, r, "/admin/moderate", http.StatusSeeOther)

	case "delete":
		flag.Delete(contentType, contentID)
		http.Redirect(w, r, "/admin/moderate", http.StatusSeeOther)

	case "approve_account":
		if err := auth.ApproveAccount(contentID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if RefreshBlogCache != nil {
			RefreshBlogCache()
		}
		http.Redirect(w, r, "/admin/moderate", http.StatusSeeOther)

	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}
