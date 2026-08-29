package apps

// Somebody's own apps.
//
// Exported because the page that builds them is not in this package: /code
// composes this service with the shell and the agent, and a composer has to sit
// above the things it composes. Reading them is apps' job either way — the
// store and the lock are here, and handing either of those out would be worse
// than handing out the answer.

import "sort"

// AuthoredBy is an account's own apps, most recently worked on first — which is
// the order somebody coming back to one wants, not alphabetical.
func AuthoredBy(accountID string) []*App {
	mutex.RLock()
	defer mutex.RUnlock()
	var out []*App
	for _, a := range apps {
		if a.AuthorID == accountID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
