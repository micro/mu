package search

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/wallet"
)

// nitterInstance is the Nitter instance used to fetch Twitter/X content.
// Nitter renders tweets as static HTML, which our extractor can parse.
var nitterInstance = "nitter.poast.org"

var fetchClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// FetchHandler serves the /fetch page — fetches a URL and returns cleaned readable content.
func FetchHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))

	// Render input form
	inputForm := `<form class="search-bar" action="/web/fetch" method="GET">` +
		`<input type="text" name="url" placeholder="Paste a URL to fetch..." value="` +
		html.EscapeString(rawURL) + `" autofocus>` +
		`<button type="submit">Fetch</button>` +
		`</form>`

	if rawURL == "" {
		content := inputForm + `<p class="empty">Paste a URL above to fetch and read a clean version of any web page.</p>`
		w.Write([]byte(app.RenderHTMLForRequest("Fetch", "Web Fetch", content, r)))
		return
	}

	// Validate URL
	if len(rawURL) > 2048 {
		app.BadRequest(w, r, "URL must not exceed 2048 characters")
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		app.BadRequest(w, r, "Invalid URL — must start with http:// or https://")
		return
	}

	// Block private/internal IPs
	host := strings.ToLower(parsed.Hostname())
	if isPrivateHost(host) {
		app.BadRequest(w, r, "Cannot fetch private or internal URLs")
		return
	}

	// Require authentication
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		app.Unauthorized(w, r)
		return
	}

	// Check quota
	canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpWebFetch)
	if !canProceed {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusPaymentRequired, fmt.Sprintf("web fetch requires %d credits", cost))
			return
		}
		content := inputForm + wallet.QuotaExceededPage(wallet.OpWebFetch, cost)
		w.Write([]byte(app.RenderHTMLForRequest("Fetch", "Web Fetch", content, r)))
		return
	}

	// Fetch the page
	title, body, fetchErr := FetchAndExtract(rawURL)

	// Only charge on success
	if fetchErr == nil {
		wallet.ConsumeQuota(sess.Account, wallet.OpWebFetch)
	}

	// JSON response for API/MCP callers
	if app.WantsJSON(r) {
		if fetchErr != nil {
			app.RespondError(w, http.StatusBadGateway, "failed to fetch page: "+fetchErr.Error())
			return
		}
		app.RespondJSON(w, map[string]interface{}{
			"url":     rawURL,
			"title":   title,
			"content": body,
		})
		return
	}

	// HTML response
	var b strings.Builder
	b.WriteString(inputForm)

	if fetchErr != nil {
		app.Log("fetch", "Fetch error for %s: %v", rawURL, fetchErr)
		b.WriteString(`<p class="empty">Failed to fetch page: ` + html.EscapeString(fetchErr.Error()) + `</p>`)
	} else {
		b.WriteString(`<div class="card">`)
		if title != "" {
			b.WriteString(`<h2>` + html.EscapeString(title) + `</h2>`)
		}
		b.WriteString(`<p style="font-size:12px;color:#888;margin-bottom:12px;">`)
		b.WriteString(`<a href="` + html.EscapeString(rawURL) + `" target="_blank" rel="noopener noreferrer">` + html.EscapeString(rawURL) + `</a>`)
		b.WriteString(`</p>`)
		// Render cleaned content as paragraphs
		paragraphs := strings.Split(body, "\n\n")
		for _, p := range paragraphs {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			b.WriteString(`<p>` + html.EscapeString(p) + `</p>`)
		}
		b.WriteString(`</div>`)
	}

	pageHTML := app.RenderHTMLForRequest("Fetch", "Fetched page", b.String(), r)
	w.Write([]byte(pageHTML))
}

// FetchAndExtract fetches a URL and returns the page title and cleaned readable text.
func FetchAndExtract(rawURL string) (string, string, error) {
	// Rewrite Twitter/X URLs to Nitter for static HTML
	fetchURL, _ := rewriteTwitterURL(rawURL)

	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Mu/1.0; +https://mu.al)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	start := time.Now()
	resp, err := fetchClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		app.RecordAPICall("fetch", "GET", rawURL, 0, duration, err, "", "")
		return "", "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		app.RecordAPICall("fetch", "GET", rawURL, resp.StatusCode, duration, fmt.Errorf("HTTP %d", resp.StatusCode), "", "")
		return "", "", fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	// Limit read to 2MB to prevent abuse
	limited := io.LimitReader(resp.Body, 2*1024*1024)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		app.RecordAPICall("fetch", "GET", rawURL, resp.StatusCode, duration, err, "", "")
		return "", "", fmt.Errorf("failed to read response: %v", err)
	}

	app.RecordAPICall("fetch", "GET", rawURL, resp.StatusCode, duration, nil, "", "")

	content := string(bodyBytes)
	if !utf8.ValidString(content) {
		// Try to handle as latin-1
		runes := make([]rune, len(bodyBytes))
		for i, b := range bodyBytes {
			runes[i] = rune(b)
		}
		content = string(runes)
	}

	title := extractTitle(content)
	readable := extractReadableContent(content)

	return title, readable, nil
}

// FetchAndExtractHTMLProxied fetches a URL and returns the page title and sanitized
// readable HTML. All links are routed through /read so the user stays in the clean
// reading experience. Unlike FetchAndExtract which returns plain text, this preserves
// structural HTML (headings, paragraphs, lists, emphasis, links).
func FetchAndExtractHTMLProxied(rawURL string) (string, string, error) {
	return fetchAndSanitize(rawURL, true)
}

// fetchAndSanitize is the shared implementation for HTML extraction.
// If proxy is true, links are routed through /read.
func fetchAndSanitize(rawURL string, proxy bool) (string, string, error) {
	// Rewrite Twitter/X URLs to Nitter for static HTML
	fetchURL, _ := rewriteTwitterURL(rawURL)

	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Mu/1.0; +https://mu.al)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	start := time.Now()
	resp, err := fetchClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		app.RecordAPICall("fetch", "GET", rawURL, 0, duration, err, "", "")
		return "", "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		app.RecordAPICall("fetch", "GET", rawURL, resp.StatusCode, duration, fmt.Errorf("HTTP %d", resp.StatusCode), "", "")
		return "", "", fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	limited := io.LimitReader(resp.Body, 2*1024*1024)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		app.RecordAPICall("fetch", "GET", rawURL, resp.StatusCode, duration, err, "", "")
		return "", "", fmt.Errorf("failed to read response: %v", err)
	}

	app.RecordAPICall("fetch", "GET", rawURL, resp.StatusCode, duration, nil, "", "")

	content := string(bodyBytes)
	if !utf8.ValidString(content) {
		runes := make([]rune, len(bodyBytes))
		for i, b := range bodyBytes {
			runes[i] = rune(b)
		}
		content = string(runes)
	}

	title := extractTitle(content)
	readable := sanitizeHTML(content, rawURL, proxy)

	return title, readable, nil
}

// sanitizeHTML extracts the main content and preserves safe structural HTML.
// It keeps headings, paragraphs, lists, emphasis, links, and blockquotes
// while removing scripts, ads, nav, and all other dangerous or noisy elements.
// baseURL is the original page URL, used to resolve relative links.
// If proxy is true, all links are routed through /read for continued clean reading.
func sanitizeHTML(htmlStr string, baseURL string, proxy bool) string {
	// Determine the base URL for resolving relative links
	// Check for <base href="..."> tag first
	baseResolved := resolveBaseURL(htmlStr, baseURL)

	content := extractMainContent(htmlStr)

	// Remove dangerous/noisy blocks entirely
	content = commentRe.ReplaceAllString(content, "")
	for _, re := range []*regexp.Regexp{
		removeScriptRe, removeStyleRe, removeNoscriptRe, removeIframeRe,
		removeSvgRe, removeNavRe, removeHeaderRe, removeFooterRe,
		removeAsideRe, removeFormRe,
	} {
		content = re.ReplaceAllString(content, "")
	}

	// Remove noisy elements by class/id (Wikipedia infoboxes, references, TOC, etc.)
	for _, pattern := range []string{
		`(?is)<[^>]+class="[^"]*(?:reflist|reference|navbox|sidebar|infobox|mw-editsection|toc|hatnote|noprint|mw-jump-link|catlinks|mw-authority-control|sistersitebox|portal|metadata|ambox|shortdescription|mw-indicators|mbox)[^"]*"[^>]*>.*?</(?:div|table|span|ul|ol|nav|section)>`,
		`(?is)<table[^>]+class="[^"]*(?:infobox|sidebar|navbox|wikitable\.sortable)[^"]*"[^>]*>.*?</table>`,
		`(?is)<sup[^>]*class="[^"]*reference[^"]*"[^>]*>.*?</sup>`,
	} {
		re := regexp.MustCompile(pattern)
		content = re.ReplaceAllString(content, "")
	}

	// Remove image tags (they usually break without their CSS context)
	imgRe := regexp.MustCompile(`(?i)<img[^>]*>`)
	content = imgRe.ReplaceAllString(content, "")

	// Remove button elements
	buttonRe := regexp.MustCompile(`(?is)<button[^>]*>.*?</button>`)
	content = buttonRe.ReplaceAllString(content, "")

	// Remove input elements
	inputRe := regexp.MustCompile(`(?i)<input[^>]*>`)
	content = inputRe.ReplaceAllString(content, "")

	// Preserve <a> tags — sanitize attributes and resolve relative URLs
	aTagRe := regexp.MustCompile(`(?i)<a\s[^>]*href=["']([^"']*)["'][^>]*>`)
	content = aTagRe.ReplaceAllStringFunc(content, func(match string) string {
		m := aTagRe.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		href := resolveLink(m[1], baseResolved)
		// Route through reader for continued clean browsing
		if proxy && isProxyableLink(href) {
			return `<a href="/web/read?url=` + url.QueryEscape(href) + `">`
		}
		return `<a href="` + href + `" target="_blank" rel="noopener noreferrer">`
	})

	// Strip all attributes from safe block/inline tags (keep the tags themselves)
	safeBlockTags := regexp.MustCompile(`(?i)<(/?(?:p|h[1-6]|ul|ol|li|blockquote|pre|code|br|hr|strong|b|em|i|sub|sup|table|thead|tbody|tr|td|th|figcaption|figure|dl|dt|dd))\b[^>]*>`)
	content = safeBlockTags.ReplaceAllString(content, `<$1>`)

	// Remove <div> and <section> tags but keep content (replace with paragraph breaks)
	divOpenRe := regexp.MustCompile(`(?i)<(?:div|section|article|main)[^>]*>`)
	content = divOpenRe.ReplaceAllString(content, "\n")
	divCloseRe := regexp.MustCompile(`(?i)</(?:div|section|article|main)>`)
	content = divCloseRe.ReplaceAllString(content, "\n")

	// Remove <span> tags but keep content (add space to prevent words merging)
	spanOpenRe := regexp.MustCompile(`(?i)<span[^>]*>`)
	content = spanOpenRe.ReplaceAllString(content, "")
	spanCloseRe := regexp.MustCompile(`(?i)</span>`)
	content = spanCloseRe.ReplaceAllString(content, " ")

	// Remove any remaining unsafe tags (but keep their text content with spacing)
	unsafeTagRe := regexp.MustCompile(`(?i)</?(?:label|abbr|time|mark|small|cite|dfn|var|samp|kbd|data|ruby|rt|rp|bdi|bdo|wbr|details|summary|dialog|slot|template|canvas|video|audio|source|track|map|area|object|embed|param|picture)\b[^>]*>`)
	content = unsafeTagRe.ReplaceAllString(content, " ")

	// Remove any remaining unknown/unsafe tags but preserve content
	remainingTagRe := regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*[^>]*>`)
	// Check if it's a safe tag before removing
	content = remainingTagRe.ReplaceAllStringFunc(content, func(tag string) string {
		lower := strings.ToLower(tag)
		// Keep safe tags
		for _, safe := range []string{"<p", "</p", "<h1", "<h2", "<h3", "<h4", "<h5", "<h6",
			"</h1", "</h2", "</h3", "</h4", "</h5", "</h6",
			"<ul", "</ul", "<ol", "</ol", "<li", "</li",
			"<blockquote", "</blockquote", "<pre", "</pre", "<code", "</code",
			"<br", "<hr",
			"<strong", "</strong", "<b>", "</b", "<em", "</em", "<i>", "</i",
			"<a ", "</a", "<table", "</table", "<thead", "</thead", "<tbody", "</tbody",
			"<tr", "</tr", "<td", "</td", "<th", "</th",
			"<sub", "</sub", "<sup", "</sup",
			"<figcaption", "</figcaption", "<figure", "</figure",
			"<dl", "</dl", "<dt", "</dt", "<dd", "</dd"} {
			if strings.HasPrefix(lower, safe) {
				return tag
			}
		}
		return " "
	})

	// Collapse excessive whitespace but preserve structure
	content = multiSpaceRe.ReplaceAllString(content, " ")
	// Clean up excessive newlines
	excessiveNewlines := regexp.MustCompile(`(\s*\n\s*){3,}`)
	content = excessiveNewlines.ReplaceAllString(content, "\n\n")

	// Filter out junk lines (cookie banners etc)
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			cleaned = append(cleaned, "")
			continue
		}
		textOnly := stripTags(trimmed)
		lower := strings.ToLower(textOnly)
		skip := false
		for _, junk := range junkPatterns {
			if strings.Contains(lower, junk) && len(textOnly) < 200 {
				skip = true
				break
			}
		}
		if !skip {
			cleaned = append(cleaned, line)
		}
	}

	content = strings.Join(cleaned, "\n")
	content = strings.TrimSpace(content)

	if len(content) > 50000 {
		content = content[:50000] + "\n\n<p><em>[Content truncated]</em></p>"
	}

	return content
}

// resolveBaseURL determines the base URL for resolving relative links.
// It checks for a <base href="..."> tag in the HTML, falling back to the page URL.
func resolveBaseURL(htmlStr string, pageURL string) *url.URL {
	baseRe := regexp.MustCompile(`(?i)<base[^>]+href=["']([^"']*)["']`)
	if m := baseRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		if u, err := url.Parse(m[1]); err == nil && u.IsAbs() {
			return u
		}
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return &url.URL{Scheme: "https", Host: "example.com"}
	}
	return u
}

// resolveLink resolves a potentially relative href against the base URL.
// Returns the absolute URL string, or the original href if resolution fails.
func resolveLink(href string, base *url.URL) string {
	href = strings.TrimSpace(href)
	// Skip fragment-only, javascript:, mailto:, tel: links
	if href == "" || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "javascript:") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "data:") {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	resolved := base.ResolveReference(ref)
	return resolved.String()
}

// isProxyableLink returns true if the link should be routed through the reader.
// Only proxy http/https links to external web pages, not fragments, mailto, etc.
func isProxyableLink(href string) bool {
	if href == "" || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "javascript:") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "data:") {
		return false
	}
	// Only proxy http/https URLs
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")
}

// rewriteTwitterURL rewrites twitter.com and x.com URLs to use a Nitter instance
// so we get static HTML instead of a JavaScript shell.
// Returns the rewritten URL and true if it was rewritten, or the original URL and false.
func rewriteTwitterURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "twitter.com" || host == "www.twitter.com" ||
		host == "x.com" || host == "www.x.com" ||
		host == "mobile.twitter.com" || host == "mobile.x.com" {
		parsed.Host = nitterInstance
		parsed.Scheme = "https"
		return parsed.String(), true
	}
	return rawURL, false
}

// extractTitle pulls the <title> from HTML.
var titleRe = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)

func extractTitle(htmlStr string) string {
	m := titleRe.FindStringSubmatch(htmlStr)
	if len(m) < 2 {
		return ""
	}
	title := html.UnescapeString(stripTags(m[1]))
	title = strings.TrimSpace(collapseWhitespace(title))
	if len(title) > 200 {
		title = title[:200]
	}
	return title
}

// Tags and patterns to remove before extracting text
var (
	// Remove script, style, nav, header, footer, aside, noscript elements and their content
	removeScriptRe  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	removeStyleRe   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	removeNoscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	removeIframeRe  = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	removeSvgRe     = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	removeNavRe     = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	removeHeaderRe  = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	removeFooterRe  = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	removeAsideRe   = regexp.MustCompile(`(?is)<aside[^>]*>.*?</aside>`)
	removeFormRe    = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)
	// Remove HTML comments
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	// Remove all HTML tags
	tagRe = regexp.MustCompile(`<[^>]+>`)
	// Collapse multiple whitespace/newlines
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
	multiSpaceRe   = regexp.MustCompile(`[ \t]+`)
	// Common cookie/popup/ad patterns to remove
	junkPatterns = []string{
		"accept cookies", "cookie policy", "cookie settings", "we use cookies",
		"subscribe to our newsletter", "sign up for our newsletter",
		"advertisement", "sponsored content",
		"skip to content", "skip to main content",
		"jump to recipe",
	}
)

func extractReadableContent(htmlStr string) string {
	// Try to find the main content area first
	content := extractMainContent(htmlStr)

	// Remove unwanted blocks
	content = commentRe.ReplaceAllString(content, "")
	for _, re := range []*regexp.Regexp{
		removeScriptRe, removeStyleRe, removeNoscriptRe, removeIframeRe,
		removeSvgRe, removeNavRe, removeHeaderRe, removeFooterRe,
		removeAsideRe, removeFormRe,
	} {
		content = re.ReplaceAllString(content, "")
	}

	// Convert block elements to newlines for paragraph breaks
	blockTags := regexp.MustCompile(`(?i)</(p|div|article|section|h[1-6]|li|tr|blockquote|br\s*/?)>`)
	content = blockTags.ReplaceAllString(content, "\n\n")
	brTag := regexp.MustCompile(`(?i)<br\s*/?>`)
	content = brTag.ReplaceAllString(content, "\n")

	// Convert list items to bullets
	liTag := regexp.MustCompile(`(?i)<li[^>]*>`)
	content = liTag.ReplaceAllString(content, "\n• ")

	// Strip remaining HTML tags
	content = tagRe.ReplaceAllString(content, "")

	// Unescape HTML entities
	content = html.UnescapeString(content)

	// Collapse whitespace
	content = multiSpaceRe.ReplaceAllString(content, " ")
	// Clean up lines
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			cleaned = append(cleaned, "")
			continue
		}
		// Skip junk lines (cookie banners, etc.)
		lower := strings.ToLower(line)
		skip := false
		for _, junk := range junkPatterns {
			if strings.Contains(lower, junk) && len(line) < 200 {
				skip = true
				break
			}
		}
		if !skip {
			cleaned = append(cleaned, line)
		}
	}

	content = strings.Join(cleaned, "\n")
	content = multiNewlineRe.ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	// Limit output length to ~50k chars to keep responses reasonable
	if len(content) > 50000 {
		content = content[:50000] + "\n\n[Content truncated]"
	}

	return content
}

// extractMainContent tries to find the main content area using common HTML patterns.
func extractMainContent(htmlStr string) string {
	// Try <main> tag first
	mainRe := regexp.MustCompile(`(?is)<main[^>]*>(.*)</main>`)
	if m := mainRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		return m[1]
	}

	// Try <article> tag
	articleRe := regexp.MustCompile(`(?is)<article[^>]*>(.*)</article>`)
	if m := articleRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		return m[1]
	}

	// Try role="main"
	roleMainRe := regexp.MustCompile(`(?is)<[^>]+role=["']main["'][^>]*>(.*?)</div>`)
	if m := roleMainRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		return m[1]
	}

	// Try common content class names
	contentRe := regexp.MustCompile(`(?is)<div[^>]+class="[^"]*(?:content|post-body|entry-content|article-body|recipe-body)[^"]*"[^>]*>(.*?)</div>`)
	if m := contentRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		return m[1]
	}

	// Fall back to the <body>
	bodyRe := regexp.MustCompile(`(?is)<body[^>]*>(.*)</body>`)
	if m := bodyRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		return m[1]
	}

	return htmlStr
}

// stripTags removes all HTML tags from a string.
func stripTags(s string) string {
	return tagRe.ReplaceAllString(s, "")
}

// collapseWhitespace replaces runs of whitespace with a single space.
func collapseWhitespace(s string) string {
	return multiSpaceRe.ReplaceAllString(s, " ")
}

// isPrivateHost returns true if the host looks like a private/internal address.
func isPrivateHost(host string) bool {
	if host == "localhost" || host == "" {
		return true
	}
	if strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "192.168.") || host == "::1" || host == "0.0.0.0" {
		return true
	}
	// Block 172.16.0.0/12
	if strings.HasPrefix(host, "172.") {
		parts := strings.SplitN(host, ".", 3)
		if len(parts) >= 2 {
			var second int
			fmt.Sscanf(parts[1], "%d", &second)
			if second >= 16 && second <= 31 {
				return true
			}
		}
	}
	// Block metadata endpoints
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return true
	}
	return false
}
