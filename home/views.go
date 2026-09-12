package home

import (
	"html"
	"strings"

	"mu/internal/auth"
)

// greeting avoids using the server timezone for a person's local time of day.
func greeting(acc *auth.Account) string {
	if acc != nil {
		if name := strings.TrimSpace(acc.Name); name != "" {
			return "Welcome back, " + html.EscapeString(name)
		}
	}
	return "Welcome back"
}
