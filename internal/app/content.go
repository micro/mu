package app

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Action is a content control (edit, delete, flag, etc.)
type Action struct {
	Label   string // e.g. "Edit", "Delete", "Flag"
	URL     string // href for links, or POST action for confirm actions
	Confirm string // if set, renders as a POST form with confirm dialog
	Method  string // form method override (e.g. "DELETE") — adds hidden _method field
	Class   string // CSS class (default: "text-muted", use "text-error" for destructive)
}

// Controls renders inline action links (e.g. "Edit · Delete · Flag") for content.
// Only renders if the user is an admin or the content author.
// Returns a string with leading " · " separators, suitable for appending to a meta line.
func Controls(userID, authorID string, isAdmin bool, actions ...Action) string {
	if !isAdmin && userID != authorID {
		return ""
	}
	if len(actions) == 0 {
		return ""
	}
	return renderActions(actions)
}

// ItemControls renders the standard set of controls for any content item.
// Shows different actions based on who you are:
//   - Author/Admin: edit, delete
//   - Logged in: flag, save, dismiss, block, share
//   - Everyone: share
//
// editURL and deleteURL are content-specific. Pass empty to omit.
// contentType and contentID identify the item for generic actions.
// authorID is the content creator's user ID.
// itemURL is the permalink to this item (used for share).
func ItemControls(userID string, isAdmin bool, contentType, contentID, authorID, editURL, deleteURL string) string {
	if userID == "" && !isAdmin {
		// Not logged in — just share
		return renderActions([]Action{
			{Label: "Share", Class: "text-muted", URL: "#", Confirm: ""},
		})
	}

	var actions []Action

	// Owner/admin controls
	isOwner := userID == authorID
	if isOwner || isAdmin {
		if editURL != "" {
			actions = append(actions, Action{Label: "Edit", URL: editURL})
		}
		if deleteURL != "" {
			method := ""
			// If delete URL doesn't end with /delete, assume it needs _method=DELETE
			if !strings.HasSuffix(deleteURL, "/delete") {
				method = "DELETE"
			}
			actions = append(actions, Action{Label: "Delete", URL: deleteURL, Confirm: "Delete this?", Method: method, Class: "text-error"})
		}
	}

	// Anyone logged in (on other people's content)
	if !isOwner {
		actions = append(actions, Action{
			Label: "Flag", URL: fmt.Sprintf("/app/flag?type=%s&id=%s", contentType, contentID),
			Confirm: "Flag this content?",
		})
		actions = append(actions, Action{
			Label: "Block", URL: fmt.Sprintf("/app/block?user=%s", authorID),
			Confirm: fmt.Sprintf("Block this user? You won't see their content."),
		})
	}

	// Any logged in user
	if IsSaved(userID, contentType, contentID) {
		actions = append(actions, Action{
			Label: "Unsave", URL: fmt.Sprintf("/app/unsave?type=%s&id=%s", contentType, contentID),
		})
	} else {
		actions = append(actions, Action{
			Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID),
		})
	}

	actions = append(actions, Action{
		Label: "Dismiss", URL: fmt.Sprintf("/app/dismiss?type=%s&id=%s", contentType, contentID),
	})

	// Share (client-side)
	actions = append(actions, shareAction(contentType, contentID))

	return renderActions(actions)
}

// ExternalControls renders save/share controls for external content (videos, news, web results).
// These have no author, so no edit/delete/flag/block — just save and share.
func ExternalControls(userID, contentType, contentID string) string {
	if userID == "" {
		return renderActions([]Action{shareAction(contentType, contentID)})
	}

	var actions []Action
	if IsSaved(userID, contentType, contentID) {
		actions = append(actions, Action{
			Label: "Unsave", URL: fmt.Sprintf("/app/unsave?type=%s&id=%s", contentType, contentID),
		})
	} else {
		actions = append(actions, Action{
			Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID),
		})
	}
	actions = append(actions, shareAction(contentType, contentID))
	return renderActions(actions)
}

// StaticControls renders save/share links for cached/static content.
// These don't check the user — the /app/ endpoints handle auth server-side.
// Use this when you don't have request context (cached HTML).
func StaticControls(contentType, contentID string) string {
	return renderActions([]Action{
		{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)},
		shareAction(contentType, contentID),
	})
}

// contentURL returns the permalink for a content item based on type and ID.
func contentURL(contentType, contentID string) string {
	switch contentType {
	case "post":
		return "/blog/post?id=" + contentID
	case "work":
		return "/work/" + contentID
	case "app":
		return "/apps/" + contentID
	case "social":
		return "/social/thread?id=" + contentID
	case "video":
		return "/video?id=" + contentID
	case "news":
		return "/news?id=" + contentID
	case "web":
		return "/web/read?url=" + contentID
	default:
		return ""
	}
}

func shareAction(contentType, contentID string) Action {
	return Action{Label: "Share", URL: contentURL(contentType, contentID), Class: "text-muted"}
}

var menuCounter atomic.Int64

func renderActions(actions []Action) string {
	if len(actions) == 0 {
		return ""
	}

	id := fmt.Sprintf("m%d", menuCounter.Add(1))

	var sb strings.Builder
	// Three-dot trigger — floats top-right of nearest positioned parent (card)
	sb.WriteString(fmt.Sprintf(`<span class="dot-menu" style="position:absolute;top:12px;right:12px;z-index:10"><a href="#" class="text-muted" onclick="var m=document.getElementById('%s');m.style.display=m.style.display==='block'?'none':'block';event.stopPropagation();return false;" style="text-decoration:none;font-size:18px;color:#999">⋯</a>`, id))

	// Dropdown
	sb.WriteString(fmt.Sprintf(`<div id="%s" style="display:none;position:absolute;right:0;top:24px;background:#fff;border:1px solid #e0e0e0;border-radius:6px;box-shadow:0 2px 8px rgba(0,0,0,0.1);z-index:100;min-width:120px;padding:4px 0">`, id))

	itemStyle := "display:block;padding:6px 14px;font-size:13px;text-decoration:none;white-space:nowrap;cursor:pointer"

	for _, a := range actions {
		style := itemStyle
		if a.Class == "text-error" {
			style += ";color:#c00"
		} else {
			style += ";color:#555"
		}

		// Share — copy link to clipboard
		if a.Label == "Share" && a.URL != "" && a.URL != "#" {
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="navigator.clipboard.writeText(location.origin+'%s').then(function(){this.textContent='Copied!'}.bind(this));return false;">Share</a>`, style, a.URL))
			continue
		}

		// Edit — plain link
		if a.Label == "Edit" {
			sb.WriteString(fmt.Sprintf(`<a href="%s" style="%s">Edit</a>`, a.URL, style))
			continue
		}

		// Everything else — fetch POST, stay on page
		if a.Confirm != "" {
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="if(confirm('%s')){fetch('%s',{method:'POST'}).then(function(){location.reload()})};return false;">%s</a>`,
				style, a.Confirm, a.URL, a.Label))
		} else {
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="fetch('%s',{method:'POST'}).then(function(){this.textContent='Done!'}.bind(this));return false;">%s</a>`,
				style, a.URL, a.Label))
		}
	}

	sb.WriteString(`</div></span>`)
	return sb.String()
}
