package search

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
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
	Age         string `json:"age"`
}

// BraveResponse is the top-level Brave Search API response
type BraveResponse struct {
	Web struct {
		Results []BraveResult `json:"results"`
	} `json:"web"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// braveCache stores recent Brave search results keyed by query to avoid
// repeated API calls for the same query within a short window (5 minutes).
var braveCache struct {
	sync.RWMutex
	entries map[string]braveCacheEntry
}

type braveCacheEntry struct {
	results []BraveResult
	fetched time.Time
}

const braveCacheTTL = 5 * time.Minute
const bravePreviewCacheTTL = 24 * time.Hour

func init() {
	braveCache.entries = make(map[string]braveCacheEntry)
}

// SearchBraveCached returns cached results if available, otherwise calls searchBrave.
func SearchBraveCached(query string, limit int) ([]BraveResult, error) {
	return searchBraveCachedWithTTL(query, limit, braveCacheTTL)
}

func searchBraveCachedWithTTL(query string, limit int, ttl time.Duration) ([]BraveResult, error) {
	key := strings.ToLower(strings.TrimSpace(query))
	braveCache.RLock()
	if e, ok := braveCache.entries[key]; ok && time.Since(e.fetched) < ttl {
		braveCache.RUnlock()
		return e.results, nil
	}
	braveCache.RUnlock()

	results, err := searchBrave(query, limit)
	if err != nil {
		return nil, err
	}

	braveCache.Lock()
	braveCache.entries[key] = braveCacheEntry{results: results, fetched: time.Now()}
	// Evict old entries
	for k, v := range braveCache.entries {
		if time.Since(v.fetched) > braveCacheTTL {
			delete(braveCache.entries, k)
		}
	}
	braveCache.Unlock()

	return results, nil
}

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
			b.WriteString(` <span class="category" style="font-size:11px;">` +
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
		var landing strings.Builder
		landing.WriteString(searchBar)
		landing.WriteString(`<div id="recent-searches-container"></div>`)

		// Topics from indexed content
		topics := GetTopics()
		if len(topics) > 0 {
			landing.WriteString(`<div class="recent-searches"><h3>Topics</h3><div class="recent-searches-scroll">`)
			for _, topic := range topics {
				landing.WriteString(`<a class="recent-search-item" href="/web?q=` + url.QueryEscape(topic) + `">`)
				landing.WriteString(`<span class="recent-search-label">` + html.EscapeString(topic) + `</span>`)
				landing.WriteString(`</a>`)
			}
			landing.WriteString(`</div></div>`)
		}

		landing.WriteString(webRecentSearchesScript)
		content := landing.String()
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

	braveResults, braveErr := SearchBraveCached(query, 10)

	// Only consume quota on success to avoid charging for failed API calls
	if braveErr == nil {
		wallet.ConsumeQuota(sess.Account, wallet.OpWebSearch)
	}

	// JSON response for API/MCP callers
	if app.WantsJSON(r) {
		if braveErr != nil {
			app.RespondError(w, http.StatusServiceUnavailable, "web search unavailable")
			return
		}
		app.RespondJSON(w, map[string]interface{}{"results": braveResults, "query": query})
		return
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
					html.EscapeString(stripHTML(result.Description)) + `</p>`)
			}
			meta := html.EscapeString(result.URL)
			if result.Age != "" {
				meta += ` · ` + html.EscapeString(result.Age)
			}
			b.WriteString(`<div style="font-size:11px;color:#888;margin-top:2px;">` + meta + `</div>`)
			b.WriteString(`</div>`)
		}
	}

	// Save this search to recent searches on the client
	b.WriteString(`<script>
	(function(){
		var KEY='mu_recent_web_searches',MAX=10;
		try{var s=localStorage.getItem(KEY);var a=s?JSON.parse(s):[];var q="` + html.EscapeString(strings.ReplaceAll(query, `"`, `\"`)) + `";
		a=a.filter(function(x){return x!==q});a.unshift(q);
		if(a.length>MAX)a=a.slice(0,MAX);localStorage.setItem(KEY,JSON.stringify(a));}catch(e){}
	})();
	</script>`)

	pageHTML := app.RenderHTMLForRequest("Web: "+query, "Web results for "+query, b.String(), r)
	w.Write([]byte(pageHTML))
}

// PreviewHandler returns cached Brave results as JSON for the landing page.
// It uses a fixed "trending" query so the landing page can show web results
// without requiring auth. Results are cached for 24 hours.
func PreviewHandler(w http.ResponseWriter, r *http.Request) {
	results, err := searchBraveCachedWithTTL("what are AI agents", 5, bravePreviewCacheTTL)
	if err != nil {
		app.RespondJSON(w, map[string]interface{}{"results": []BraveResult{}})
		return
	}
	app.RespondJSON(w, map[string]interface{}{"results": results})
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

// htmlTagRe matches any HTML tag.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes HTML tags from s and unescapes HTML entities.
func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// webRecentSearchesScript is the client-side JS for recent web searches (localStorage).
var webRecentSearchesScript = `
<script>
  const MAX_RECENT_SEARCHES = 10;
  const STORAGE_KEY = 'mu_recent_web_searches';

  function escapeHTML(text) {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
               .replace(/"/g, '&quot;').replace(/'/g, '&#039;');
  }

  function loadRecentSearches() {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch (e) { return []; }
  }

  function saveRecentSearch(query) {
    if (!query || !query.trim()) return;
    try {
      let searches = loadRecentSearches();
      searches = searches.filter(s => s !== query);
      searches.unshift(query);
      if (searches.length > MAX_RECENT_SEARCHES) searches = searches.slice(0, MAX_RECENT_SEARCHES);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(searches));
    } catch (e) {}
  }

  function removeRecentSearch(query) {
    try {
      let searches = loadRecentSearches().filter(s => s !== query);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(searches));
      displayRecentSearches();
    } catch (e) {}
  }

  function displayRecentSearches() {
    const searches = loadRecentSearches();
    const container = document.getElementById('recent-searches-container');
    if (!container || searches.length === 0) { if (container) container.innerHTML = ''; return; }

    let h = '<div class="recent-searches"><h3>Recent Searches</h3><div class="recent-searches-scroll">';
    searches.forEach(function(search) {
      const escaped = escapeHTML(search);
      h += '<span class="recent-search-item" data-query="' + escaped + '">'
         + '<span class="recent-search-label">' + escaped + '</span>'
         + '<span class="recent-search-close" title="Remove">&times;</span>'
         + '</span>';
    });
    h += '</div></div>';
    container.innerHTML = h;

    container.querySelectorAll('.recent-search-item').forEach(function(item) {
      var label = item.querySelector('.recent-search-label');
      var close = item.querySelector('.recent-search-close');
      if (label) {
        label.addEventListener('click', function(e) {
          e.preventDefault(); e.stopPropagation();
          var q = item.getAttribute('data-query');
          saveRecentSearch(q);
          window.location.href = '/web?q=' + encodeURIComponent(q);
        });
      }
      if (close) {
        close.addEventListener('click', function(e) {
          e.preventDefault(); e.stopPropagation();
          removeRecentSearch(item.getAttribute('data-query'));
        });
      }
    });
  }

  document.addEventListener('DOMContentLoaded', function() {
    displayRecentSearches();
    var form = document.querySelector('form[action="/web"]');
    if (form) {
      form.addEventListener('submit', function() {
        var q = form.querySelector('input[name="q"]');
        if (q && q.value.trim()) saveRecentSearch(q.value.trim());
      });
    }
  });
</script>
`

// truncate shortens s to at most max runes, appending "…" if truncated.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
