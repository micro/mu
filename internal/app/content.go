package app

import (
	"fmt"
	"html"
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

	// Reporting it, and nothing else somebody can do to another person's item.
	//
	// Save, Hide and Block author were here, with a page at /user listing what
	// you had chosen. They are gone, and the reason is what they are for: those
	// three are the controls of a feed — somewhere strangers' content is put in
	// front of you by default and you need a way to push it back. Mu has no
	// feed. Its surfaces are your inbox, your agents and your tools, so nothing
	// arrives unasked and there is nothing to hide from. They were not
	// underused; they were unmotivated.
	//
	// Report survives because it is not one of those three. It is not about
	// what you see — it is telling an operator that something here should not
	// be, which stays true on an instance with a blog and public threads on it.
	if !isOwner {
		actions = append(actions,
			Action{Label: "Report", URL: "/report?type=" + contentType + "&id=" + contentID,
				Confirm: "Report this for a moderator to look at?"})
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
			sb.WriteString(deleteItem(style, a))
		case a.Confirm != "":
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="if(confirm('%s')){fetch('%s',{method:'POST'}).then(function(){location.reload()})};return false;">%s</a>`, style, a.Confirm, a.URL, a.Label))
		default:
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="var el=this;fetch('%s',{method:'POST'}).then(function(){el.textContent='Done';el.style.color='#1a7f37'});event.stopPropagation();return false;">%s</a>`, style, a.URL, a.Label))
		}
	}

	sb.WriteString(`</div></span>`)
	return sb.String()
}

// deleteItem is the Delete line in that menu, and the request it sends.
//
// # It sent a request no handler recognised
//
// The comment here read "Use POST (not DELETE) — handlers check for POST",
// which is half of the contract. They check for POST *and* a _method=DELETE
// field, because a browser form cannot issue a DELETE; this sent a bare POST
// with no body at all. On the blog that fell through every branch to the page
// renderer and came back 200 with the post still there. On /social it was
// worse: the method switch reaches "case POST" before the _method check, so
// deleting a message called handleCreateThread.
//
// Then the handler's answer was thrown away — `.then(function(){...})` with no
// look at r.ok — and the page navigated regardless. So both failures looked
// exactly like success: confirm, redirect, item still there. A person doing
// this has no way to tell it did not work except by going back and looking.
//
// # So
//
// The body carries _method=DELETE, which is what every one of these handlers
// is waiting for. The response is checked, and a refusal says what it was
// rather than pretending. Only a real delete navigates.
func deleteItem(style string, a Action) string {
	// Where to go afterwards. The thing that was on this page is gone, so
	// "back" has to mean its listing and not the page itself — the referrer is
	// the fallback, and for a permalink that is usually where you came from.
	after := "document.referrer||'/'"
	switch {
	case strings.HasPrefix(a.URL, "/apps/"):
		after = "'/apps'"
	case strings.HasPrefix(a.URL, "/social"):
		after = "'/social'"
	case strings.HasPrefix(a.URL, "/blog"):
		after = "'/blog'"
	}

	js := `if(!confirm('` + a.Confirm + `'))return false;` +
		`var h={'Content-Type':'application/x-www-form-urlencoded'};` +
		`var t=(document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)||[])[1];` +
		`if(t)h['X-CSRF-Token']=decodeURIComponent(t);` +
		`fetch('` + a.URL + `',{method:'POST',credentials:'same-origin',headers:h,body:'_method=DELETE'})` +
		`.then(function(r){if(!r.ok){throw new Error(r.status===403?'That is not yours to delete.':'Could not delete that ('+r.status+').')}` +
		`window.location=` + after + `})` +
		`.catch(function(e){alert(e.message||'Could not delete that.')});return false;`

	return fmt.Sprintf(`<a href="#" class="%s" onclick="%s">%s</a>`,
		style, html.EscapeString(js), a.Label)
}
