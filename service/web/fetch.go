package web

import (
	"context"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/quota"
)

// nitterInstance is the Nitter instance used to fetch Twitter/X content.
// Nitter renders tweets as static HTML, which our extractor can parse.
var nitterInstance = "nitter.poast.org"

var fetchLookupIP = net.DefaultResolver.LookupIPAddr

var fetchClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:       http.ProxyFromEnvironment,
		DialContext: fetchDialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if err := validateResolvedFetchURL(req.Context(), req.URL); err != nil {
			return err
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
		app.Respond(w, r, app.Response{Title: "Fetch", Description: "Web Fetch", HTML: content})
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

	// Fetching a page is priced at zero: it costs this instance bandwidth and
	// nothing else. So this asks for a session only where an operator has given
	// it a price, and answers a guest everywhere else.
	caller, ok := app.BillableCaller(w, r, quota.OpWebFetch)
	if !ok {
		return
	}

	// Fetch the page
	title, body, fetchErr := FetchAndExtract(rawURL)

	// Only charge on success
	if fetchErr == nil {
		app.Charge(caller, quota.OpWebFetch)
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
		b.WriteString(`<p class="text-xs text-muted mb-3">`)
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

	app.Respond(w, r, app.Response{Title: "Fetch", Description: "Fetched page", HTML: b.String()})
}

// FetchAndExtract fetches a URL and returns the page title and cleaned readable text.
func FetchAndExtract(rawURL string) (string, string, error) {
	// Rewrite Twitter/X URLs to Nitter for static HTML
	fetchURL, _ := rewriteTwitterURL(rawURL)
	if err := validateRawFetchURL(fetchURL); err != nil {
		return "", "", err
	}

	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return "", "", err
	}
	if err := validateResolvedFetchURL(req.Context(), req.URL); err != nil {
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
	if err := validateRawFetchURL(fetchURL); err != nil {
		return "", "", err
	}

	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return "", "", err
	}
	if err := validateResolvedFetchURL(req.Context(), req.URL); err != nil {
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

	title, readable := readability(content, rawURL)

	return title, readable, nil
}

// resolveLink resolves a potentially relative href against the base URL.
// Returns the absolute URL string, or the original href if resolution fails.
func resolveLink(href string, base *url.URL) string {
	href = strings.TrimSpace(href)
	// Keep fragment-only and contact links unchanged, but neutralize script/data URLs.
	if href == "" || strings.HasPrefix(href, "#") ||
		hasHrefScheme(href, "mailto") ||
		hasHrefScheme(href, "tel") {
		return href
	}
	if isUnsafeHref(href) {
		return "#"
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
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || isUnsafeHref(href) ||
		hasHrefScheme(href, "mailto") || hasHrefScheme(href, "tel") {
		return false
	}
	// Only proxy http/https URLs.
	return hasHrefScheme(href, "http") || hasHrefScheme(href, "https")
}

func isUnsafeHref(href string) bool {
	return hasHrefScheme(href, "javascript") || hasHrefScheme(href, "data")
}

func hasHrefScheme(href, scheme string) bool {
	colon := strings.IndexByte(href, ':')
	if colon < 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(href[:colon]), scheme)
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
	removeScriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	removeStyleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	removeNoscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	removeIframeRe   = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	removeSvgRe      = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	removeNavRe      = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	removeHeaderRe   = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	removeFooterRe   = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	removeAsideRe    = regexp.MustCompile(`(?is)<aside[^>]*>.*?</aside>`)
	removeFormRe     = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)
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

func fetchDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := resolvePublicFetchHost(ctx, host)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no resolved addresses for %s", host)
}

func validateResolvedFetchURL(ctx context.Context, parsed *url.URL) error {
	if err := validateFetchURL(parsed); err != nil {
		return err
	}
	_, err := resolvePublicFetchHost(ctx, parsed.Hostname())
	return err
}

func resolvePublicFetchHost(ctx context.Context, host string) ([]net.IP, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if isPrivateHost(host) {
		return nil, fmt.Errorf("cannot fetch private or internal URL")
	}

	addrs, err := fetchLookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", host)
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ip := addr.IP
		if isPrivateIP(ip) {
			return nil, fmt.Errorf("cannot fetch private or internal URL")
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

func validateRawFetchURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	return validateFetchURL(parsed)
}

func validateFetchURL(parsed *url.URL) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid URL: must use http or https")
	}
	if isPrivateHost(strings.ToLower(parsed.Hostname())) {
		return fmt.Errorf("cannot fetch private or internal URL")
	}
	return nil
}

// isPrivateHost returns true if the host looks like a private/internal address.
func isPrivateHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(ip)
	}

	return strings.EqualFold(host, "metadata.google.internal")
}

func isPrivateIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
