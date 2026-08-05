package app

import (
	"fmt"
	"strings"

	"mu/internal/service"
)

// navPlaceholder marks where the sidebar entries go in Template.
//
// Template is a package-level var, evaluated at init — before main() registers
// any service. Building the nav into it there produced a sidebar with only
// Home and Agent, because service.Nav() was still empty. The entries are
// substituted per render instead.
const navPlaceholder = "<!--mu:nav-->"

// withNav fills in the sidebar on a rendered page.
func withNav(page string) string {
	return strings.Replace(page, navPlaceholder, navLinks(), 1)
}

// navLinks renders the services kept in view — the ones that change without
// you, so you would want to notice.
//
// This used to be the whole set of nineteen, alphabetically, and then a link to
// the catalogue. Both moved: the catalogue is a noun in the nav now, beside
// Agent, Tasks, Apps and Tools, and what is left here is the short list of
// services with something new in them.
func navLinks() string {
	var b strings.Builder
	for _, s := range service.Pinned() {
		id, extra := "", ""
		// Mail carries an id and a badge the client JS updates.
		if s.Name == "mail" {
			id, extra = ` id="nav-mail"`, `<span id="nav-mail-badge"></span>`
		}
		fmt.Fprintf(&b, "          <a%s href=\"%s\"><img src=\"/%s?%s\"><span class=\"label\">%s</span>%s</a>\n",
			id, s.Page, s.NavIcon(), Version, s.NavLabel(), extra)
	}
	return strings.TrimRight(b.String(), "\n")
}
