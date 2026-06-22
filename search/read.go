package search

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/wallet"
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

	// Check quota (same as web_fetch)
	canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpWebFetch)
	if !canProceed {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusPaymentRequired, fmt.Sprintf("reading pages requires %d credits", cost))
			return
		}
		content := wallet.QuotaExceededPage(wallet.OpWebFetch, cost)
		w.Write([]byte(app.RenderHTMLForRequest("Read", "Read Page", content, r)))
		return
	}

	// Fetch and extract — use HTML-preserving extraction with proxied links
	title, body, fetchErr := FetchAndExtractHTMLProxied(rawURL)

	if fetchErr == nil {
		wallet.ConsumeQuota(sess.Account, wallet.OpWebFetch)
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
		cached := GetCachedResult(resultID)

		var b strings.Builder
		b.WriteString(`<div id="news-article">`)
		if cached != nil && cached.Title != "" {
			if cached.Description != "" {
				b.WriteString(fmt.Sprintf(`<p style="color:#555;line-height:1.6;margin:0 0 12px">%s</p>`, html.EscapeString(cached.Description)))
			}
			b.WriteString(`<p style="color:#888;font-size:13px;margin:0 0 12px">The full page content couldn't be loaded. You can visit the original site below.</p>`)
		} else {
			b.WriteString(fmt.Sprintf(`<p style="color:#888">This page couldn't be loaded.</p>`))
		}
		b.WriteString(fmt.Sprintf(`<div class="article-actions" style="margin-top:16px">
			<a href="%s" target="_blank" rel="noopener noreferrer" style="font-weight:600">Visit original site →</a>
			<span style="margin:0 8px;color:#ddd">·</span>
			<a href="javascript:history.back()">← Back to results</a>
		</div>`, html.EscapeString(rawURL)))
		b.WriteString(`</div>`)

		pageTitle := "Read"
		if cached != nil && cached.Title != "" {
			pageTitle = cached.Title
		}
		w.Write([]byte(app.RenderHTMLForRequest(pageTitle, "Read Page", b.String(), r)))
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

	pageHTML := app.RenderHTMLForRequest(title, title, b.String(), r)
	w.Write([]byte(pageHTML))
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
