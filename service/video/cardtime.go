package video

import "time"

// CardAt is when the newest video on the card was published.
func CardAt() time.Time {
	if latest := LatestVideos(1); len(latest) > 0 && latest[0] != nil {
		return latest[0].Published
	}
	return time.Time{}
}
