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
// Home, Agent and Tools are not services; they stay written out at the call
// site, in the top group with Usage and Wallet.
//
// Wallet is a service with a page, so it would be derived into this list as
// well and appear twice — once pinned, once filed under W. Pinned wins: it is
// part of operating the instance rather than one more thing the instance can
// do, which is the whole reason for the two groups.
var pinned = map[string]bool{"wallet": true}

func navLinks() string {
	var b strings.Builder
	for _, s := range service.Nav() {
		if pinned[s.Name] {
			continue
		}
		id := ""
		// Mail carries an id the client JS updates (unread badge). It is an
		// anchor on the element, not an extra nav entry.
		if s.Name == "mail" {
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
