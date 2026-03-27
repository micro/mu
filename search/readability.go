package search

import (
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// readability extracts the main readable content from HTML using a
// scoring algorithm similar to Mozilla's Readability.js.
// Returns clean HTML suitable for a reader view.
func readability(htmlStr string, pageURL string) (title string, content string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return "", htmlStr
	}

	// Extract title
	title = strings.TrimSpace(doc.Find("title").First().Text())
	if t := doc.Find("h1").First().Text(); t != "" && len(t) < len(title) {
		title = strings.TrimSpace(t)
	}

	// Remove known junk elements
	junkSelectors := []string{
		"script", "style", "noscript", "iframe", "svg",
		"nav", "header", "footer", "aside", "form",
		"button", "input", "select", "textarea",
		"img", "figure", "figcaption", "picture", "video", "audio", "canvas",
		"[role=navigation]", "[role=banner]", "[role=contentinfo]",
		"[role=complementary]", "[role=search]",
		// Common ad/social/cookie classes
		".ad, .ads, .advertisement, .social-share, .share-buttons",
		".cookie-banner, .cookie-notice, .gdpr",
		".sidebar, .widget, .related-posts",
		".comments, .comment-section, #comments",
		".newsletter, .subscribe",
		// Wikipedia specific
		".reflist, .references, .navbox, .infobox, .sidebar",
		".mw-editsection, .toc, .hatnote, .noprint, .mw-jump-link",
		".catlinks, .mw-authority-control, .sistersitebox, .portal",
		".metadata, .ambox, .shortdescription, .mw-indicators, .mbox",
		".reference, .mw-cite-backlink",
		"sup.reference",
	}
	for _, sel := range junkSelectors {
		doc.Find(sel).Remove()
	}

	// Parse base URL for resolving relative links
	base, _ := url.Parse(pageURL)

	// Score each candidate element
	type candidate struct {
		sel   *goquery.Selection
		score float64
	}
	var candidates []candidate

	doc.Find("article, [role=main], main, #content, #main-content, .post-content, .entry-content, .article-body, #mw-content-text, .mw-parser-output, #bodyContent, .story-body, .post-body").Each(func(_ int, s *goquery.Selection) {
		score := scoreElement(s)
		candidates = append(candidates, candidate{sel: s, score: score + 100}) // bonus for semantic tags
	})

	// Also score all divs and sections
	doc.Find("div, section, td").Each(func(_ int, s *goquery.Selection) {
		score := scoreElement(s)
		if score > 20 {
			candidates = append(candidates, candidate{sel: s, score: score})
		}
	})

	if len(candidates) == 0 {
		// Fallback: use body
		candidates = append(candidates, candidate{sel: doc.Find("body"), score: 0})
	}

	// Pick the highest-scoring candidate
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	// Clean the winning element
	el := best.sel

	// Remove low-content children (likely boilerplate)
	el.Find("div, section, aside, table").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		linkText := ""
		s.Find("a").Each(func(_ int, a *goquery.Selection) {
			linkText += a.Text()
		})
		// If more than 70% of text is links, it's probably navigation
		if len(text) > 0 && len(text) < 200 && float64(len(linkText))/float64(len(text)) > 0.7 {
			s.Remove()
		}
	})

	// Resolve relative links
	el.Find("a").Each(func(_ int, a *goquery.Selection) {
		if href, exists := a.Attr("href"); exists {
			resolved := resolveHref(href, base)
			if isProxyableLink(resolved) {
				a.SetAttr("href", "/web/read?url="+url.QueryEscape(resolved))
			} else {
				a.SetAttr("href", resolved)
				a.SetAttr("target", "_blank")
				a.SetAttr("rel", "noopener noreferrer")
			}
		}
	})

	// Strip all attributes except href, target, rel on anchors
	el.Find("*").Each(func(_ int, s *goquery.Selection) {
		tag := goquery.NodeName(s)
		if tag == "a" {
			// Keep href, target, rel only
			for _, attr := range s.Nodes[0].Attr {
				if attr.Key != "href" && attr.Key != "target" && attr.Key != "rel" {
					s.RemoveAttr(attr.Key)
				}
			}
		} else {
			// Remove all attributes
			for _, attr := range s.Nodes[0].Attr {
				s.RemoveAttr(attr.Key)
			}
		}
	})

	// Only keep safe tags
	safeTagSet := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"ul": true, "ol": true, "li": true,
		"blockquote": true, "pre": true, "code": true,
		"br": true, "hr": true,
		"strong": true, "b": true, "em": true, "i": true,
		"a": true, "sub": true, "sup": true,
		"table": true, "thead": true, "tbody": true, "tr": true, "td": true, "th": true,
		"dl": true, "dt": true, "dd": true,
	}

	// Get HTML and strip unsafe tags
	rawHTML, _ := el.Html()

	// Remove any tags not in the safe set (keep their content)
	tagRe := regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)[^>]*>`)
	content = tagRe.ReplaceAllStringFunc(rawHTML, func(tag string) string {
		m := tagRe.FindStringSubmatch(tag)
		if len(m) < 2 {
			return ""
		}
		tagName := strings.ToLower(m[1])
		if safeTagSet[tagName] {
			return tag
		}
		// Replace block-level unknowns with line break, inline with space
		if tagName == "div" || tagName == "section" || tagName == "article" || tagName == "main" {
			return "\n"
		}
		return " "
	})

	// Clean up whitespace
	multiSpace := regexp.MustCompile(`[ \t]+`)
	content = multiSpace.ReplaceAllString(content, " ")
	excessiveNewlines := regexp.MustCompile(`(\s*\n\s*){3,}`)
	content = excessiveNewlines.ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	// Truncate if too long
	if len(content) > 50000 {
		content = content[:50000] + "\n\n<p><em>[Content truncated]</em></p>"
	}

	return title, content
}

// scoreElement scores an element based on content density.
func scoreElement(s *goquery.Selection) float64 {
	text := strings.TrimSpace(s.Text())
	if len(text) == 0 {
		return 0
	}

	// Count paragraphs (good signal for article content)
	paragraphs := s.Find("p").Length()
	score := float64(paragraphs) * 5

	// Text length bonus (log scale to avoid huge pages dominating)
	score += math.Log(float64(len(text)+1)) * 3

	// Penalize link-heavy content
	linkText := ""
	s.Find("a").Each(func(_ int, a *goquery.Selection) {
		linkText += a.Text()
	})
	linkRatio := float64(len(linkText)) / float64(len(text))
	if linkRatio > 0.5 {
		score *= 0.3
	}

	// Bonus for certain class/id names
	classID := strings.ToLower(s.AttrOr("class", "") + " " + s.AttrOr("id", ""))
	for _, good := range []string{"article", "content", "post", "entry", "story", "text", "body", "parser-output"} {
		if strings.Contains(classID, good) {
			score += 25
		}
	}
	for _, bad := range []string{"comment", "sidebar", "footer", "header", "nav", "menu", "social", "share", "ad", "widget"} {
		if strings.Contains(classID, bad) {
			score -= 50
		}
	}

	return score
}

// resolveHref resolves a potentially relative URL against a base URL.
func resolveHref(href string, base *url.URL) string {
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
		return href
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if base != nil {
		return base.ResolveReference(u).String()
	}
	return href
}
