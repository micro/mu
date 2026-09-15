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
	"mu/internal/auth"
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
// self-hoster looking for a bug; the answer is a Brave key in /admin/config.
var ErrNotConfigured = errors.New("no web search provider configured on this instance")

// searchBrave calls the Brave Search API and returns up to limit results.
func searchBrave(query string, limit int) ([]BraveResult, error) {
	// settings, not os.Getenv: a key set from /admin/config is stored, not exported
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
	// POST, and out of the URL entirely.
	//
	// What somebody searches the web for is the example everybody reaches for
	// when they say "that is private", and it was the one thing here written into
	// the URL on every query — so into the browser history, and into the access
	// log of whatever terminates TLS in front of this instance, which for a
	// self-hosted install is an nginx or a Caddy logging the full URI by default.
	// See AGENTS.md, "What may travel in a URL".
	query := strings.TrimSpace(r.PostFormValue("q"))
	if query == "" {
		// A topic chip, which is a submit button in the same form under its own
		// name. Its own name because a button called q alongside a text input
		// called q sends both, and the empty input is the one that wins.
		query = strings.TrimSpace(r.PostFormValue("topic"))
	}
	if query == "" && app.SendsJSON(r) {
		// A JSON body, which PostFormValue cannot see.
		//
		// The apps SDK posts one — mu.search(q) — and used to GET /web?q=,
		// which returned the landing page because the query was never in a
		// place this handler reads. Both halves of that are fixed: it posts
		// now, and this reads what it posts.
		var body struct {
			Q     string `json:"q"`
			Query string `json:"query"`
		}
		if err := app.DecodeJSON(r, &body); err == nil {
			query = strings.TrimSpace(body.Q)
			if query == "" {
				query = strings.TrimSpace(body.Query)
			}
		}
	}

	// The row, and the form around it, kept apart.
	//
	// .search-bar is display:flex — it is the input beside the button — so it
	// has to be the element that holds exactly those two and nothing else.
	// Putting the topic chips inside it made them a third flex child and
	// squashed the search box into a third of the width.
	//
	// So the form is the outer element and carries no styling, and the row is a
	// div inside it. The chips then sit inside the form, where a submit button
	// has to be, and outside the row, where the flexbox is.
	searchRow := `<div class="search-bar">` +
		app.CSRFField(auth.CSRFToken(r)) +
		`<input type="search" name="q" aria-label="Search the web" placeholder="Search the web..." value="` +
		html.EscapeString(query) + `" autofocus>` +
		`<button type="submit">Search</button>` +
		`</div>`
	form := func(inner string) string {
		return `<form class="form" id="web-search" action="/web" method="POST">` + inner + `</form>`
	}
	// One column, at the measure every other column page uses.
	//
	// This page had no wrapper, so it took the shell's full width while the
	// result cards under it stopped at the card measure — the search box ran
	// wider than its own results. Every page in the row under the front door's
	// box is a column of text now, and they are all the same column. See
	// --measure in mu.css.
	page := func(inner string) string { return `<div class="w-760 page-stack">` + inner + `</div>` }

	if query == "" {
		var landing strings.Builder

		// The topics go inside the form, as submit buttons.
		//
		// They were links to /web?q=<topic>, and they did nothing: this handler
		// reads the query from the POST body and not from the URL, deliberately,
		// because what somebody searches the web for is the example everybody
		// reaches for when they say "that is private" — see the note above and
		// AGENTS.md, "What may travel in a URL". So the chips were putting a
		// search term in the browser history and in the access log of whatever
		// terminates TLS, and getting the landing page back for their trouble.
		//
		// Fixing it by also reading the URL would have made them work by
		// reintroducing exactly the leak the POST-only rule exists to prevent.
		// A submit button posts, carries the CSRF field the form already has,
		// and needs no JavaScript — so a topic works on the first paint and
		// leaves nothing behind it.
		var topicChips string
		if topics := Topics(); len(topics) > 0 {
			var t strings.Builder
			t.WriteString(`<div class="recent-searches"><h3>Topics</h3><div class="recent-searches-scroll">`)
			for _, topic := range topics {
				t.WriteString(`<button type="submit" class="recent-search-item" name="topic" value="` +
					html.EscapeString(topic) + `">` +
					`<span class="recent-search-label">` + html.EscapeString(topic) + `</span>` +
					`</button>`)
			}
			t.WriteString(`</div></div>`)
			topicChips = t.String()
		}

		// One form around both, so a chip submits the same thing the button does.
		landing.WriteString(form(searchRow + topicChips))

		landing.WriteString(webRecentSearchesScript)
		content := page(landing.String())
		app.Respond(w, r, app.Response{Title: "Search", Description: "Search the web", HTML: content})
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
		quota.Charge(caller, quota.OpWebSearch, nil) //nolint:errcheck
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

	// A search gets an address, and the address says nothing.
	//
	// Post, then redirect, then get: the results are stored under a short random
	// id and the browser is sent to /web/r/<id>. That page can be bookmarked,
	// reloaded and sent to somebody, which a posted page cannot — and the id in
	// the history, in the log and in the pasted link carries eight random
	// characters rather than what was typed. See shared.go.
	//
	// Only when there is something to share. An error or an empty result set is
	// rendered here: an address for "no results" is an address for nothing, and
	// a link to a provider outage is worse than useless a day later.
	if braveErr == nil && len(braveResults) > 0 {
		if id := Share(query, braveResults); id != "" {
			http.Redirect(w, r, "/web/r/"+id, http.StatusSeeOther)
			return
		}
	}

	var b strings.Builder
	b.WriteString(form(searchRow))
	if braveErr != nil {
		app.Log("search", "Brave search error: %v", braveErr)
		b.WriteString(`<p class="empty">Web search unavailable.</p>`)
	} else {
		b.WriteString(`<p class="empty">No web results found.</p>`)
	}
	b.WriteString(rememberScript(query))
	app.Respond(w, r, app.Response{Title: "Search: " + query, Description: "Results for " + query, HTML: page(b.String())})
}

// ResultsHandler serves /web/r/<id> — a search that has an address.
//
// No auth. The results were fetched and paid for when the search ran; serving
// them again calls nothing and costs nothing, and a link only its author could
// open is not a link. See shared.go.
//
// A missing id is a 404 and not a redirect to the search page: an expired share
// and a mistyped one look the same from here, and quietly landing somebody on
// an empty search box tells them nothing about which it was.
func ResultsHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/web/r/")
	query, results, ok := Shared(id)
	if !ok {
		app.NotFound(w, r, "That search link has expired or does not exist.")
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"results": results, "query": query})
		return
	}

	var b strings.Builder
	b.WriteString(searchForm(r, query))
	b.WriteString(renderResults(results))
	b.WriteString(rememberScript(query))
	app.Respond(w, r, app.Response{
		Title:       "Search: " + query,
		Description: "Results for " + query,
		HTML:        `<div class="w-760 page-stack">` + b.String() + `</div>`,
	})
}

// renderResults is the list, wherever it is being shown.
func renderResults(results []BraveResult) string {
	if len(results) == 0 {
		return `<p class="empty">No web results found.</p>`
	}
	var b strings.Builder
	for _, result := range results {
		rid := cacheResult(result)
		b.WriteString(`<div class="card mb-3">`)
		readURL := "/web/read?id=" + rid + "&url=" + url.QueryEscape(result.URL)
		b.WriteString(`<div><a href="` + html.EscapeString(readURL) +
			`" class="card-title">` +
			html.EscapeString(result.Title) + `</a></div>`)
		if result.Description != "" {
			b.WriteString(`<p class="card-desc mt-1 m-0">` +
				html.EscapeString(stripHTML(result.Description)) + `</p>`)
		}
		meta := `<a href="` + html.EscapeString(result.URL) + `" target="_blank" rel="noopener noreferrer" class="text-muted">` + html.EscapeString(result.URL) + `</a>`
		if result.Age != "" {
			meta += ` · ` + html.EscapeString(result.Age)
		}
		b.WriteString(`<div class="text-sm text-muted mt-px">` + meta + `</div>`)
		b.WriteString(`</div>`)
	}
	return b.String()
}

// searchForm is the box, prefilled, for a page that is not Handler.
func searchForm(r *http.Request, query string) string {
	return `<form class="search-bar" id="web-search" action="/web" method="POST">` +
		app.CSRFField(auth.CSRFToken(r)) +
		`<input type="search" name="q" aria-label="Search the web" placeholder="Search the web..." value="` +
		html.EscapeString(query) + `"><button type="submit">Search</button></form>`
}

// rememberScript adds this search to the recent list in the browser.
//
// The client keeps its own list because the server does not: a record of what
// somebody searched for, keyed to them, is the thing this page is arranged not
// to have. localStorage is theirs, on their machine, and they can clear it.
func rememberScript(query string) string {
	return `<script>
	(function(){
		var KEY='mu_recent_web_searches',MAX=10;
		try{var s=localStorage.getItem(KEY);var a=s?JSON.parse(s):[];var q="` + html.EscapeString(strings.ReplaceAll(query, `"`, `\"`)) + `";
		a=a.filter(function(x){return x!==q});a.unshift(q);
		if(a.length>MAX)a=a.slice(0,MAX);localStorage.setItem(KEY,JSON.stringify(a));}catch(e){}
	})();
	</script>`
}

// htmlTagRe matches any HTML tag.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes HTML tags from s and unescapes HTML entities.
func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// webRecentSearchesScript is the client-side JS for recent web searches (localStorage).
var webRecentSearchesScript = app.RecentSearches("web-search", "mu_recent_web_searches")
