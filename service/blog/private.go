package blog

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
)

func (p *Post) isDraft() bool {
	// Older scheduled readings were saved before publication and privacy were
	// separate. Recognise their stable identity without changing user drafts.
	reading := strings.HasPrefix(p.ID, "reading-") && (p.Tags == "evening-reading" || hasTag(p.Tags, "Research"))
	return p.Private && !p.Published && !reading
}

func postView(p *Post) string {
	if p.Private && !p.isDraft() {
		return "editorial"
	}
	return publication(p.Editorial, p.Community)
}

// privatePostItems is called with mutex held. Its output belongs only to the
// signed-in owner; it must never enter the public render or snapshot caches.
func privatePostItems(owner string) []listItem {
	var items []listItem
	if owner == "" {
		return items
	}
	for _, p := range posts {
		if !p.Private || p.AuthorID != owner || p.isDraft() {
			continue
		}
		body := fmt.Sprintf(`<article class="editorial-entry"><div class="metadata-row"><span>Private</span><time datetime="%s">%s</time></div><h2><a href="/blog/post?id=%s">%s</a></h2><p>%s</p><div class="metadata-row">%s</div></article>`, p.CreatedAt.Format(time.RFC3339), p.CreatedAt.Format("2 January 2006"), url.QueryEscape(p.ID), html.EscapeString(p.Title), previewText(p.Content), formatTags(p.Tags))
		at := p.UpdatedAt
		if at.IsZero() {
			at = p.CreatedAt
		}
		items = append(items, listItem{At: at, Editorial: true, ID: p.ID, AuthorID: p.AuthorID, HTML: body, Tags: p.Tags, Search: strings.ToLower(p.Title + " " + p.Content + " " + p.Tags + " " + p.Author)})
	}
	return items
}

// PrivatePreview adds the owner's latest completed reading to their Home feed.
func PrivatePreview(owner string) string {
	mutex.RLock()
	defer mutex.RUnlock()
	if owner == "" {
		return ""
	}
	for _, p := range posts {
		if p.Private && p.AuthorID == owner && !p.isDraft() {
			return previewPost(p)
		}
	}
	return ""
}

// Older generated readings stored the schedule topic instead of the essay title.
// Upgrade only legacy readings and preserve explicit edits and publication state.
func repairReading(p *Post) {
	if !strings.HasPrefix(p.ID, "reading-") || p.Tags != "evening-reading" {
		return
	}
	p.Tags = "Research"
	if !p.UpdatedAt.IsZero() {
		return
	}
	for _, line := range strings.Split(p.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			if title := strings.TrimSpace(strings.TrimPrefix(line, "# ")); title != "" {
				p.Title = title
			}
		}
		break
	}
}

func hasTag(tags, selected string) bool {
	if selected == "" {
		return true
	}
	for _, tag := range strings.Split(tags, ",") {
		if strings.EqualFold(strings.TrimSpace(tag), selected) {
			return true
		}
	}
	return false
}
