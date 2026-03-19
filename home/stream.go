package home

import (
	"fmt"
	htmlpkg "html"
	"sort"
	"strings"
	"time"

	"mu/apps"
	"mu/blog"
	"mu/markets"
	"mu/news"
	"mu/social"
	"mu/video"
)

// StreamItem represents a single message in the unified stream.
type StreamItem struct {
	Type      string    // "news", "blog", "social", "video", "market", "app"
	Title     string    // headline / post title
	Snippet   string    // short description or content preview
	Link      string    // URL to the full item
	Source    string    // author / channel / category
	Icon      string    // emoji icon for the type
	Timestamp time.Time // when it happened
}

// BuildStream collects recent items from all sources and returns them
// interleaved chronologically (newest first), limited to n items.
func BuildStream(n int) []StreamItem {
	var items []StreamItem

	// News — latest 8 headlines
	for _, p := range news.GetFeed() {
		if len(items) >= 8 {
			break
		}
		items = append(items, StreamItem{
			Type:      "news",
			Title:     p.Title,
			Snippet:   truncateStr(p.Description, 120),
			Link:      "/news?id=" + p.ID,
			Source:    p.Category,
			Icon:      "📰",
			Timestamp: p.PostedAt,
		})
	}

	// Blog — latest 4 public posts
	for _, p := range blog.GetRecentPosts(4) {
		snippet := p.Content
		// Strip markdown
		snippet = strings.ReplaceAll(snippet, "**", "")
		snippet = strings.ReplaceAll(snippet, "\n", " ")
		items = append(items, StreamItem{
			Type:      "blog",
			Title:     p.Title,
			Snippet:   truncateStr(snippet, 120),
			Link:      "/post?id=" + p.ID,
			Source:    p.Author,
			Icon:      "📝",
			Timestamp: p.CreatedAt,
		})
	}

	// Social — latest 4 threads
	for _, t := range social.GetRecentThreads(4) {
		items = append(items, StreamItem{
			Type:      "social",
			Title:     t.Title,
			Snippet:   truncateStr(t.Content, 120),
			Link:      "/social?id=" + t.ID,
			Source:    t.Author,
			Icon:      "💬",
			Timestamp: t.CreatedAt,
		})
	}

	// Video — latest 4 videos
	for _, v := range video.GetLatestVideos(4) {
		items = append(items, StreamItem{
			Type:      "video",
			Title:     v.Title,
			Link:      v.URL,
			Source:    v.Channel,
			Icon:      "🎬",
			Timestamp: v.Published,
		})
	}

	// Markets — snapshot (single item, always "now")
	prices := markets.GetAllPriceData()
	if len(prices) > 0 {
		var parts []string
		// Pick top symbols
		for _, sym := range []string{"BTC", "ETH", "SOL", "GOLD", "SP500"} {
			if pd, ok := prices[sym]; ok {
				sign := "+"
				if pd.Change24h < 0 {
					sign = ""
				}
				parts = append(parts, fmt.Sprintf("%s $%s (%s%.1f%%)",
					sym, formatCompactPrice(pd.Price), sign, pd.Change24h))
			}
		}
		if len(parts) > 0 {
			items = append(items, StreamItem{
				Type:      "market",
				Title:     "Market Update",
				Snippet:   strings.Join(parts, " · "),
				Link:      "/markets",
				Icon:      "📈",
				Timestamp: time.Now(),
			})
		}
	}

	// Apps — latest 3 public apps
	appCount := 0
	for _, a := range apps.GetPublicApps() {
		if appCount >= 3 {
			break
		}
		appCount++
		items = append(items, StreamItem{
			Type:      "app",
			Title:     a.Name,
			Snippet:   a.Description,
			Link:      "/apps/" + a.Slug,
			Source:    a.Author,
			Icon:      "📱",
			Timestamp: a.CreatedAt,
		})
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})

	if len(items) > n {
		items = items[:n]
	}
	return items
}

// RenderStream returns an HTML string for the message stream.
func RenderStream(items []StreamItem) string {
	if len(items) == 0 {
		return `<div class="card"><p style="color:#888;">No updates yet.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div id="stream">`)

	var lastDate string
	for _, item := range items {
		// Date separator
		date := item.Timestamp.Format("Monday, 2 January")
		if date != lastDate {
			b.WriteString(`<div style="font-size:12px;color:#999;font-weight:700;padding:12px 0 4px;border-bottom:1px solid #f0f0f0;margin-bottom:4px;">` + htmlpkg.EscapeString(date) + `</div>`)
			lastDate = date
		}

		b.WriteString(`<div class="stream-item" style="display:flex;gap:10px;padding:10px 0;border-bottom:1px solid #f5f5f5;align-items:flex-start;">`)

		// Icon
		b.WriteString(`<div style="font-size:18px;flex-shrink:0;width:24px;text-align:center;">` + item.Icon + `</div>`)

		// Content
		b.WriteString(`<div style="min-width:0;flex:1;">`)

		// Title row with time
		b.WriteString(`<div style="display:flex;justify-content:space-between;align-items:baseline;gap:8px;">`)
		b.WriteString(`<a href="` + htmlpkg.EscapeString(item.Link) + `" style="font-size:14px;font-weight:600;color:#111;display:block;line-height:1.3;">` + htmlpkg.EscapeString(item.Title) + `</a>`)
		b.WriteString(`<span style="font-size:11px;color:#aaa;white-space:nowrap;flex-shrink:0;">` + formatStreamTime(item.Timestamp) + `</span>`)
		b.WriteString(`</div>`)

		// Snippet
		if item.Snippet != "" {
			b.WriteString(`<div style="font-size:13px;color:#555;line-height:1.4;margin-top:2px;">` + htmlpkg.EscapeString(item.Snippet) + `</div>`)
		}

		// Source badge
		if item.Source != "" {
			b.WriteString(`<div style="font-size:11px;color:#888;margin-top:3px;">` + htmlpkg.EscapeString(item.Source) + `</div>`)
		}

		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}

func formatStreamTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return t.Format("2 Jan")
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func formatCompactPrice(price float64) string {
	if price >= 1000 {
		return formatLargeNum(price)
	}
	if price >= 1 {
		return fmt.Sprintf("%.2f", price)
	}
	return fmt.Sprintf("%.4f", price)
}

func formatLargeNum(n float64) string {
	i := int64(n)
	s := fmt.Sprintf("%d", i)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if s != "" {
		parts = append([]string{s}, parts...)
	}
	return strings.Join(parts, ",")
}
