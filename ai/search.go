package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"mu/app"
)

// SearchResult represents a web search result
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearch performs a web search using DuckDuckGo.
// Tries the instant answer API first, then falls back to DuckDuckGo lite HTML.
// Returns up to 8 results.
func WebSearch(query string) ([]SearchResult, error) {
	app.Log("ai", "[Search] Query: %s", query)

	// Try DuckDuckGo instant answers API first (works for factual queries)
	results, err := duckduckgoSearch(query)
	if err == nil && len(results) > 0 {
		app.Log("ai", "[Search] DDG instant: %d results", len(results))
		return results, nil
	}

	// Fallback: scrape DuckDuckGo lite HTML (works for general queries)
	results, err = duckduckgoLiteSearch(query)
	if err != nil {
		app.Log("ai", "[Search] DDG lite error: %v", err)
		return nil, err
	}

	app.Log("ai", "[Search] DDG lite: %d results", len(results))
	return results, nil
}

func duckduckgoSearch(query string) ([]SearchResult, error) {
	// DuckDuckGo instant answer API
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var ddg struct {
		Abstract       string `json:"Abstract"`
		AbstractSource string `json:"AbstractSource"`
		AbstractURL    string `json:"AbstractURL"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	json.Unmarshal(body, &ddg)

	var results []SearchResult

	// Add abstract if available
	if ddg.Abstract != "" {
		results = append(results, SearchResult{
			Title:   ddg.AbstractSource,
			URL:     ddg.AbstractURL,
			Snippet: ddg.Abstract,
		})
	}

	// Add related topics
	for _, topic := range ddg.RelatedTopics {
		if topic.Text != "" && topic.FirstURL != "" && len(results) < 5 {
			title := topic.Text
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			results = append(results, SearchResult{
				Title:   title,
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
	}

	return results, nil
}

// duckduckgoLiteSearch scrapes DuckDuckGo lite (HTML) for actual web results.
// This works for general queries where the instant answer API returns nothing.
func duckduckgoLiteSearch(query string) ([]SearchResult, error) {
	liteURL := "https://lite.duckduckgo.com/lite/"

	form := url.Values{}
	form.Set("q", query)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", liteURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; mu-search/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	return parseDDGLiteHTML(html), nil
}

// reLink matches DDG lite result links.
var reLink = regexp.MustCompile(`<a[^>]+rel="nofollow"[^>]+href="([^"]+)"[^>]*class="result-link"[^>]*>([^<]+)</a>`)

// reSnippet matches DDG lite result snippets (the <td> class="result-snippet").
var reSnippet = regexp.MustCompile(`class="result-snippet"[^>]*>([^<]+)<`)

func parseDDGLiteHTML(html string) []SearchResult {
	links := reLink.FindAllStringSubmatch(html, -1)
	snippets := reSnippet.FindAllStringSubmatch(html, -1)

	var results []SearchResult
	for i, m := range links {
		if len(results) >= 8 {
			break
		}
		u := strings.TrimSpace(m[1])
		title := strings.TrimSpace(m[2])
		snippet := ""
		if i < len(snippets) {
			snippet = strings.TrimSpace(snippets[i][1])
		}
		if u != "" && title != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     u,
				Snippet: snippet,
			})
		}
	}
	return results
}

// FormatSearchResults formats search results for inclusion in RAG context
func FormatSearchResults(results []SearchResult) []string {
	var formatted []string
	for _, r := range results {
		text := fmt.Sprintf("%s: %s", r.Title, r.Snippet)
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		if r.URL != "" {
			text += fmt.Sprintf(" (Source: %s)", r.URL)
		}
		formatted = append(formatted, text)
	}
	return formatted
}

// ShouldWebSearch determines if a query would benefit from web search
// based on keywords suggesting current events or specific data needs
func ShouldWebSearch(query string) bool {
	query = strings.ToLower(query)

	keywords := []string{
		"current", "today", "latest", "recent", "now",
		"price", "worth", "cost", "rate",
		"news", "happening", "update",
		"who is", "what is", "where is",
		"how much", "how many",
		"compare", "versus", "vs",
		"best", "top", "recommend",
	}

	for _, kw := range keywords {
		if strings.Contains(query, kw) {
			return true
		}
	}

	return false
}
