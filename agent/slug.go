package agent

// An agent has an address in the app, not just a query parameter.
//
// `/agent?id=2d5e1f4d-edd1-488c-9b49-5fb2bf7e518f` is a page you cannot say out
// loud, cannot bookmark meaningfully and cannot tell apart from another one at
// a glance. `/agent/micro` is the same page with a name on it, the way `/mail`
// and `/news` are — and it is what makes an agent a place rather than a
// selection you happen to be holding.
//
// The id still works. Links to it exist, /agents wrote them, and breaking a URL
// to tidy a parameter is a bad trade — so `?id=` resolves and redirects to the
// name, which is one hop and leaves the address bar saying something true.

import (
	"strings"
	"unicode"
)

// DefaultSlug is the built-in agent's name in a path. It has no roster entry —
// it is this account talking to the instance — so it needs a name of its own to
// be addressable at all.
const DefaultSlug = "micro"

// Slug is an agent's name as it appears in a path.
//
// The mail tag where there is one, because an agent that answers at
// `you+research@` should be at `/agent/research`: one name for the same thing
// is the whole reason tags exist. Otherwise the display name, lowercased with
// anything that is not a letter or digit removed — the same shape tagFor
// produces, so the two agree for every agent that has both.
func Slug(a *Agent) string {
	if a == nil {
		return DefaultSlug
	}
	if a.Tag != "" {
		return a.Tag
	}
	return slugify(a.Name)
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// BySlug resolves a name from a path to an agent id, and reports whether the
// name named anything at all.
//
// The default agent answers to DefaultSlug and has an empty id, which is what
// the rest of this package already means by "no particular agent". A name that
// matches nothing is not the default — that would quietly serve the wrong agent
// for a typo, and a page about a specific thing should 404 rather than show a
// different thing.
func BySlug(owner, slug string) (id string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || slug == DefaultSlug {
		return "", true
	}
	all := Agents(owner)
	for _, a := range all {
		if Slug(a) == slug {
			return a.ID, true
		}
		// The id itself, so a link written before names existed still lands.
		if strings.EqualFold(a.ID, slug) {
			return a.ID, true
		}
	}
	// A name it used to have. Renaming an agent moves its page and its endpoint
	// to the new word, and whatever was pointing at the old one — a bookmark, a
	// cron job, a curl in somebody's notes — keeps working. Checked after every
	// live name, so an agent that has since taken the word answers to it.
	for _, a := range all {
		for _, f := range a.Former {
			if strings.EqualFold(f, slug) {
				return a.ID, true
			}
		}
	}
	// Then this instance's own — news, markets, weather, the rest.
	//
	// Eleven of them have existed since the router was written, each with its
	// own instruction and its own tools, and every one was reachable at
	// agent+news@ and at nothing else. Not a page, not a link, not the picker.
	// A specialist you can only reach if you already know the plus-address
	// convention and the name is one nobody has.
	//
	// Your roster wins on a collision, which is the same rule the addresses
	// use: your namespace is yours, and a built-in agent must never take over a
	// name you were already using.
	if a := Platform(slug); a != nil {
		return a.ID, true
	}
	return "", false
}

// SlugFor is the path name of one of an owner's agents by id, for building
// links. Empty id is the default agent.
func SlugFor(owner, id string) string {
	if id == "" {
		return DefaultSlug
	}
	if a := For(owner, id); a != nil {
		return Slug(a)
	}
	// One of this instance's own. Its id is already its name — see the registry
	// in agent/micro — so it is its own slug.
	if platformName(id) != "" {
		return strings.ToLower(id)
	}
	return DefaultSlug
}

// Path is where to find a conversation with this agent.
func Path(owner, id string) string { return "/agent/" + SlugFor(owner, id) }

// agentSlugTarget resolves a path name for whoever is looking.
//
// It took a guest flag: a signed-out visitor had no roster, so the only agent
// they could be on a page about was the default one. There are no signed-out
// visitors on these pages now — /agent redirects to the login — so there is one
// answer.
func agentSlugTarget(accountID, slug string) (string, bool) {
	return BySlug(accountID, slug)
}

// agentTitle is what to call the agent this page is about.
func agentTitle(accountID, id string) string {
	if id == "" {
		return "Micro"
	}
	if a := For(accountID, id); a != nil {
		return a.Name
	}
	if n := platformName(id); n != "" {
		return n
	}
	return "Micro"
}
