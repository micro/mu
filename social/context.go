package social

import (
	"fmt"
	htmlpkg "html"

	"mu/internal/app"
)

// SocialContext holds social post references found in a news article
type SocialContext struct {
	Posts []ContextPost `json:"posts"`
}

// ContextPost is a social post referenced by a news article
type ContextPost struct {
	Author   string `json:"author"`
	Handle   string `json:"handle"`
	Platform string `json:"platform"`
	Content  string `json:"content"`
	URL      string `json:"url"`
}

// FetchContext detects social media URLs in article content and fetches the posts.
// Returns nil if no social URLs are found.
func FetchContext(articleURL, articleContent string) *SocialContext {
	urls := DetectSocialURLs(articleContent)
	if len(urls) == 0 {
		return nil
	}

	var ctxPosts []ContextPost
	for _, u := range urls {
		if u == articleURL {
			continue
		}

		post, err := FetchPost(u)
		if err != nil {
			app.Log("social", "Failed to fetch context post %s: %v", u, err)
			continue
		}

		// Derive platform from URL
		platform := "X"
		if DetectTruthSocial(u) {
			platform = "Truth Social"
		}

		ctxPosts = append(ctxPosts, ContextPost{
			Author:   post.Author,
			Handle:   post.Author,
			Platform: platform,
			Content:  post.Content,
			URL:      u,
		})
	}

	if len(ctxPosts) == 0 {
		return nil
	}

	return &SocialContext{Posts: ctxPosts}
}

// DetectTruthSocial checks if a URL is from Truth Social
func DetectTruthSocial(u string) bool {
	return len(u) > 0 && (len(u) > 20 && u[:20] == "https://truthsocial." || len(u) > 19 && u[:19] == "http://truthsocial.")
}

// RenderContextHTML renders social context as HTML blockquotes for embedding in news articles
func RenderContextHTML(ctx *SocialContext) string {
	if ctx == nil || len(ctx.Posts) == 0 {
		return ""
	}

	html := `<div class="social-context" style="margin:12px 0;">`
	for _, p := range ctx.Posts {
		content := p.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		html += fmt.Sprintf(`<blockquote style="border-left:3px solid #ccc;margin:8px 0;padding:8px 12px;background:#fafafa;border-radius:0 4px 4px 0;">
  <div style="font-size:12px;color:#666;margin-bottom:4px;"><b>@%s</b> · %s</div>
  <div style="font-size:13px;">%s</div>
  <a href="%s" target="_blank" rel="noopener noreferrer" style="font-size:12px;color:#888;">View original</a>
</blockquote>`, htmlpkg.EscapeString(p.Handle), p.Platform, htmlpkg.EscapeString(content), htmlpkg.EscapeString(p.URL))
	}
	html += `</div>`
	return html
}
