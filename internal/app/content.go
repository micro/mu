package app

import (
	"fmt"
	"strings"
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
	actions = append(actions, shareAction())

	return renderActions(actions)
}

// ExternalControls renders save/share controls for external content (videos, news, web results).
// These have no author, so no edit/delete/flag/block — just save and share.
func ExternalControls(userID, contentType, contentID string) string {
	if userID == "" {
		return renderActions([]Action{shareAction()})
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
	actions = append(actions, shareAction())
	return renderActions(actions)
}

// StaticControls renders save/share links for cached/static content.
// These don't check the user — the /app/ endpoints handle auth server-side.
// Use this when you don't have request context (cached HTML).
func StaticControls(contentType, contentID string) string {
	return renderActions([]Action{
		{Label: "Save", URL: fmt.Sprintf("/app/save?type=%s&id=%s", contentType, contentID)},
		shareAction(),
	})
}

func shareAction() Action {
	return Action{Label: "Share", URL: "#", Class: "text-muted"}
}

func renderActions(actions []Action) string {
	if len(actions) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, a := range actions {
		sb.WriteString(` · `)
		cls := a.Class
		if cls == "" {
			cls = "text-muted"
		}

		// Share is client-side
		if a.Label == "Share" {
			sb.WriteString(`<a href="#" class="text-muted" onclick="navigator.share ? navigator.share({title: document.title, url: window.location.href}) : navigator.clipboard.writeText(window.location.href).then(() => alert('Link copied!')); return false;">Share</a>`)
			continue
		}

		if a.Confirm != "" {
			methodField := ""
			if a.Method != "" {
				methodField = fmt.Sprintf("var i=document.createElement('input');i.type='hidden';i.name='_method';i.value='%s';f.appendChild(i);", a.Method)
			}
			sb.WriteString(fmt.Sprintf(`<a href="#" class="%s" onclick="if(confirm('%s')){var f=document.createElement('form');f.method='POST';f.action='%s';%sdocument.body.appendChild(f);f.submit();}return false;">%s</a>`,
				cls, a.Confirm, a.URL, methodField, a.Label))
		} else {
			sb.WriteString(fmt.Sprintf(`<a href="%s" class="%s">%s</a>`, a.URL, cls, a.Label))
		}
	}
	return sb.String()
}
