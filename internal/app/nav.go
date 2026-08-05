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

// navLinks renders the lower half of the sidebar: the services kept in view,
// then the way to all the rest.
//
// This was every service with a page — nineteen of them, alphabetical. A grid
// of nineteen is browsable; a list of nineteen is a menu, and a menu of
// nineteen is a failure to choose. Same items, opposite affordance, which is
// why the old home-screen-of-apps never felt confusing and the sidebar did.
//
// So the full set moved to the catalogue at /services, which is derived from
// the same Specs and is also the shop window — a visitor who never sees that
// this instance runs a real mail server has not seen the product. What stays
// here is what changes without you: unread mail, a task the agent finished.
// Declared on the Spec as Pinned, so it is still derived rather than a list
// kept by hand next to the services it names.
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
	fmt.Fprintf(&b, "          <a href=\"/services\"><img src=\"/services.svg?%s\"><span class=\"label\">Services</span></a>",
		Version)
	return b.String()
}
