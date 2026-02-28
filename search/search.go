package search

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"
	"mu/wallet"
)

// BraveResult represents a single result from the Brave Search API
type BraveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Age         string `json:"age"`
}

// BraveResponse is the top-level Brave Search API response
type BraveResponse struct {
	Web struct {
		Results []BraveResult `json:"results"`
	} `json:"web"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// searchBrave calls the Brave Search API and returns up to limit results.
func searchBrave(query string, limit int) ([]BraveResult, error) {
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY not set")
	}

	reqURL := "https://api.search.brave.com/res/v1/web/search?q=" +
		url.QueryEscape(query) + fmt.Sprintf("&count=%d", limit)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	start := time.Now()
	resp, err := httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		app.RecordAPICall("brave", "GET", reqURL, 0, duration, err, "", "")
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		app.RecordAPICall("brave", "GET", reqURL, resp.StatusCode, duration, readErr, "", "")
		return nil, readErr
	}

	if resp.StatusCode != http.StatusOK {
		callErr := fmt.Errorf("brave search API error: %s: %s", resp.Status, string(body))
		app.RecordAPICall("brave", "GET", reqURL, resp.StatusCode, duration, callErr, "", string(body))
		return nil, callErr
	}

	app.RecordAPICall("brave", "GET", reqURL, resp.StatusCode, duration, nil, "", "")

	var braveResp BraveResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, err
	}

	return braveResp.Web.Results, nil
}

// Handler serves the /search page (local data index only, free, no auth required).
func Handler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Render search bar
	searchBar := `<form class="search-bar" action="/search" method="GET">` +
		`<input type="text" name="q" placeholder="Search..." value="` +
		html.EscapeString(query) + `" autofocus>` +
		`<button type="submit">Search</button>` +
		`</form>`

	if query == "" {
		content := searchBar + `<p class="empty">Enter a query above to search.</p>`
		w.Write([]byte(app.RenderHTMLForRequest("Search", "Search", content, r)))
		return
	}

	// Limit query length to prevent abuse
	if len(query) > 256 {
		app.BadRequest(w, r, "Search query must not exceed 256 characters")
		return
	}

	localResults := data.Search(query, 10)

	var b strings.Builder
	b.WriteString(searchBar)

	if len(localResults) == 0 {
		b.WriteString(`<p class="empty">No results found.</p>`)
	} else {
		for _, entry := range localResults {
			link := entryLink(entry)
			b.WriteString(`<div class="card" style="margin-bottom:12px;">`)
			b.WriteString(`<div><a href="` + html.EscapeString(link) + `" class="card-title">` +
				html.EscapeString(entry.Title) + `</a>`)
			b.WriteString(` <span class="category" style="font-size:11px;margin-left:6px;">` +
				html.EscapeString(entry.Type) + `</span>`)
			if !entry.IndexedAt.IsZero() {
				b.WriteString(` <span style="font-size:11px;color:#888;margin-left:4px;">` +
					html.EscapeString(app.TimeAgo(entry.IndexedAt)) + `</span>`)
			}
			b.WriteString(`</div>`)
			if entry.Content != "" {
				snippet := truncate(entry.Content, 160)
				b.WriteString(`<p class="card-desc" style="margin:4px 0 0;">` +
					html.EscapeString(snippet) + `</p>`)
			}
			b.WriteString(`</div>`)
		}
	}

	pageHTML := app.RenderHTMLForRequest("Search: "+query, "Search results for "+query, b.String(), r)
	w.Write([]byte(pageHTML))
}

// WebHandler serves the /web page (Brave web search, paid, auth required).
func WebHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Render search bar
	searchBar := `<form class="search-bar" action="/web" method="GET">` +
		`<input type="text" name="q" placeholder="Search the web..." value="` +
		html.EscapeString(query) + `" autofocus>` +
		`<button type="submit">Search</button>` +
		`</form>`

	if query == "" {
		content := searchBar + `<p class="empty">Enter a query above to search the web.</p>`
		w.Write([]byte(app.RenderHTMLForRequest("Web Search", "Web Search", content, r)))
		return
	}

	// Limit query length to prevent abuse
	if len(query) > 256 {
		app.BadRequest(w, r, "Search query must not exceed 256 characters")
		return
	}

	// Require authentication to charge for the search
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	// Check quota (5p per search)
	canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpWebSearch)
	if !canProceed {
		content := searchBar + wallet.QuotaExceededPage(wallet.OpWebSearch, cost)
		w.Write([]byte(app.RenderHTMLForRequest("Web Search", "Web Search", content, r)))
		return
	}

	braveResults, braveErr := searchBrave(query, 10)

	// Only consume quota on success to avoid charging for failed API calls
	if braveErr == nil {
		wallet.ConsumeQuota(sess.Account, wallet.OpWebSearch)
	}

	var b strings.Builder
	b.WriteString(searchBar)

	if braveErr != nil {
		app.Log("search", "Brave search error: %v", braveErr)
		b.WriteString(`<p class="empty">Web search unavailable.</p>`)
	} else if len(braveResults) == 0 {
		b.WriteString(`<p class="empty">No web results found.</p>`)
	} else {
		for _, result := range braveResults {
			b.WriteString(`<div class="card" style="margin-bottom:12px;">`)
			b.WriteString(`<div><a href="` + html.EscapeString(result.URL) +
				`" class="card-title" target="_blank" rel="noopener noreferrer">` +
				html.EscapeString(result.Title) + `</a></div>`)
			if result.Description != "" {
				b.WriteString(`<p class="card-desc" style="margin:4px 0 0;">` +
					html.EscapeString(result.Description) + `</p>`)
			}
			meta := html.EscapeString(result.URL)
			if result.Age != "" {
				meta += ` · ` + html.EscapeString(result.Age)
			}
			b.WriteString(`<div style="font-size:11px;color:#888;margin-top:2px;">` + meta + `</div>`)
			b.WriteString(`</div>`)
		}
	}

	pageHTML := app.RenderHTMLForRequest("Web: "+query, "Web results for "+query, b.String(), r)
	w.Write([]byte(pageHTML))
}

// entryLink returns the URL for a local search result entry.
func entryLink(entry *data.IndexEntry) string {
	switch entry.Type {
	case "news":
		return "/news?id=" + url.QueryEscape(entry.ID)
	case "video":
		if u, ok := entry.Metadata["url"].(string); ok && u != "" {
			return u
		}
		return "/video"
	case "blog":
		return "/post?id=" + url.QueryEscape(entry.ID)
	default:
		return "/" + entry.Type
	}
}

// truncate shortens s to at most max runes, appending "…" if truncated.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
