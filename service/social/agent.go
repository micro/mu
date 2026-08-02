package social

import (
	"fmt"
	"strings"
)

// FeedText returns a compact, model-ready snapshot of the latest social posts.
// It is the AI-first accessor behind the social agent tool.
func FeedText(limit int) string {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	threads := GetThreads()
	if len(threads) == 0 {
		return "No social posts available right now."
	}

	var sb strings.Builder
	sb.WriteString("Latest social posts:\n")
	n := 0
	for _, m := range threads {
		if m == nil {
			continue
		}
		content := strings.Join(strings.Fields(m.Content), " ")
		if content == "" {
			continue
		}
		if len(content) > 240 {
			content = content[:240] + "…"
		}
		fmt.Fprintf(&sb, "@%s: %s\n", m.Author, content)
		n++
		if n >= limit {
			break
		}
	}
	if n == 0 {
		return "No social posts available right now."
	}
	return sb.String()
}
