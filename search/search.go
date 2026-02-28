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
	"sync"
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
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave search API error: %s: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var braveResp BraveResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, err
	}

	return braveResp.Web.Results, nil
}

// Handler serves the /search page.
// It runs a local data search and a Brave web search in parallel,
// then renders both sets of results in separate sections.
func Handler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Render search bar
	searchBar := `<form class="search-bar" action="/search" method="GET">` +
		`<input type="text" name="q" placeholder="Search the web..." value="` +
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
		w.Write([]byte(app.RenderHTMLForRequest("Search", "Search", content, r)))
		return
	}

	// Run local and Brave searches in parallel
	var (
		localResults []*data.IndexEntry
		braveResults []BraveResult
		braveErr     error
		wg           sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		localResults = data.Search(query, 10)
	}()

	go func() {
		defer wg.Done()
		braveResults, braveErr = searchBrave(query, 10)
	}()

	wg.Wait()

	// Consume quota after successful search
	wallet.ConsumeQuota(sess.Account, wallet.OpWebSearch)

	var b strings.Builder
	b.WriteString(searchBar)

	// Local results section
	b.WriteString(`<h3>Local Results</h3>`)
	if len(localResults) == 0 {
		b.WriteString(`<p class="empty">No local results found.</p>`)
	} else {
		for _, entry := range localResults {
			link := entryLink(entry)
			b.WriteString(`<div class="card" style="margin-bottom:12px;">`)
			b.WriteString(`<div><a href="` + html.EscapeString(link) + `" class="card-title">` +
				html.EscapeString(entry.Title) + `</a>`)
			b.WriteString(` <span class="category" style="font-size:11px;margin-left:6px;">` +
				html.EscapeString(entry.Type) + `</span></div>`)
			if entry.Content != "" {
				snippet := truncate(entry.Content, 160)
				b.WriteString(`<p class="card-desc" style="margin:4px 0 0;">` +
					html.EscapeString(snippet) + `</p>`)
			}
			b.WriteString(`</div>`)
		}
	}

	// Web results section
	b.WriteString(`<h3 style="margin-top:24px;">On the Web</h3>`)
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
			b.WriteString(`<div style="font-size:11px;color:#888;margin-top:2px;">` +
				html.EscapeString(result.URL) + `</div>`)
			b.WriteString(`</div>`)
		}
	}

	pageHTML := app.RenderHTMLForRequest("Search: "+query, "Search results for "+query, b.String(), r)
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
