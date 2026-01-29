package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// WebSearch performs a web search using DuckDuckGo's instant answer API
// Falls back to scraping if needed. Returns up to 5 results.
func WebSearch(query string) ([]SearchResult, error) {
	app.Log("ai", "[Search] Query: %s", query)

	// Try DuckDuckGo instant answers API first
	results, err := duckduckgoSearch(query)
	if err != nil {
		app.Log("ai", "[Search] DuckDuckGo error: %v", err)
		return nil, err
	}

	app.Log("ai", "[Search] Found %d results", len(results))
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
			// Extract title from URL or text
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

	// Keywords suggesting need for current/external info
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
