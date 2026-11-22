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

// Slug is the server-reserved URL name. It is independent of the display name
// and owner-scoped email tag, and remains reserved after deletion.
func Slug(a *Agent) string {
	if a == nil {
		return DefaultSlug
	}
	reserved, err := names()
	if err == nil {
		for name, ref := range reserved {
			if ref.Owner == a.Owner && ref.ID == a.ID {
				return name
			}
		}
	}
	// Fail closed to the owner's opaque ID if the index is unavailable.
	return a.ID
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

// BySlug resolves the server-wide name, then checks execution ownership.
// A different account can never resolve the same name to its own agent.
func BySlug(owner, slug string) (id string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || slug == DefaultSlug {
		return "", true
	}
	if a := Platform(slug); a != nil {
		return a.ID, true
	}
	a, err := namedAgent(slug)
	if err != nil {
		return "", false
	}
	if a != nil {
		return a.ID, a.Owner == owner
	}
	// Legacy opaque IDs remain owner-scoped; ambiguous old names do not.
	if a := For(owner, slug); a != nil {
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

// agentSlugTarget resolves a reserved URL name with the caller's permissions.
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
