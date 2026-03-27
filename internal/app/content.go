package app

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Action is a content control (edit, delete, flag, etc.)
type Action struct {
	Label   string
	URL     string
	Confirm string
	Method  string
	Class   string
}

// Controls renders a ⋯ dropdown with actions. Only shows if user is admin or author.
func Controls(userID, authorID string, isAdmin bool, actions ...Action) string {
	if !isAdmin && userID != authorID {
		return ""
	}
	if len(actions) == 0 {
		return ""
	}
	return renderMenu(actions)
}

// ItemControls renders a ⋯ dropdown with all applicable controls for a content item.
func ItemControls(userID string, isAdmin bool, contentType, contentID, authorID, editURL, deleteURL string) string {
	var actions []Action

	// Save/unsave
	if IsSaved(userID, contentType, contentID) {
		actions = append(actions, Action{Label: "Unsave", URL: fmt.Sprintf("/app/unsave?type=%s&id=%s", contentType, contentID)})
	} else {
		actions = append(actions, Action{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)})
	}

	// Share
	u := contentURL(contentType, contentID)
	if u != "" {
		actions = append(actions, Action{Label: "Share", URL: u})
	}

	// Owner/admin
	isOwner := userID == authorID
	if isOwner || isAdmin {
		if editURL != "" {
			actions = append(actions, Action{Label: "Edit", URL: editURL})
		}
		if deleteURL != "" {
			actions = append(actions, Action{Label: "Delete", URL: deleteURL, Confirm: "Delete this?", Class: "text-error"})
		}
	}

	// Others
	if userID != "" && !isOwner {
		actions = append(actions, Action{Label: "Flag", URL: fmt.Sprintf("/app/flag?type=%s&id=%s", contentType, contentID), Confirm: "Flag this content?"})
		actions = append(actions, Action{Label: "Block", URL: fmt.Sprintf("/app/block?user=%s", authorID), Confirm: "Block this user?"})
	}

	return renderMenu(actions)
}

// ExternalControls renders a ⋯ dropdown for external content (video, news, web).
func ExternalControls(userID, contentType, contentID string) string {
	var actions []Action

	if userID != "" && IsSaved(userID, contentType, contentID) {
		actions = append(actions, Action{Label: "Unsave", URL: fmt.Sprintf("/app/unsave?type=%s&id=%s", contentType, contentID)})
	} else {
		actions = append(actions, Action{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)})
	}

	u := contentURL(contentType, contentID)
	if u != "" {
		actions = append(actions, Action{Label: "Share", URL: u})
	}

	return renderMenu(actions)
}

// StaticControls renders a ⋯ dropdown for cached content (no user context).
func StaticControls(contentType, contentID string) string {
	u := contentURL(contentType, contentID)
	actions := []Action{
		{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)},
	}
	if u != "" {
		actions = append(actions, Action{Label: "Share", URL: u})
	}
	return renderMenu(actions)
}

// contentURL returns the permalink for a content item.
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

var menuCounter atomic.Int64

// renderMenu renders a ⋯ button with a dropdown containing all actions.
func renderMenu(actions []Action) string {
	if len(actions) == 0 {
		return ""
	}

	id := fmt.Sprintf("cm%d", menuCounter.Add(1))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<span style="position:absolute;top:10px;right:10px"><a href="#" style="text-decoration:none;font-size:18px;color:#999;line-height:1" onclick="var m=document.getElementById('%s');m.style.display=m.style.display==='block'?'none':'block';event.stopPropagation();return false;">⋯</a>`, id))
	sb.WriteString(fmt.Sprintf(`<div id="%s" class="ctrl-menu" style="display:none;position:absolute;right:0;top:24px;background:#fff;border:1px solid #e0e0e0;border-radius:6px;box-shadow:0 2px 8px rgba(0,0,0,0.1);z-index:100;min-width:130px;padding:4px 0">`, id))

	itemStyle := "display:block;padding:8px 16px;font-size:13px;text-decoration:none;white-space:nowrap;cursor:pointer;color:#333"

	for _, a := range actions {
		style := itemStyle
		if a.Class == "text-error" {
			style = strings.Replace(style, "color:#333", "color:#c00", 1)
		}

		switch {
		case a.Label == "Share":
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="var u=location.origin+'%s';if(navigator.share){navigator.share({url:u})}else if(navigator.clipboard){navigator.clipboard.writeText(u).then(function(){this.textContent='Copied!'}.bind(this))}else{prompt('Copy link:',u)};return false;">Share</a>`, style, a.URL))
		case a.Label == "Edit":
			sb.WriteString(fmt.Sprintf(`<a href="%s" style="%s">Edit</a>`, a.URL, style))
		case a.Label == "Delete" && a.Confirm != "":
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="if(confirm('%s')){fetch('%s',{method:'POST',redirect:'follow'}).then(function(r){window.location=r.url})};return false;">%s</a>`, style, a.Confirm, a.URL, a.Label))
		case a.Confirm != "":
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="if(confirm('%s')){fetch('%s',{method:'POST'}).then(function(){location.reload()})};return false;">%s</a>`, style, a.Confirm, a.URL, a.Label))
		default:
			sb.WriteString(fmt.Sprintf(`<a href="#" style="%s" onclick="var el=this;fetch('%s',{method:'POST'}).then(function(){el.textContent='%s ✓';el.style.color='#28a745'});event.stopPropagation();return false;">%s</a>`, style, a.URL, a.Label, a.Label))
		}
	}

	sb.WriteString(`</div></span>`)
	return sb.String()
}
