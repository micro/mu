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
	vids := GetLatestVideos(limit)
	if len(vids) == 0 {
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
