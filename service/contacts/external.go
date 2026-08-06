package contacts

// The address book somebody already has.
//
// contacts exists so a name resolves to an address, and it could only resolve
// names somebody had typed into it. Almost nobody types their address book in
// twice, so "email Sarah about Thursday" failed for the ordinary reason that Mu
// had never heard of Sarah — the exact gap this service was created to close,
// still open.
//
// Resolved, not imported. Google's contacts include everyone the person has
// ever mailed, so a sync would drop thousands of junk cards into a list they
// curate by hand; and it would leave Mu holding a copy of their social graph at
// rest, permanently, whether or not they ever ask a question that needs it.
// Looking up on demand costs one request and keeps nothing.
//
// Same hook pattern as everywhere else: this package does not import a client
// for anybody's account, and an instance with nothing attached simply resolves
// against its own book, which is what it did before.

import "strings"

// External is somebody found in an address book Mu does not own. No id, because
// Mu holds no record to refer back to.
type External struct {
	Name   string
	Email  string
	Phone  string
	Source string
}

var (
	// ExternalFind resolves a query against an attached address book.
	ExternalFind func(owner, query string) []External

	// ExternalConnected reports whether this owner has attached one, so the UI
	// can tell "nobody by that name" apart from "nowhere to look".
	ExternalConnected func(owner string) bool

	// ExternalName is what to call it in front of a person.
	ExternalName = "Google Contacts"
)

// externalFind is the guarded call: an unset hook, or an owner who never
// connected anything, resolves to nothing rather than failing.
func externalFind(owner, query string) []External {
	if ExternalFind == nil || owner == "" || strings.TrimSpace(query) == "" {
		return nil
	}
	return ExternalFind(owner, query)
}

// HasExternal reports whether this owner has an outside address book attached.
func HasExternal(owner string) bool {
	return ExternalConnected != nil && owner != "" && ExternalConnected(owner)
}

// CanConnectExternal reports whether attaching one is offered on this instance.
func CanConnectExternal() bool { return ExternalConnected != nil }

// FindEverywhere resolves a name against Mu's own book first, then the attached
// one, dropping anybody already answered for.
//
// Mu's own first because it is the curated one: a person who wrote down an
// address here meant that address, and an auto-suggested duplicate from a
// mail-derived contact should not outrank it.
func FindEverywhere(owner, query string) ([]*Contact, []External) {
	own := Find(owner, query)

	seen := map[string]bool{}
	for _, c := range own {
		if c.Email != "" {
			seen[strings.ToLower(c.Email)] = true
		}
	}

	var ext []External
	for _, p := range externalFind(owner, query) {
		if p.Email != "" && seen[strings.ToLower(p.Email)] {
			continue
		}
		if p.Email != "" {
			seen[strings.ToLower(p.Email)] = true
		}
		p.Source = ExternalName
		ext = append(ext, p)
	}
	return own, ext
}

// RenderExternal writes outside matches the way Render writes Mu's own, minus
// the id — there is nothing to refer back to, and offering one would be
// offering something that fails.
func RenderExternal(people []External) string {
	if len(people) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range people {
		b.WriteString("- " + p.Name)
		if p.Email != "" {
			b.WriteString(" <" + p.Email + ">")
		}
		if p.Phone != "" {
			b.WriteString(" tel:" + p.Phone)
		}
		source := p.Source
		if source == "" {
			source = ExternalName
		}
		b.WriteString(" [" + source + "]\n")
	}
	return strings.TrimSpace(b.String())
}

// connectHint tells the caller when a name was looked for in one book while the
// person keeps another. Placed in the answer because the agent is usually the
// one reading it — it is how "I don't know a Sarah" becomes "I don't know a
// Sarah, and I could look in the address book you already have".
func connectHint(owner string) string {
	if !CanConnectExternal() || HasExternal(owner) {
		return ""
	}
	return "\n\n(Only your Mu address book was searched. Connect your " +
		ExternalName + " at /contacts to resolve names from it too.)"
}
