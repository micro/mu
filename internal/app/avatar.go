package app

import (
	"html"
	"mu/internal/auth"
	"strings"
	"unicode/utf8"
)

// Avatar shows an account photo, falling back to its display-name initial.
func Avatar(acc *auth.Account) string {
	if strings.HasPrefix(acc.Avatar, "data:image/jpeg;base64,") {
		return `<img class="account-avatar" src="` + html.EscapeString(acc.Avatar) + `" alt="" width="30" height="30">`
	}
	name := strings.TrimSpace(acc.Name)
	if name == "" {
		name = acc.ID
	}
	initial, _ := utf8.DecodeRuneInString(name)
	return `<span class="account-avatar" aria-hidden="true">` + html.EscapeString(strings.ToUpper(string(initial))) + `</span>`
}
