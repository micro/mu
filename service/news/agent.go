package news

// Agent-facing helpers for the news_headlines and news_read tools.
//
// The default feed is grouped alphabetically by category, so the first slice
// of it (all the agent ever sees once truncated) is dominated by whichever
// topic sorts first — in practice crypto. These helpers give the agent a
// topic-balanced list of headlines to scan, plus a way to pull one article in
// full, so briefings summarise across topics rather than fixating on crypto.

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/data"
)

// HeadlinesText returns a compact, topic-balanced list of recent headlines
// (title + short description + id) suitable for an LLM to scan before deciding
// what to read in full via news_read. Categories are interleaved round-robin
// and ordered by recency, so every topic is represented near the top
// regardless of its name. An optional topic filters to matching categories.
func HeadlinesText(topic string, limit int) string {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	topic = strings.TrimSpace(strings.ToLower(topic))

	byCat := map[string][]*Post{}
	var catOrder []string
	for _, p := range GetFeed() {
		if p == nil || strings.TrimSpace(p.Title) == "" {
			continue
		}
		if topic != "" && !postMatchesNewsTopic(p, topic) {
			continue
		}
		if _, ok := byCat[p.Category]; !ok {
			catOrder = append(catOrder, p.Category)
		}
		byCat[p.Category] = append(byCat[p.Category], p)
	}

	if len(catOrder) == 0 {
		if topic != "" {
			return fmt.Sprintf("No news headlines found for topic %q.", topic)
		}
		return "No news headlines available right now."
	}

	// Newest-first within each category.
	for cat := range byCat {
		ps := byCat[cat]
		sort.Slice(ps, func(i, j int) bool { return ps[i].PostedAt.After(ps[j].PostedAt) })
		byCat[cat] = ps
	}
	// Lead with the categories that have the freshest news, not the ones that
	// happen to sort first alphabetically.
	sort.Slice(catOrder, func(i, j int) bool {
		return byCat[catOrder[i]][0].PostedAt.After(byCat[catOrder[j]][0].PostedAt)
	})

	// Round-robin: take one headline from each category per pass.
	var picked []*Post
	for round := 0; len(picked) < limit; round++ {
		progressed := false
		for _, cat := range catOrder {
			if ps := byCat[cat]; round < len(ps) {
				picked = append(picked, ps[round])
				progressed = true
				if len(picked) >= limit {
					break
				}
			}
		}
		if !progressed {
			break
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Current request date: %s.\n", time.Now().UTC().Format("Monday, 2 January 2006 (2006-01-02, UTC)"))
	if topic != "" {
		fmt.Fprintf(&sb, "Latest %q headlines (%d). Use news_read with an id to read one in full.\n\n", topic, len(picked))
	} else {
		fmt.Fprintf(&sb, "Latest headlines — %d across %d topics. Summarise a spread of topics, not just the first few. Use news_read with an id to read any article in full.\n\n", len(picked), len(catOrder))
	}
	for _, p := range picked {
		cat := p.Category
		if cat == "" {
			cat = "General"
		}
		line := fmt.Sprintf("[%s] %s", cat, strings.TrimSpace(p.Title))
		if desc := strings.TrimSpace(p.Description); desc != "" {
			line += " — " + desc
		}
		if p.URL != "" {
			line += fmt.Sprintf(" (source: %s, url: %s", getDomain(p.URL), p.URL)
			if p.ID != "" {
				line += fmt.Sprintf(", id: %s", p.ID)
			}
			line += ")"
		} else if p.ID != "" {
			line += fmt.Sprintf(" (id: %s)", p.ID)
		}
		fmt.Fprintln(&sb, line)
	}
	return sb.String()
}

func postMatchesNewsTopic(p *Post, topic string) bool {
	if p == nil {
		return false
	}
	haystack := strings.ToLower(strings.Join([]string{
		p.Category,
		p.Title,
		p.Description,
		p.Content,
	}, " "))
	for _, term := range newsTopicTerms(topic) {
		if strings.Contains(haystack, term) {
			return true
		}
	}
	return false
}

func newsTopicTerms(topic string) []string {
	topic = strings.TrimSpace(strings.ToLower(topic))
	if topic == "" {
		return nil
	}
	terms := []string{topic}
	for _, field := range strings.Fields(topic) {
		if field != topic {
			terms = append(terms, field)
		}
	}
	if strings.Contains(topic, "technology") {
		terms = append(terms, "tech", "ai", "artificial intelligence", "machine learning", "model", "software")
	}
	if topic == "ai" || strings.Contains(topic, "artificial intelligence") {
		terms = append(terms, "ai", "artificial intelligence", "machine learning", "model", "llm")
	}
	return terms
}

// ArticleText returns a single news article in full — title, source, summary
// and extracted body — for the given article id (as shown by news_headlines)
// or a full article URL. Used by the news_read tool.
func ArticleText(idOrURL string) (string, error) {
	idOrURL = strings.TrimSpace(idOrURL)
	if idOrURL == "" {
		return "", fmt.Errorf("article id is required")
	}

	// Accept either an article id or a full URL.
	url := ""
	entry := data.GetByID(idOrURL)
	if entry == nil && strings.HasPrefix(idOrURL, "http") {
		url = idOrURL
		entry = data.GetByID(articleID(idOrURL))
	}
	// Only ever expose news entries through this tool — never another type
	// (e.g. a private mail entry) that happens to share an id space.
	if entry != nil && entry.Type != "news" {
		entry = nil
	}

	title, category, description, summary := "", "", "", ""
	var postedAt time.Time
	if entry != nil {
		title = entry.Title
		if v, ok := entry.Metadata["url"].(string); ok && url == "" {
			url = v
		}
		category, _ = entry.Metadata["category"].(string)
		description, _ = entry.Metadata["description"].(string)
		summary, _ = entry.Metadata["summary"].(string)
		postedAt = metaTime(entry.Metadata["posted_at"])
	}

	// Pull the richest body/summary we have cached for this URL.
	body := ""
	if url != "" {
		if md, ok := LookupMetadata(url); ok {
			if md.Content != "" {
				body = htmlToText(md.Content)
			}
			if summary == "" {
				summary = md.Summary
			}
			if description == "" {
				description = md.Description
			}
			if title == "" {
				title = md.Title
			}
		}
	}
	if body == "" && entry != nil {
		body = htmlToText(entry.Content)
	}
	if body == "" {
		body = description
	}

	summary = strings.TrimSpace(summary)
	body = strings.TrimSpace(body)
	if summary == "" && body == "" {
		return "", fmt.Errorf("no readable content for article: %s", idOrURL)
	}

	var sb strings.Builder
	if t := strings.TrimSpace(title); t != "" {
		fmt.Fprintf(&sb, "Title: %s\n", t)
	}
	if url != "" {
		fmt.Fprintf(&sb, "Source: %s\n", getDomain(url))
	}
	if category != "" {
		fmt.Fprintf(&sb, "Category: %s\n", category)
	}
	if !postedAt.IsZero() {
		fmt.Fprintf(&sb, "Published: %s\n", postedAt.UTC().Format("Mon, 2 Jan 2006 15:04 MST"))
	}
	if url != "" {
		fmt.Fprintf(&sb, "URL: %s\n", url)
	}
	sb.WriteString("\n")

	if summary != "" {
		fmt.Fprintf(&sb, "Summary:\n%s\n\n", summary)
	}
	if body != "" {
		const maxBody = 6000
		if len(body) > maxBody {
			body = body[:maxBody] + "…"
		}
		fmt.Fprintf(&sb, "Article:\n%s\n", body)
	}
	return sb.String(), nil
}

// articleID derives the stable 16-char id used when indexing a post, matching
// getMetadataPath / parseFeedItem so a URL can be resolved to its index entry.
func articleID(uri string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(uri)))[:16]
}

// metaTime coerces an index metadata timestamp (stored as time.Time or an
// RFC3339 string) into a time.Time, returning the zero value on failure.
func metaTime(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
