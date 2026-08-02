package blog

import (
	"fmt"
	"strings"
)

// RecentText returns a compact, model-ready list of recent public blog posts.
// It is the AI-first accessor behind the blog_list agent tool.
func RecentText(limit int) string {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	mutex.RLock()
	defer mutex.RUnlock()

	var sb strings.Builder
	sb.WriteString("Recent blog posts:\n")
	n := 0
	for _, p := range posts {
		if p == nil || p.Private {
			continue
		}
		title := strings.TrimSpace(p.Title)
		if title == "" {
			continue
		}
		fmt.Fprintf(&sb, "%s — %s (id: %s)\n", title, blogSnippet(p.Content, 160), p.ID)
		n++
		if n >= limit {
			break
		}
	}
	if n == 0 {
		return "No blog posts available right now."
	}
	return sb.String()
}

func blogSnippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
