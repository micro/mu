package news

// When the card is from.
//
// A card that knows this is a card that can be put in a stream — see
// service.Card. Without it the home screen had to be told, by hand, in a JSON
// file kept beside the services it named, which is why the ordering there was
// a configuration rather than a fact.

import "time"

// CardAt is when the newest headline on the card was published.
//
// The newest rather than the fetch time: what a reader wants to know is how old
// the news is, and a feed we polled a minute ago can be carrying yesterday's.
func CardAt() time.Time {
	mutex.RLock()
	defer mutex.RUnlock()
	var newest time.Time
	for _, p := range feed {
		if p != nil && p.PostedAt.After(newest) {
			newest = p.PostedAt
		}
	}
	return newest
}
