package app

import (
	"fmt"
	"html"
	"mu/internal/auth"
	"mu/internal/group"
	"net/url"
	"strings"
)

func spaceSelector(acc *auth.Account, here string) (string, string) {
	if acc == nil {
		return "", ""
	}
	gs, _, err := group.List(acc.ID)
	if err != nil {
		return "", ""
	}
	active := ""
	label := "Personal"
	u, _ := url.Parse(here)
	if u != nil {
		if u.Path == "/groups" {
			active = u.Query().Get("id")
		}
		if u.Path == "/chat" {
			active = group.RoomGroup(u.Query().Get("id"))
		}
	}
	valid := false
	for _, g := range gs {
		if g.ID == active {
			label = g.Name
			valid = true
		}
	}
	if !valid {
		active = ""
	}
	current := func(id string) string {
		if id == active {
			return ` aria-current="page"`
		}
		return ""
	}
	var b strings.Builder
	b.WriteString(`<details class="space-selector"><summary aria-label="Switch space"><span>` + html.EscapeString(label) + `</span></summary><nav class="space-options" aria-label="Spaces"><a href="/home"` + current("") + `>Personal</a>`)
	for _, g := range gs {
		b.WriteString(`<a href="/groups?id=` + g.ID + `"` + current(g.ID) + `>` + html.EscapeString(g.Name) + func() string {
			if n := group.Unread(g, acc.ID); n > 0 {
				return fmt.Sprintf(" (%d)", n)
			}
			return ""
		}() + `</a>`)
	}
	b.WriteString(`<a class="space-menu-action" href="/groups?new=1">Create group</a><a href="/groups">Groups and invitations</a></nav></details>`)
	return b.String(), active
}
