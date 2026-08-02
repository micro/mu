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

// navLinks renders the sidebar entries for every service that has a page,
// derived from the service Specs.
//
// This was a hand-written list of eighteen anchors. Nothing connected an entry
// to the service behind it, so a service could be added and never appear, a
// route could move and the link rot, and two services could quietly share an
// icon — Stream and Chat both showed a speech bubble for months, because no
// list can notice a repeat in itself.
//
// Home and Agent are not services; they stay written out at the call site.
func navLinks() string {
	var b strings.Builder
	for _, s := range service.Nav() {
		id := ""
		// Mail and Wallet carry ids the client JS updates (unread badge,
		// balance). They are anchors on the element, not extra nav entries.
		switch s.Name {
		case "mail", "wallet":
			id = fmt.Sprintf(` id="nav-%s"`, s.Name)
		}
		extra := ""
		if s.Name == "mail" {
			extra = `<span id="nav-mail-badge"></span>`
		}
		fmt.Fprintf(&b, "          <a%s href=\"%s\"><img src=\"/%s?%s\"><span class=\"label\">%s</span>%s</a>\n",
			id, s.Page, s.NavIcon(), Version, s.NavLabel(), extra)
	}
	return strings.TrimRight(b.String(), "\n")
}
