package agent

import (
	"golang.org/x/net/html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/service/blog"
	"mu/service/events"
	"net/url"
	"sort"
	"strings"
	"time"
)

func eveningDigest(e *events.Event, at time.Time) (string, error) {
	if zone, err := time.LoadLocation(e.Zone); err == nil {
		at = at.In(zone)
	}
	return renderEveningDigest(blog.PostsByAuthorID(auth.MicroID, "Micro"), e.Prompt, at)
}

func digestTag(tags, want string) bool {
	for _, tag := range strings.Split(tags, ",") {
		if strings.EqualFold(strings.TrimSpace(tag), want) {
			return true
		}
	}
	return false
}

// Delivery reads shared public posts only; it never launches per-reader research.
func renderEveningDigest(posts []*blog.Post, topic string, at time.Time) (string, error) {
	posts = append([]*blog.Post(nil), posts...)
	sort.SliceStable(posts, func(i, j int) bool { return posts[i].CreatedAt.After(posts[j].CreatedAt) })
	var roundup *blog.Post
	var reading []*blog.Post
	seen := map[string]bool{}
	for _, p := range posts {
		if p.Private || p.AuthorID != auth.MicroID || p.CreatedAt.After(at) || p.CreatedAt.In(at.Location()).Format("2006-01-02") != at.Format("2006-01-02") || strings.TrimSpace(p.Content) == "" {
			continue
		}
		if topic == "" && roundup == nil && digestTag(p.Tags, "digest") {
			roundup = p
			continue
		}
		if !digestTag(p.Tags, "opinion") || (topic != "" && !digestTag(p.Tags, topic)) {
			continue
		}
		// Keep one current piece per topic rather than repeating the same subject.
		key := strings.ToLower(p.Tags)
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(reading) < 8 {
			reading = append(reading, p)
		}
	}
	var b strings.Builder
	b.WriteString("# Evening Digest\n\n")
	if roundup != nil {
		b.WriteString("## World roundup\n\nPublished " + roundup.CreatedAt.In(at.Location()).Format("Monday 2 January, 15:04 MST") + ".\n\n")
		b.WriteString(digestParagraph(strings.SplitN(roundup.Content, "<details", 2)[0]))
		b.WriteString("\n\n[World roundup](" + origin.Self() + "/blog/post?id=" + url.QueryEscape(roundup.ID) + ")\n\n")
	}
	for _, p := range reading {
		b.WriteString("## " + p.Title + "\n\n" + digestParagraph(p.Content) + "\n\n[Read more](" + origin.Self() + "/blog/post?id=" + url.QueryEscape(p.ID) + ")\n\n")
	}
	if roundup == nil && len(reading) == 0 {
		subject := "blog digests"
		if topic != "" {
			subject = topic + " digests"
		}
		b.WriteString("No new " + subject + " were published today.\n\n[Browse the blog](" + origin.Self() + "/blog)\n\n")
	}
	b.WriteString("[Manage Evening Digest](" + origin.Self() + "/agents?view=scheduled&event=research)")
	return b.String(), nil
}

func digestParagraph(content string) string {
	for _, paragraph := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" || strings.HasPrefix(paragraph, "#") || strings.HasPrefix(paragraph, "<") {
			continue
		}
		// Render first, then shorten visible text: never slice through link syntax.
		tokenizer := html.NewTokenizer(strings.NewReader(app.RenderString(paragraph)))
		var plain strings.Builder
		for {
			kind := tokenizer.Next()
			if kind == html.ErrorToken {
				break
			}
			if kind == html.TextToken {
				plain.Write(tokenizer.Text())
			}
		}
		paragraph = strings.TrimSpace(plain.String())
		words := strings.Fields(paragraph)
		if len(words) > 90 {
			paragraph = strings.Join(words[:90], " ") + "…"
		}
		// This excerpt is embedded in Markdown; keep visible punctuation literal.
		return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "<", "&lt;", ">", "&gt;").Replace(paragraph)
	}
	return "Read the full digest on the blog."
}
