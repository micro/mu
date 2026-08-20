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

// ItemControls renders a ⋯ dropdown with all applicable controls for a content item.
func ItemControls(userID string, isAdmin bool, contentType, contentID, authorID, editURL, deleteURL string) string {
	if userID == "" {
		return ""
	}
	isOwner := userID == authorID

	var actions []Action

	// What anybody signed in can do with somebody else's item.
	//
	// These existed as tools and as a page listing what you had saved, hidden
	// and blocked, and nothing in the interface ever wrote any of it — so the
	// lists were empty by construction and /user was a room with nothing in it.
	// The menu is where the item is, which is the only place a decision about
	// an item gets made.
	actions = append(actions,
		Action{Label: "Save", URL: "/user/save?type=" + contentType + "&id=" + contentID},
		Action{Label: "Hide", URL: "/user/hide?type=" + contentType + "&id=" + contentID})
	if !isOwner {
		actions = append(actions,
			Action{Label: "Report", URL: "/user/flag?type=" + contentType + "&id=" + contentID,
				Confirm: "Report this for a moderator to look at?"})
		if authorID != "" {
			actions = append(actions, Action{Label: "Block author",
				URL:     "/user/block?user=" + authorID,
				Confirm: "Block this account? You will stop seeing everything they post.",
				Class:   "text-error"})
		}
	}

	// And what only its owner, or an operator, can.
	if isOwner || isAdmin {
		if editURL != "" {
			actions = append(actions, Action{Label: "Edit", URL: editURL})
		}
		if deleteURL != "" {
			actions = append(actions, Action{Label: "Delete", URL: deleteURL, Confirm: "Delete this?", Class: "text-error"})
		}
	}
	if len(actions) == 0 {
		return ""
	}
	return renderMenu(actions)
}

// StaticControls is a no-op — cached content (news, video, blog listings)
// shouldn't have per-item dropdown menus. They add noise.
func StaticControls(contentType, contentID string) string {
	return ""
}

// contentURL returns the permalink for a content item.
func contentURL(contentType, contentID string) string {
	switch contentType {
	case "post":
		return "/blog/post?id=" + contentID
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
	sb.WriteString(fmt.Sprintf(`<span class="corner"><a href="#" class="corner-x" onclick="var m=document.getElementById('%s');m.style.display=m.style.display==='block'?'none':'block';event.stopPropagation();return false;">⋯</a>`, id))
	sb.WriteString(fmt.Sprintf(`<div id="%s" class="ctrl-menu dropdown">`, id))

	// The menu item's look is .menu-item, not a style string assembled here and
	// interpolated into six format calls. The one thing that varied was the
	// colour of a destructive action, which is a modifier.
	for _, a := range actions {
		style := "menu-item"
		if a.Class == "text-error" {
			style += " danger"
		}

		switch {
		case a.Label == "Share":
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="var u=location.origin+'%s';if(navigator.share){navigator.share({url:u})}else if(navigator.clipboard){navigator.clipboard.writeText(u).then(function(){this.textContent='Copied!'}.bind(this))}else{prompt('Copy link:',u)};return false;">Share</a>`, style, a.URL))
		case a.Label == "Edit":
			sb.WriteString(fmt.Sprintf(`<a href="%s" class="%s">Edit</a>`, a.URL, style))
		case a.Label == "Delete" && a.Confirm != "":
			// Use POST (not DELETE) — handlers check for POST.
			// Redirect to the parent listing page, derived from the URL pattern.
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="if(confirm('%s')){var h={};var t=(document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)||[])[1];if(t)h['X-CSRF-Token']=decodeURIComponent(t);fetch('%s',{method:'POST',credentials:'same-origin',headers:h}).then(function(){var p='%s';if(p.indexOf('/apps/')===0)window.location='/apps';else if(p.indexOf('/social')===0)window.location='/social';else window.location=document.referrer||'/'})};return false;">%s</a>`, style, a.Confirm, a.URL, a.URL, a.Label))
		case a.Confirm != "":
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="if(confirm('%s')){fetch('%s',{method:'POST'}).then(function(){location.reload()})};return false;">%s</a>`, style, a.Confirm, a.URL, a.Label))
		default:
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="var el=this;fetch('%s',{method:'POST'}).then(function(){el.textContent='Done';el.style.color='#1a7f37'});event.stopPropagation();return false;">%s</a>`, style, a.URL, a.Label))
		}
	}

	sb.WriteString(`</div></span>`)
	return sb.String()
}
