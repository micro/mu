package app

import (
	"fmt"
	"strings"
)

// AdminAction is a single admin control (edit, delete, flag, etc.)
type AdminAction struct {
	Label   string // e.g. "Edit", "Delete", "Flag"
	URL     string // href for links, or POST action for confirm actions
	Confirm string // if set, renders as a POST form with confirm dialog
	Method  string // form method override (e.g. "DELETE") — adds hidden _method field
	Class   string // CSS class (default: "text-muted", use "text-error" for destructive)
}

// AdminControls renders inline admin action links (matching blog's "Edit · Delete" style).
// Only renders if the user is an admin or the content author.
// Returns a string with leading " · " separators, suitable for appending to a meta line.
func AdminControls(userID, authorID string, isAdmin bool, actions ...AdminAction) string {
	if !isAdmin && userID != authorID {
		return ""
	}
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
		if a.Confirm != "" {
			// POST with confirm dialog — inline JS form submission
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
