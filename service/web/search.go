package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/settings"
)

// Load initializes the search package. It registers no service of its own:
// the web search capability is web.Search, and this package provides the
// provider, the reader and the /search page that sit around it.
// BraveResult represents a single result from the Brave Search API
type BraveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Age         string `json:"age"`
}

// resultCache stores search results so the read page can show metadata
// when the full page content can't be fetched.
var (
	resultCacheMu sync.RWMutex
	resultCache   = map[string]*BraveResult{} // url hash → result
)

func cacheResult(r BraveResult) string {
	id := fmt.Sprintf("%x", len(r.URL)+len(r.Title))
	// Simple hash from URL
	h := uint32(0)
	for _, c := range r.URL {
		h = h*31 + uint32(c)
	}
	id = fmt.Sprintf("%08x", h)
	resultCacheMu.Lock()
	resultCache[id] = &r
	if len(resultCache) > 500 {
		for k := range resultCache {
			delete(resultCache, k)
			if len(resultCache) <= 250 {
				break
			}
		}
	}
	resultCacheMu.Unlock()
	return id
}

// CachedResult returns a cached search result by ID.
func CachedResult(id string) *BraveResult {
	resultCacheMu.RLock()
	defer resultCacheMu.RUnlock()
	return resultCache[id]
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

// ErrNotConfigured means this instance has no web search provider, which is a
// different thing from web search failing — and the difference matters to
// whoever has to fix it. "Unavailable right now" reads as an outage and sends a
// self-hoster looking for a bug; the answer is a Brave key in /admin/env.
var ErrNotConfigured = errors.New("no web search provider configured on this instance")

// searchBrave calls the Brave Search API and returns up to limit results.
func searchBrave(query string, limit int) ([]BraveResult, error) {
	// settings, not os.Getenv: a key set from /admin/env is stored, not exported
	// into the process, so a self-hoster who configured web search in the
	// browser still got "not set" here.
	apiKey := settings.Get("BRAVE_API_KEY")
	if apiKey == "" {
		return nil, ErrNotConfigured
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
	app.RecordUsage("brave", "search", 0.5, nil) // ~$0.005/query

	var braveResp BraveResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, err
	}

	return braveResp.Web.Results, nil
}

// Handler serves the /web page (Brave web search, paid, auth required).
func Handler(w http.ResponseWriter, r *http.Request) {
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
		topics := Topics()
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
		w.Write([]byte(app.RenderHTMLForRequest("Search", "Search the web", content, r)))
		return
	}

	// Limit query length to prevent abuse
	if len(query) > 256 {
		app.BadRequest(w, r, "Search query must not exceed 256 characters")
		return
	}

	// Who pays, if anybody does. A search is metered where Brave is being paid
	// for and free where it is not, and the gate asks which before it asks who.
	caller, ok := app.BillableCaller(w, r, quota.OpWebSearch)
	if !ok {
		return
	}

	braveResults, braveErr := SearchBraveCached(query, 10)

	// Only charge on success, so a provider outage is not something the caller
	// pays for.
	if braveErr == nil {
		app.Charge(caller, quota.OpWebSearch)
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
			rid := cacheResult(result)
			b.WriteString(`<div class="card" style="margin-bottom:12px;">`)
			readURL := "/web/read?id=" + rid + "&url=" + url.QueryEscape(result.URL)
			b.WriteString(`<div><a href="` + html.EscapeString(readURL) +
				`" class="card-title">` +
				html.EscapeString(result.Title) + `</a></div>`)
			if result.Description != "" {
				b.WriteString(`<p class="card-desc" style="margin:4px 0 0;">` +
					html.EscapeString(stripHTML(result.Description)) + `</p>`)
			}
			meta := `<a href="` + html.EscapeString(result.URL) + `" target="_blank" rel="noopener noreferrer" style="color:#888;">` + html.EscapeString(result.URL) + `</a>`
			if result.Age != "" {
				meta += ` · ` + html.EscapeString(result.Age)
			}
			b.WriteString(`<div style="font-size:13px;color:#888;margin-top:2px;">` + meta + `</div>`)
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

	pageHTML := app.RenderHTMLForRequest("Search: "+query, "Results for "+query, b.String(), r)
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
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
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
      h += '<span class="recent-search-item" data-query="' + encodeURIComponent(search) + '">'
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
          var q = decodeURIComponent(item.getAttribute('data-query') || '');
          saveRecentSearch(q);
          window.location.href = '/web?q=' + encodeURIComponent(q);
        });
      }
      if (close) {
        close.addEventListener('click', function(e) {
          e.preventDefault(); e.stopPropagation();
          removeRecentSearch(decodeURIComponent(item.getAttribute('data-query') || ''));
        });
      }
    });
  }

  // Run now if the document is already parsed, not only on DOMContentLoaded.
  //
  // A soft navigation swaps #content and re-creates the scripts inside it, so
  // this script runs again — but DOMContentLoaded fired once, on the first real
  // page load, and never again. So arriving at /search by clicking a link left
  // the recent searches unrendered and the form unwired, and reloading fixed
  // it, which is the tell.
  function wireSearch() {
    displayRecentSearches();
    var form = document.querySelector('form[action="/web"]');
    if (form) {
      form.addEventListener('submit', function() {
        var q = form.querySelector('input[name="q"]');
        if (q && q.value.trim()) saveRecentSearch(q.value.trim());
      });
    }
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireSearch);
  } else {
    wireSearch();
  }
</script>
`
