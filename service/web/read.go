package web

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
)

// ReadHandler serves /read — a clean reader view for any web page.
// Fetches the URL, extracts readable content, and renders it in a clean layout.
func ReadHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))

	if rawURL == "" {
		http.Redirect(w, r, "/web", http.StatusFound)
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

	host := strings.ToLower(parsed.Hostname())
	if isPrivateHost(host) {
		app.BadRequest(w, r, "Cannot fetch private or internal URLs")
		return
	}

	// Same gate as web fetch, because it is the same operation with the HTML kept.
	caller, ok := app.BillableCaller(w, r, quota.OpWebFetch)
	if !ok {
		return
	}

	// Fetch and extract — use HTML-preserving extraction with proxied links
	title, body, fetchErr := FetchAndExtractHTMLProxied(rawURL)

	if fetchErr == nil {
		app.Charge(caller, quota.OpWebFetch)
	}

	// JSON response for API callers
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

	// HTML — render as a clean article page
	if fetchErr != nil {
		app.Log("read", "Fetch error for %s: %v", rawURL, fetchErr)

		// Try to show cached search result data instead of a bare error
		resultID := r.URL.Query().Get("id")
		cached := CachedResult(resultID)

		var b strings.Builder
		b.WriteString(`<div id="news-article">`)
		if cached != nil && cached.Title != "" {
			if cached.Description != "" {
				b.WriteString(fmt.Sprintf(`<p class="text-secondary m-0 mb-3">%s</p>`, html.EscapeString(cached.Description)))
			}
			b.WriteString(`<p class="text-muted text-sm m-0 mb-3">The full page content couldn't be loaded. You can visit the original site below.</p>`)
		} else {
			b.WriteString(fmt.Sprintf(`<p class="text-muted">This page couldn't be loaded.</p>`))
		}
		b.WriteString(fmt.Sprintf(`<div class="article-actions mt-4">
			<a href="%s" target="_blank" rel="noopener noreferrer" class="semibold">Visit original site →</a>
			<span class="sep">·</span>
			<a href="javascript:history.back()">← Back to results</a>
		</div>`, html.EscapeString(rawURL)))
		b.WriteString(`</div>`)

		pageTitle := "Read"
		if cached != nil && cached.Title != "" {
			pageTitle = cached.Title
		}
		app.Respond(w, r, app.Response{Title: pageTitle, Description: "Read Page", HTML: b.String()})
		return
	}

	domain := getDomain(rawURL)
	if title == "" {
		title = domain
	}

	var b strings.Builder
	b.WriteString(`<div id="news-article">`)

	// Meta: source domain
	b.WriteString(fmt.Sprintf(`<div class="article-meta"><span>Source: <i>%s</i></span></div>`, html.EscapeString(domain)))

	// Render sanitized HTML content
	b.WriteString(`<div class="reader-content">`)
	b.WriteString(body)
	b.WriteString(`</div>`)

	// Actions
	b.WriteString(fmt.Sprintf(`
		<div class="article-actions">
			<a href="%s" target="_blank" rel="noopener noreferrer">Visit Original →</a>
			<span class="mx-2">·</span>
			<a href="#" onclick="navigator.share ? navigator.share({title: document.title, url: '%s'}) : navigator.clipboard.writeText('%s').then(() => alert('Link copied!')); return false;">Share →</a>
		</div>`,
		html.EscapeString(rawURL),
		html.EscapeString(strings.ReplaceAll(rawURL, "'", "\\'")),
		html.EscapeString(strings.ReplaceAll(rawURL, "'", "\\'")),
	))

	b.WriteString(`<div class="article-back"><a href="javascript:history.back()">← Back to results</a></div>`)
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: title, Description: title, HTML: b.String()})
}

// getDomain extracts the domain from a URL string.
func getDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := u.Hostname()
	if host == "" {
		return rawURL
	}
	return host
}
