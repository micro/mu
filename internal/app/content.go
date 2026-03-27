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
	return renderActions(nil, actions)
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
// ItemControls renders save/share icons plus a ⋯ menu for other actions.
// Icons are always visible top-right. Menu contains edit/delete/flag/block.
func ItemControls(userID string, isAdmin bool, contentType, contentID, authorID, editURL, deleteURL string) string {
	var icons []Action
	var menu []Action

	// Save/unsave — visible icon
	if IsSaved(userID, contentType, contentID) {
		icons = append(icons, Action{Label: "Unsave", URL: fmt.Sprintf("/app/unsave?type=%s&id=%s", contentType, contentID)})
	} else {
		icons = append(icons, Action{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)})
	}

	// Share — visible icon
	icons = append(icons, shareAction(contentType, contentID))

	// Menu items — behind ⋯
	isOwner := userID == authorID
	if isOwner || isAdmin {
		if editURL != "" {
			menu = append(menu, Action{Label: "Edit", URL: editURL})
		}
		if deleteURL != "" {
			menu = append(menu, Action{Label: "Delete", URL: deleteURL, Confirm: "Delete this?", Class: "text-error"})
		}
	}
	if userID != "" && !isOwner {
		menu = append(menu, Action{Label: "Flag", URL: fmt.Sprintf("/app/flag?type=%s&id=%s", contentType, contentID), Confirm: "Flag this content?"})
		menu = append(menu, Action{Label: "Block", URL: fmt.Sprintf("/app/block?user=%s", authorID), Confirm: "Block this user?"})
	}
	if userID != "" {
		menu = append(menu, Action{Label: "Dismiss", URL: fmt.Sprintf("/app/dismiss?type=%s&id=%s", contentType, contentID)})
	}

	return renderActions(icons, menu)
}

// ExternalControls renders save/share controls for external content (videos, news, web results).
// These have no author, so no edit/delete/flag/block — just save and share.
func ExternalControls(userID, contentType, contentID string) string {
	var icons []Action
	if userID != "" && IsSaved(userID, contentType, contentID) {
		icons = append(icons, Action{Label: "Unsave", URL: fmt.Sprintf("/app/unsave?type=%s&id=%s", contentType, contentID)})
	} else {
		icons = append(icons, Action{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)})
	}
	icons = append(icons, shareAction(contentType, contentID))
	return renderActions(icons, nil)
}

// StaticControls renders save/share links for cached/static content.
// These don't check the user — the /app/ endpoints handle auth server-side.
// Use this when you don't have request context (cached HTML).
func StaticControls(contentType, contentID string) string {
	return renderActions([]Action{
		{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)},
		shareAction(contentType, contentID),
	}, nil)
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

// renderActions renders visible icons + optional ⋯ dropdown menu.
func renderActions(icons []Action, menu []Action) string {
	if len(icons) == 0 && len(menu) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(`<span style="position:absolute;top:12px;right:12px">`)

	iconStyle := "text-decoration:none;font-size:14px;color:#bbb;cursor:pointer;margin-left:8px"

	for _, a := range icons {
		switch a.Label {
		case "Save":
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" title="Save" onclick="fetch('%s',{method:'POST'});this.textContent='★';this.style.color='#000';return false;">☆</a>`, iconStyle, a.URL))
		case "Unsave":
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s;color:#000" title="Saved" onclick="fetch('%s',{method:'POST'});this.textContent='☆';this.style.color='#bbb';return false;">★</a>`, iconStyle, a.URL))
		case "Share":
			if a.URL != "" && a.URL != "#" {
				sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" title="Copy link" onclick="navigator.clipboard.writeText(location.origin+'%s');this.style.color='#000';setTimeout(function(){this.style.color='#bbb'}.bind(this),1000);return false;">↗</a>`, iconStyle, a.URL))
			}
		}
	}

	// ⋯ menu for remaining actions
	if len(menu) > 0 {
		id := fmt.Sprintf("m%d", menuCounter.Add(1))
		sb.WriteString(fmt.Sprintf(`<a href="#" style="%s;font-size:16px;letter-spacing:-1px" onclick="var m=document.getElementById('%s');m.style.display=m.style.display==='block'?'none':'block';event.stopPropagation();return false;">⋯</a>`, iconStyle, id))
		sb.WriteString(fmt.Sprintf(`<div id="%s" class="ctrl-menu" style="display:none;position:absolute;right:0;top:24px;background:#fff;border:1px solid #e0e0e0;border-radius:6px;box-shadow:0 2px 8px rgba(0,0,0,0.1);z-index:100;min-width:120px;padding:4px 0">`, id))

		menuStyle := "display:block;padding:6px 14px;font-size:13px;text-decoration:none;white-space:nowrap;cursor:pointer"
		for _, a := range menu {
			style := menuStyle
			if a.Class == "text-error" {
				style += ";color:#c00"
			} else {
				style += ";color:#555"
			}

			if a.Label == "Edit" {
				sb.WriteString(fmt.Sprintf(`<a href="%s" style="%s">Edit</a>`, a.URL, style))
			} else if a.Confirm != "" {
				sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="if(confirm('%s')){fetch('%s',{method:'POST'}).then(function(){location.reload()})};return false;">%s</a>`,
					style, a.Confirm, a.URL, a.Label))
			} else {
				sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="fetch('%s',{method:'POST'}).then(function(){this.textContent='Done!'}.bind(this));return false;">%s</a>`,
					style, a.URL, a.Label))
			}
		}
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`</span>`)
	return sb.String()
}

var menuCounter atomic.Int64
