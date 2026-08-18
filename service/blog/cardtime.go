package blog

import "time"

// CardAt is when the newest post shown on the card was written.
func CardAt() time.Time {
	mutex.RLock()
	defer mutex.RUnlock()
	var newest time.Time
	for _, p := range posts {
		// Private posts are not on the card, so they do not date it.
		if p == nil || p.Private {
			continue
		}
		if p.CreatedAt.After(newest) {
			newest = p.CreatedAt
		}
	}
	return newest
}
