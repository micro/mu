package blog

// Finding a post by what it is called.
//
// Every blog tool took an id and nothing else, so an agent asked to "read the
// post about the migration" had to call blog_list first, scan for a title, copy
// a uuid and call again. That is two model calls and a chance to copy the wrong
// one, to do something a person would express in one sentence.
//
// The id still works and is still what List returns. This adds the name.

import "strings"

// ByTitle returns the post whose title best matches, or nil.
//
// Exact match first, then a unique prefix, then a unique substring. Ambiguity
// returns nil rather than a guess: picking one of two plausible posts and
// deleting it is the failure this must not have.
func ByTitle(title string) *Post {
	want := strings.ToLower(strings.TrimSpace(title))
	if want == "" {
		return nil
	}
	mutex.RLock()
	all := append([]*Post(nil), posts...)
	mutex.RUnlock()

	var prefix, substr []*Post
	for _, p := range all {
		got := strings.ToLower(strings.TrimSpace(p.Title))
		switch {
		case got == want:
			return p
		case strings.HasPrefix(got, want):
			prefix = append(prefix, p)
		case strings.Contains(got, want):
			substr = append(substr, p)
		}
	}
	if len(prefix) == 1 {
		return prefix[0]
	}
	if len(prefix) == 0 && len(substr) == 1 {
		return substr[0]
	}
	return nil
}

// Resolve returns the post for an id, or failing that a title. It is what a
// handler should call when it accepts either.
func Resolve(id, title string) *Post {
	if id = strings.TrimSpace(id); id != "" {
		if p := GetPost(id); p != nil {
			return p
		}
	}
	return ByTitle(title)
}
