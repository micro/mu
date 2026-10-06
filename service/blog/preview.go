package blog

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
	"mu/internal/app"
)

// previewPost is shared by the public cache and the owner's private feed item.
func previewPost(p *Post) string {
	title := p.Title
	if title == "" {
		title = "Untitled"
	}
	at, label := p.CreatedAt, ""
	if !p.UpdatedAt.IsZero() {
		at = p.UpdatedAt
		label = "Updated "
	}
	metadata := `<span>` + html.EscapeString(label+app.TimeAgo(at)) + `</span>`
	if p.Private {
		metadata += `<span>Private</span>`
	}
	if p.Author != "" {
		metadata += `<span>` + html.EscapeString(p.Author) + `</span>`
	}
	tags := ""
	if p.Tags != "" {
		tags = `<div class="metadata-row">` + formatTags(p.Tags) + `</div>`
	}
	return fmt.Sprintf(`<article class="preview-entry"><h3><a href="/blog/post?id=%s">%s</a></h3><div class="metadata-row">%s</div><p>%s</p>%s</article>`, url.QueryEscape(p.ID), html.EscapeString(title), metadata, previewText(p.Content), tags)
}

// previewText reads the first paragraph, excluding headings, images and embeds.
// The displayed limit includes the ellipsis and counts Unicode characters.
func previewText(markdown string) string {
	if len(markdown) > 4096 {
		end := 4096
		for !utf8.RuneStart(markdown[end]) {
			end--
		}
		markdown = markdown[:end]
	}
	z := xhtml.NewTokenizer(strings.NewReader(string(app.RenderNoImages([]byte(markdown)))))
	var text strings.Builder
	paragraph := false
	for {
		kind := z.Next()
		if kind == xhtml.ErrorToken {
			break
		}
		if kind == xhtml.StartTagToken {
			name, _ := z.TagName()
			if string(name) == "p" {
				paragraph = true
			}
			if paragraph && string(name) == "br" {
				text.WriteByte(' ')
			}
		}
		if kind == xhtml.EndTagToken {
			name, _ := z.TagName()
			if paragraph && string(name) == "p" && strings.TrimSpace(text.String()) != "" {
				break
			}
		}
		if kind == xhtml.TextToken && paragraph {
			text.Write(z.Text())
		}
	}
	runes := []rune(strings.Join(strings.Fields(text.String()), " "))
	if len(runes) > 280 {
		runes = append(runes[:279], '…')
	}
	return html.EscapeString(string(runes))
}
