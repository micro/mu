package video

import (
	"fmt"
	"strings"
)

// LatestText returns a compact, model-ready list of the latest videos.
// It is the AI-first accessor behind the video agent tool.
func LatestText(limit int) string {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	vids := LatestVideos(limit)
	if len(vids) == 0 {
		// Which of the two it is. "No videos available right now" is what an
		// instance with no YouTube key said, and it is not true: there are
		// videos, there is no key. An agent reads it as nothing to report and
		// moves on, the operator is never told, and a self-hoster waits for
		// something that is never coming.
		//
		// Same reasoning as service/web, which has the only other copy of it —
		// and had it alone for months while every other key-gated service
		// reported a missing key as an empty result.
		if !Configured() {
			return "Video is not configured on this instance. The operator needs to " +
				"set a YouTube API key (YOUTUBE_API_KEY) — it is one field on /admin/config."
		}
		return "No videos available right now."
	}

	var sb strings.Builder
	sb.WriteString("Latest videos:\n")
	for _, v := range vids {
		if v == nil {
			continue
		}
		title := strings.TrimSpace(v.Title)
		if title == "" {
			continue
		}
		ch := ""
		if v.Channel != "" {
			ch = " — " + v.Channel
		}
		fmt.Fprintf(&sb, "%s%s (%s)\n", title, ch, v.URL)
	}
	return sb.String()
}
