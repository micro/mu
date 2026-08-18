package social

import "time"

// CardAt is when the newest message on the card was posted.
func CardAt() time.Time {
	mutex.RLock()
	defer mutex.RUnlock()
	var newest time.Time
	for _, m := range messages {
		if m != nil && m.PostedAt.After(newest) {
			newest = m.PostedAt
		}
	}
	return newest
}
