package news

import (
	"crypto/md5"
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
	"github.com/mrz1836/go-sanitize"
	"github.com/piquette/finance-go/future"
	nethtml "golang.org/x/net/html"
	"mu/app"
	"mu/auth"
	"mu/data"
)

//go:embed feeds.json
var f embed.FS

var mutex sync.RWMutex

var feeds = map[string]string{}

var status = map[string]*Feed{}

// Semaphore to limit concurrent metadata fetches (reduces memory spike on startup)
var metadataFetchSem = make(chan struct{}, 10) // Allow max 10 concurrent fetches

// cached news html
var html string

// cached news body (without full page wrapper)
var newsBodyHtml string

// cached headlines
var headlinesHtml string

// markets
var marketsHtml string

// cached prices
var cachedPrices map[string]float64

// reminder
var reminderHtml string

// the cached feed
var feed []*Post

type Feed struct {
	Name     string
	URL      string
	Error    error
	Attempts int
	Backoff  time.Time
}

type Post struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Published   string    `json:"published"`
	Category    string    `json:"category"`
	PostedAt    time.Time `json:"posted_at"`
	Image       string    `json:"image"`
	Content     string    `json:"content"`
}

type Metadata struct {
	Created            int64
	Title              string
	Description        string
	Type               string
	Image              string
	Url                string
	Site               string
	Content            string
	Comments           string // Comments/discussion context from any source
	Summary            string // LLM-generated summary for chat context
	SummaryRequestedAt int64  // Last time we requested summary generation
	SummaryAttempts    int    // Number of times we've requested a summary
}

// htmlToText converts HTML to plain text with proper spacing
func htmlToText(html string) string {
	if html == "" {
		return ""
	}

	// Parse HTML
	doc, err := nethtml.Parse(strings.NewReader(html))
	if err != nil {
		// If parsing fails, just strip tags the simple way
		re := regexp.MustCompile(`<[^>]*>`)
		text := re.ReplaceAllString(html, " ")
		// Collapse multiple spaces
		re2 := regexp.MustCompile(`\s+`)
		return strings.TrimSpace(re2.ReplaceAllString(text, " "))
	}

	var sb strings.Builder
	var extract func(*nethtml.Node)
	extract = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			sb.WriteString(n.Data)
		}
		if n.Type == nethtml.ElementNode {
			// Preserve <a> tags with their href
			if n.Data == "a" {
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				if href != "" {
					sb.WriteString(`<a href="`)
					sb.WriteString(href)
					sb.WriteString(`" target="_blank" rel="noopener noreferrer">`)
				}
				// Process children
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extract(c)
				}
				if href != "" {
					sb.WriteString("</a>")
				}
				sb.WriteString(" ")
			} else {
				// For other elements, process children but don't preserve the tag
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extract(c)
				}
				// Add space after block elements
				switch n.Data {
				case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
					sb.WriteString(" ")
				}
			}
		} else {
			// For non-element nodes, process children
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				extract(c)
			}
		}
	}
	extract(doc)

	// Collapse multiple spaces and trim
	text := sb.String()
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(text, " "))
}

func getDomain(v string) string {
	var host string

	u, err := url.Parse(v)
	if err == nil {
		host = u.Hostname()
	} else {
		parts := strings.Split(v, "/")
		if len(parts) < 3 {
			return v
		}
		host = strings.TrimSpace(parts[2])
	}

	if strings.Contains(host, "github.io") {
		return host
	}

	parts := strings.Split(host, ".")
	if len(parts) == 2 {
		return host
	} else if len(parts) == 3 {
		return strings.Join(parts[1:3], ".")
	}
	return host
}

var Results = `
<div id="topics">%s</div>
<h1 style="margin-top: 0">Results</h1>
<div id="results">
%s
</div>`

func getSummary(post *Post) string {
	timestamp := ""
	if !post.PostedAt.IsZero() {
		timestamp = app.TimeAgo(post.PostedAt) + " · "
	}
	return fmt.Sprintf(`%sSource: <i>%s</i>`, timestamp, getDomain(post.URL))
}

func getCategoryBadge(post *Post) string {
	if post.Category == "" {
		return ""
	}
	return fmt.Sprintf(`<a href="/news#%s" class="category">%s</a>`, post.Category, post.Category)
}

func getPrices() map[string]float64 {
	app.Log("news", "Getting prices")
	rsp, err := http.Get("https://api.coinbase.com/v2/exchange-rates?currency=USD")
	if err != nil {
		app.Log("news", "Error getting prices: %v", err)
		return nil
	}
	defer rsp.Body.Close()
	b, _ := ioutil.ReadAll(rsp.Body)
	var res map[string]interface{}
	json.Unmarshal(b, &res)
	if res == nil {
		return nil
	}

	rates := res["data"].(map[string]interface{})["rates"].(map[string]interface{})

	prices := map[string]float64{}

	for k, t := range rates {
		val, err := strconv.ParseFloat(t.(string), 64)
		if err != nil {
			continue
		}
		prices[k] = 1 / val
	}

	// let's get other prices
	app.Log("news", "Getting futures prices...")
	for key, ftr := range futures {
		// Use closure to safely handle potential panics from individual futures
		func() {
			defer func() {
				if r := recover(); r != nil {
					app.Log("news", "Recovered from panic getting future %s (%s): %v", key, ftr, r)
				}
			}()

			f, err := future.Get(ftr)
			if err != nil {
				app.Log("news", "Failed to get future %s (%s): %v", key, ftr, err)
				return
			}
			if f == nil {
				app.Log("news", "Future returned nil for %s (%s)", key, ftr)
				return
			}
			// Access the price, which may panic if Quote struct is malformed
			price := f.Quote.RegularMarketPrice
			if price > 0 {
				prices[key] = price
			}
		}()
	}

	app.Log("news", "Finished getting all prices")
	return prices
}

var tickers = []string{"GBP", "XLM", "ETH", "BTC", "PAXG"}

var futures = map[string]string{
	"OIL":      "CL=F",
	"GOLD":     "GC=F",
	"COFFEE":   "KC=F",
	"OATS":     "ZO=F",
	"WHEAT":    "KE=F",
	"SILVER":   "SI=F",
	"COPPER":   "HG=F",
	"CORN":     "ZC=F",
	"SOYBEANS": "ZS=F",
}

var futuresKeys = []string{"OIL", "OATS", "COFFEE", "WHEAT", "GOLD"}

// ContentParser functions clean up feed descriptions
type ContentParser struct {
	Name      string
	FeedNames []string // Apply to these feeds only (empty = all feeds)
	Parse     func(string) string
}

var contentParsers = []ContentParser{
	{
		Name:      "Strip HackerNews Comments-Only Descriptions",
		FeedNames: []string{"Dev"},
		Parse: func(desc string) string {
			// HN items with no description have: <![CDATA[<a href="...">Comments</a>]]>
			// Strip CDATA wrapper
			desc = strings.TrimSpace(desc)
			desc = strings.TrimPrefix(desc, "<![CDATA[")
			desc = strings.TrimSuffix(desc, "]]>")
			desc = strings.TrimSpace(desc)

			// If it's just a link to HN comments with "Comments" text, strip it
			if strings.HasPrefix(desc, `<a href="https://news.ycombinator.com/item?id=`) &&
				strings.HasSuffix(desc, `">Comments</a>`) {
				return ""
			}

			// Also catch the plain text version
			if desc == "Comments" {
				return ""
			}

			return desc
		},
	},
	{
		Name: "Strip TechCrunch Copyright",
		Parse: func(desc string) string {
			return strings.Replace(desc, "© 2025 TechCrunch. All rights reserved. For personal use only.", "", -1)
		},
	},
	{
		Name: "Remove Images",
		Parse: func(desc string) string {
			return regexp.MustCompile(`<img .*>`).ReplaceAllString(desc, "")
		},
	},
	{
		Name: "Extract First Paragraph",
		Parse: func(desc string) string {
			parts := strings.Split(desc, "</p>")
			if len(parts) > 0 {
				return strings.Replace(parts[0], "<p>", "", 1)
			}
			return desc
		},
	},
	{
		Name: "Sanitize HTML",
		Parse: func(desc string) string {
			return sanitize.HTML(desc)
		},
	},
}

// applyContentParsers applies all relevant parsers to a description
func applyContentParsers(desc string, feedName string) string {
	for _, parser := range contentParsers {
		// If parser has specific feed names, check if current feed matches
		if len(parser.FeedNames) > 0 {
			matched := false
			for _, name := range parser.FeedNames {
				if name == feedName {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		// Apply the parser
		desc = parser.Parse(desc)
	}
	return desc
}

func saveHtml(head, content []byte) {
	if len(content) == 0 {
		return
	}
	searchForm := `<form id="news-search" action="/news" method="GET">
  <input id="news-query" name="query" placeholder="Search news">
  <button id="news-search-btn">Search</button>
</form>`
	body := fmt.Sprintf(`%s<div id="topics">%s</div><div>%s</div>`, searchForm, string(head), string(content))
	newsBodyHtml = body
	data.SaveFile("news.html", newsBodyHtml)
	app.Log("news", "Saved news.html (%d bytes)", len(newsBodyHtml))
}

// generateNewsHtml generates fresh HTML from the feed data with current timestamps
func generateNewsHtml() string {
	mutex.RLock()
	defer mutex.RUnlock()

	var content []byte
	var categories = make(map[string][]*Post)

	// Group posts by category
	for _, post := range feed {
		categories[post.Category] = append(categories[post.Category], post)
	}

	// Sort categories
	var sortedCategories []string
	for cat := range categories {
		sortedCategories = append(sortedCategories, cat)
	}
	sort.Strings(sortedCategories)

	// Generate HTML for each category
	for _, cat := range sortedCategories {
		posts := categories[cat]
		if len(posts) == 0 {
			continue
		}

		content = append(content, []byte(`<div class=section>`)...)
		content = append(content, []byte(`<hr id="`+cat+`" class="anchor">`)...)
		content = append(content, []byte(`<h1>`+cat+`</h1>`)...)

		for _, post := range posts {
			cleanDescription := strings.TrimSpace(post.Description)
			if len(cleanDescription) > 300 {
				cleanDescription = cleanDescription[:300] + "..."
			}

			link := post.URL
			if post.ID != "" {
				link = "/news?id=" + post.ID
			}

			var val string
			if len(post.Image) > 0 {
				categoryBadge := ""
				if post.Category != "" {
					categoryBadge = fmt.Sprintf(`<div class="category-header"><a href="/news#%s" class="category">%s</a></div>`, post.Category, post.Category)
				}
				val = fmt.Sprintf(`
	<div id="%s" class="news">
	    %s
	    <img class="cover" src="%s">
	    <div class="blurb">
	      <a href="%s"><span class="title">%s</span></a>
	      <span class="description">%s</span>
	    </div>
	  <div class="summary">%s</div>
				`, post.ID, categoryBadge, post.Image, link, post.Title, cleanDescription, getSummary(post))
			} else {
				categoryBadge := ""
				if post.Category != "" {
					categoryBadge = fmt.Sprintf(`<div class="category-header"><a href="/news#%s" class="category">%s</a></div>`, post.Category, post.Category)
				}
				val = fmt.Sprintf(`
	<div id="%s" class="news">
	    %s
	    <img class="cover">
	    <div class="blurb">
	      <a href="%s"><span class="title">%s</span></a>
	      <span class="description">%s</span>
	    </div>
	  <div class="summary">%s</div>
				`, post.ID, categoryBadge, link, post.Title, cleanDescription, getSummary(post))
			}

			val += `</div>`
			content = append(content, []byte(val)...)
		}

		content = append(content, []byte(`</div>`)...)
	}

	searchForm := `<form id="news-search" action="/news" method="GET">
  <input id="news-query" name="query" placeholder="Search news">
  <button id="news-search-btn">Search</button>
</form>`

	// Generate headlines
	headlines := generateHeadlinesHtml()

	// Get topics header
	var sortedFeeds []string
	for name := range feeds {
		sortedFeeds = append(sortedFeeds, name)
	}
	sort.Strings(sortedFeeds)
	head := app.Head("news", sortedFeeds)

	return fmt.Sprintf(`%s<div id="topics">%s</div><div>%s</div>`, searchForm, head, headlines+string(content))
}

// generateHeadlinesHtml generates fresh HTML for headlines with current timestamps
func generateHeadlinesHtml() string {
	mutex.RLock()
	defer mutex.RUnlock()

	// Get first post from each category for headlines
	seenCategories := make(map[string]bool)
	var headlines []*Post

	for _, post := range feed {
		if !seenCategories[post.Category] {
			headlines = append(headlines, post)
			seenCategories[post.Category] = true
		}
		if len(headlines) >= 10 {
			break
		}
	}

	// Sort by posted date
	sort.Slice(headlines, func(i, j int) bool {
		return headlines[i].PostedAt.After(headlines[j].PostedAt)
	})

	var headline []byte
	headline = append(headline, []byte(`<div class="headlines">`)...)

	for _, h := range headlines {
		link := h.URL
		if h.ID != "" {
			link = "/news?id=" + h.ID
		}

		categoryBadge := ""
		if h.Category != "" {
			categoryBadge = fmt.Sprintf(`<div class="category-header"><a href="/news#%s" class="category">%s</a></div>`, h.Category, h.Category)
		}
		summary := getSummary(h)

		val := fmt.Sprintf(`
		<div class="headline">
		   %s
		   <a href="%s"><span class="title">%s</span></a>
		 <span class="description">%s</span>
		 <div class="summary">%s</div>
		`, categoryBadge, link, h.Title, h.Description, summary)

		val += `</div>`
		headline = append(headline, []byte(val)...)
	}

	headline = append(headline, []byte(`</div>`)...)
	return string(headline)
}

func loadFeed() {
	// load the feeds file
	data, _ := f.ReadFile("feeds.json")
	// unpack into feeds
	mutex.Lock()
	if err := json.Unmarshal(data, &feeds); err != nil {
		fmt.Println("Error parsing feeds.json", err)
	}
	mutex.Unlock()
}

func getMetadataPath(uri string) string {
	// Generate stable ID from URL hash
	itemID := fmt.Sprintf("%x", md5.Sum([]byte(uri)))[:16]
	return filepath.Join("news", "metadata", itemID+".json")
}

func loadCachedMetadata(uri string) (*Metadata, bool) {
	path := getMetadataPath(uri)
	var md Metadata
	if err := data.LoadJSON(path, &md); err != nil {
		return nil, false
	}
	return &md, true
}

func saveCachedMetadata(uri string, md *Metadata) {
	path := getMetadataPath(uri)
	if err := data.SaveJSON(path, md); err != nil {
		app.Log("news", "Error saving metadata: %v", err)
	}
}

func backoff(attempts int) time.Duration {
	if attempts > 13 {
		return time.Hour
	}
	return time.Duration(math.Pow(float64(attempts), math.E)) * time.Millisecond * 100
}

func getMetadata(uri string, publishedAt time.Time) (*Metadata, bool, error) {
	// Check cache first
	if cached, exists := loadCachedMetadata(uri); exists {
		// For HN articles: refresh after 1 hour (for new comments)
		isHN := strings.Contains(uri, "news.ycombinator.com/item?id=")
		if isHN {
			age := time.Since(time.Unix(0, cached.Created))
			if age < time.Hour {
				// Request summary if we don't have one yet, with smart retry
				if cached.Summary == "" {
					shouldRetry := shouldRequestSummary(cached)
					if shouldRetry {
						go requestArticleSummary(uri, cached)
					}
				}
				return cached, false, nil // false = from cache
			}
			app.Log("news", "HN metadata cache expired for %s (age: %v), refetching comments", uri, age.Round(time.Minute))
		} else {
			// For regular articles: check if our cached metadata is older than the published date
			// This means the article was updated after we cached it
			cachedTime := time.Unix(0, cached.Created)
			if !publishedAt.IsZero() && cachedTime.Before(publishedAt) {
				app.Log("news", "Article updated after cache for %s (cached: %v, published: %v), refetching",
					uri, cachedTime.Format(time.RFC3339), publishedAt.Format(time.RFC3339))
			} else {
				// Cache is still valid
				// Request summary if we don't have one yet, with smart retry
				if cached.Summary == "" {
					shouldRetry := shouldRequestSummary(cached)
					if shouldRetry {
						go requestArticleSummary(uri, cached)
					}
				}
				return cached, false, nil
			}
		}
	}

	// Acquire semaphore to limit concurrent fetches (prevents memory spike)
	metadataFetchSem <- struct{}{}
	defer func() { <-metadataFetchSem }()

	u, err := url.Parse(uri)
	if err != nil {
		return nil, false, err
	}

	// Fetch HTML with proper resource cleanup
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	// Parse HTML
	d, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, false, err
	}

	g := &Metadata{
		Created: time.Now().UnixNano(),
	}

	check := func(p []string) bool {
		if p[0] == "twitter" {
			return true
		}
		if p[0] == "og" {
			return true
		}

		return false
	}

	for _, node := range d.Find("meta").Nodes {
		if len(node.Attr) < 2 {
			continue
		}

		p := strings.Split(node.Attr[0].Val, ":")
		if !check(p) {
			p = strings.Split(node.Attr[1].Val, ":")
			if !check(p) {
				continue
			}
			node.Attr = node.Attr[1:]
			if len(node.Attr) < 2 {
				continue
			}
		}

		switch p[1] {
		case "site_name":
			g.Site = node.Attr[1].Val
		case "site":
			if len(g.Site) == 0 {
				g.Site = node.Attr[1].Val
			}
		case "title":
			g.Title = node.Attr[1].Val
		case "description":
			g.Description = node.Attr[1].Val
		case "card", "type":
			g.Type = node.Attr[1].Val
		case "url":
			g.Url = node.Attr[1].Val
		case "image":
			if len(p) > 2 && p[2] == "src" {
				g.Image = node.Attr[1].Val
			} else if len(p) > 2 {
				// skip
				continue
			} else if len(g.Image) == 0 {
				g.Image = node.Attr[1].Val
			}

			// relative url needs fixing
			if len(g.Image) > 0 && g.Image[0] == '/' {
				g.Image = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, g.Image)
			}
		}
	}

	// attempt to get the content from article body
	var fn func(*nethtml.Node)

	fn = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			if len(node.Data) < 10 {
				return // Skip very short text nodes
			}

			first := node.Data[0]
			last := node.Data[len(node.Data)-1]

			data := sanitize.HTML(node.Data)

			if unicode.IsUpper(rune(first)) && last == '.' {
				g.Content += fmt.Sprintf(`<p>%s</p>`, data)
			} else if first == '"' && last == '"' {
				g.Content += fmt.Sprintf(`<p>%s</p>`, data)
			} else {
				g.Content += fmt.Sprintf(` %s`, data)
			}
		}

		if node.FirstChild != nil {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				fn(c)
			}
		}
	}

	// Extract content from common article selectors
	selectors := []string{
		".ArticleBody-articleBody", // CNBC
		"article",                  // Generic
		".article-body",            // Common
		".post-content",            // Common
		".entry-content",           // WordPress
		"[itemprop='articleBody']", // Schema.org
		".story-body",              // BBC-style
		"main article",             // Semantic HTML
		".content",                 // Generic content class
		"#content",                 // Generic content ID
	}

	contentExtracted := false
	for _, selector := range selectors {
		nodes := d.Find(selector).Nodes
		if len(nodes) > 0 {
			for _, node := range nodes {
				fn(node)
			}
			if len(g.Content) > 200 {
				contentExtracted = true
				break
			}
		}
	}

	// If no content extracted and it's not a HackerNews link, try to get paragraphs
	if !contentExtracted && !strings.Contains(u.Host, "news.ycombinator.com") {
		for _, node := range d.Find("p").Nodes {
			if node.Parent != nil {
				// Skip paragraphs in nav, footer, sidebar
				parent := node.Parent.Data
				if parent == "nav" || parent == "footer" || parent == "aside" {
					continue
				}
			}
			fn(node)
			if len(g.Content) > 2000 {
				break // Limit content extraction
			}
		}
	}
	//if len(g.Type) == 0 || len(g.Image) == 0 || len(g.Title) == 0 || len(g.Url) == 0 {
	//	fmt.Println("Not returning", u.String())
	//	return nil
	//}

	// Fetch discussion/comments based on source
	if strings.Contains(uri, "news.ycombinator.com/item?id=") {
		// Extract HackerNews story ID
		hnID := ""
		if idx := strings.Index(uri, "id="); idx != -1 {
			hnID = uri[idx+3:]
			if idx := strings.IndexAny(hnID, "&?#"); idx != -1 {
				hnID = hnID[:idx]
			}
		}
		if hnID != "" {
			comments, err := FetchHNComments(hnID)
			if err == nil && len(comments) > 0 {
				g.Comments = comments
				app.Log("news", "Fetched comments for HN story %s (%d chars)", hnID, len(comments))
			}
		}
	}
	// Future: Add other comment sources here (Reddit, forums, etc.)

	// Preserve existing summary if we already have one
	if existing, exists := loadCachedMetadata(uri); exists && existing.Summary != "" {
		g.Summary = existing.Summary
		g.SummaryRequestedAt = existing.SummaryRequestedAt
		g.SummaryAttempts = existing.SummaryAttempts
	}

	// Cache the metadata
	saveCachedMetadata(uri, g)

	// Request LLM summary generation via event (non-blocking) only if we don't have one
	if g.Summary == "" {
		go requestArticleSummary(uri, g)
	}

	return g, true, nil // true = freshly fetched
}

// shouldRequestSummary determines if we should retry requesting a summary
// Uses exponential backoff: 5min, 30min, 2hr, 6hr, 24hr, then stop
func shouldRequestSummary(md *Metadata) bool {
	// Never requested before
	if md.SummaryRequestedAt == 0 {
		return true
	}

	// Stop retrying after 5 attempts
	if md.SummaryAttempts >= 5 {
		return false
	}

	// Calculate backoff duration based on attempts
	var backoffDuration time.Duration
	switch md.SummaryAttempts {
	case 0:
		backoffDuration = 0 // First attempt, no delay
	case 1:
		backoffDuration = 5 * time.Minute
	case 2:
		backoffDuration = 30 * time.Minute
	case 3:
		backoffDuration = 2 * time.Hour
	case 4:
		backoffDuration = 6 * time.Hour
	default:
		backoffDuration = 24 * time.Hour
	}

	timeSinceLastRequest := time.Since(time.Unix(0, md.SummaryRequestedAt))
	return timeSinceLastRequest >= backoffDuration
}

// requestArticleSummary publishes a request for LLM summary generation
func requestArticleSummary(uri string, md *Metadata) {
	// Skip if we already have a summary
	if md.Summary != "" {
		return
	}

	// Prepare content for summarization
	contentToSummarize := md.Title
	if md.Description != "" {
		contentToSummarize += "\n\n" + md.Description
	}
	if md.Content != "" {
		// Limit content length to avoid overwhelming the LLM
		content := htmlToText(md.Content)
		if len(content) > 2000 {
			content = content[:2000]
		}
		contentToSummarize += "\n\n" + content
	}

	// Skip if there's not enough content
	if len(contentToSummarize) < 100 {
		return
	}

	// Update request tracking
	md.SummaryRequestedAt = time.Now().UnixNano()
	md.SummaryAttempts++
	saveCachedMetadata(uri, md)

	app.Log("news", "Requesting summary generation for %s (attempt %d)", uri, md.SummaryAttempts)

	// Publish summary generation request
	data.Publish(data.Event{
		Type: data.EventGenerateSummary,
		Data: map[string]interface{}{
			"uri":     uri,
			"content": contentToSummarize,
			"type":    "news",
		},
	})
}

// FetchHNComments fetches top-level comments from a HackerNews story
func FetchHNComments(storyID string) (string, error) {
	apiURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%s.json", storyID)

	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var story struct {
		Comments []int `json:"kids"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&story); err != nil {
		return "", err
	}

	// Fetch top 10 comments for context
	var comments []string
	maxComments := 10
	for i, commentID := range story.Comments {
		if i >= maxComments {
			break
		}

		commentURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", commentID)
		commentResp, err := http.Get(commentURL)
		if err != nil {
			continue
		}

		var comment struct {
			Text string `json:"text"`
			By   string `json:"by"`
		}

		if err := json.NewDecoder(commentResp.Body).Decode(&comment); err != nil {
			commentResp.Body.Close()
			continue
		}
		commentResp.Body.Close()

		if comment.Text != "" {
			// Strip HTML tags from comment
			cleanText := sanitize.HTML(comment.Text)
			comments = append(comments, fmt.Sprintf("[%s]: %s", comment.By, cleanText))
		}

		// Rate limit: small delay between requests
		time.Sleep(50 * time.Millisecond)
	}

	if len(comments) > 0 {
		return "Discussion: " + strings.Join(comments, " | "), nil
	}

	return "", nil
}

func getReminder() {
	fmt.Println("Getting Reminder at", time.Now().String())
	uri := "https://reminder.dev/api/latest"

	resp, err := http.Get(uri)
	if err != nil {
		app.Log("news", "Error getting reminder: %v", err)
		time.Sleep(time.Minute)

		go getReminder()
		return
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)

	var val map[string]interface{}

	err = json.Unmarshal(b, &val)
	if err != nil {
		app.Log("news", "Error getting reminder: %v", err)
		time.Sleep(time.Minute)

		go getReminder()
		return
	}

	link := fmt.Sprintf("https://reminder.dev%s", val["links"].(map[string]interface{})["verse"].(string))

	html := fmt.Sprintf(`<div class="item"><div class="verse">%s</div></div>`, val["verse"])
	html += app.Link("More", link)

	mutex.Lock()
	data.SaveFile("reminder.html", html)
	reminderHtml = html
	mutex.Unlock()

	// Index the daily card content for search/RAG
	// Extract all rich fields from the API response
	verse := val["verse"].(string)
	name := ""
	if v, ok := val["name"]; ok {
		name = v.(string)
	}
	hadith := ""
	if h, ok := val["hadith"]; ok {
		hadith = h.(string)
	}
	message := ""
	if m, ok := val["message"]; ok {
		message = m.(string)
	}
	updated := ""
	if u, ok := val["updated"]; ok {
		updated = u.(string)
	}

	// Combine all content for comprehensive indexing
	content := fmt.Sprintf("Name of Allah: %s\n\nVerse: %s\n\nHadith: %s\n\n%s", name, verse, hadith, message)

	data.Index(
		"reminder_card_daily",
		"reminder",
		"Daily Islamic Reminder",
		content,
		map[string]interface{}{
			"url":     link,
			"updated": updated,
			"source":  "card",
		},
	)

	time.Sleep(time.Hour)

	go getReminder()
}

// parseFeedItem processes a single RSS feed item and returns a Post
func parseFeedItem(item *gofeed.Item, categoryName string) (*Post, error) {
	// Apply content parsers to clean up description
	item.Description = applyContentParsers(item.Description, categoryName)

	link := item.Link
	app.Log("news", "Checking link %s", link)

	if strings.HasPrefix(link, "https://themwl.org/ar/") {
		link = strings.Replace(link, "themwl.org/ar/", "themwl.org/en/", 1)
		app.Log("news", "Replacing mwl ar link %s -> %s", item.Link, link)
	}

	// Parse publish timestamp
	postedAt := parsePublishTime(item)

	// Get metadata
	md, _, err := getMetadata(link, postedAt)
	if err != nil {
		return nil, fmt.Errorf("error parsing metadata: %w", err)
	}

	if strings.Contains(link, "themwl.org") {
		item.Title = md.Title
	}

	// Use extracted content if available
	if len(md.Content) > 0 && len(item.Content) == 0 {
		item.Content = md.Content
	}

	// Clean and truncate description
	cleanDescription := cleanAndTruncateDescription(item.Description)

	// Generate stable ID from URL hash
	itemID := fmt.Sprintf("%x", md5.Sum([]byte(link)))[:16]

	// Use summary as post content if available
	postContent := item.Content
	if md.Summary != "" {
		postContent = md.Summary
	}

	// Use metadata title if RSS title is empty
	itemTitle := item.Title
	if itemTitle == "" && md.Title != "" {
		itemTitle = md.Title
		app.Log("news", "Using metadata title for %s: %s", link, itemTitle)
	}

	post := &Post{
		ID:          itemID,
		Title:       itemTitle,
		Description: cleanDescription,
		URL:         link,
		Published:   item.Published,
		PostedAt:    postedAt,
		Category:    categoryName,
		Image:       md.Image,
		Content:     postContent,
	}

	// Index the article for search/RAG
	indexArticle(post, item, md)

	return post, nil
}

// parsePublishTime extracts and parses the publish time from a feed item
func parsePublishTime(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}

	if item.Published != "" {
		// Try common date formats
		formats := []string{time.RFC1123Z, time.RFC3339}
		for _, format := range formats {
			if parsed, err := time.Parse(format, item.Published); err == nil {
				return parsed
			}
		}
		app.Log("news", "Failed to parse timestamp for %s: %s - using current time", item.Link, item.Published)
	} else {
		app.Log("news", "No timestamp available for %s - using current time", item.Link)
	}

	return time.Now()
}

// cleanAndTruncateDescription cleans HTML and truncates description to first sentence
func cleanAndTruncateDescription(desc string) string {
	// Convert plain text newlines to em dashes
	if !strings.Contains(desc, "<") {
		desc = strings.ReplaceAll(desc, "\n", " — ")
		desc = strings.ReplaceAll(desc, "— —", "—")
		desc = strings.ReplaceAll(desc, "—  —", "—")
	}

	cleanDescription := htmlToText(desc)

	// Truncate to first sentence (max 250 chars)
	maxLen := 250
	if len(cleanDescription) > maxLen {
		truncated := cleanDescription[:maxLen]
		if idx := strings.Index(truncated, ". "); idx > 0 {
			return truncated[:idx+1]
		}
		if idx := strings.Index(truncated, ".\n"); idx > 0 {
			return truncated[:idx+1]
		}
		return truncated[:247] + "..."
	}

	// Look for first sentence break
	if idx := strings.Index(cleanDescription, ". "); idx > 0 {
		return cleanDescription[:idx+1]
	}
	if idx := strings.Index(cleanDescription, ".\n"); idx > 0 {
		return cleanDescription[:idx+1]
	}

	return cleanDescription
}

// indexArticle indexes an article for search/RAG
func indexArticle(post *Post, item *gofeed.Item, md *Metadata) {
	// Use LLM summary if available, otherwise combine description + content
	fullContent := item.Description + " " + item.Content
	if md.Summary != "" {
		fullContent = md.Summary
	}
	if len(md.Comments) > 0 {
		fullContent += " " + md.Comments
	}

	data.Index(
		post.ID,
		"news",
		post.Title,
		fullContent,
		map[string]interface{}{
			"url":         post.URL,
			"category":    post.Category,
			"posted_at":   post.PostedAt,
			"image":       post.Image,
			"description": item.Description,
			"summary":     md.Summary,
		},
	)
}

// formatFeedItemHTML formats a single feed item as HTML
func formatFeedItemHTML(post *Post, itemGUID string) string {
	categoryBadge := ""
	if post.Category != "" {
		categoryBadge = fmt.Sprintf(`<div class="category-header"><a href="/news#%s" class="category">%s</a></div>`, post.Category, post.Category)
	}
	summary := getSummary(post)

	if len(post.Image) > 0 {
		return fmt.Sprintf(`
	<div id="%s" class="news">
	  %s
	  <a href="%s" rel="noopener noreferrer" target="_blank">
	    <img class="cover" src="%s">
	    <div class="blurb">
	      <span class="title">%s</span>
	      <span class="description">%s</span>
	    </div>
	  </a>
	  <div class="summary">%s</div>
</div>`, itemGUID, categoryBadge, post.URL, post.Image, post.Title, post.Description, summary)
	}

	return fmt.Sprintf(`
	<div id="%s" class="news">
	  %s
	  <a href="%s" rel="noopener noreferrer" target="_blank">
	    <img class="cover">
	    <div class="blurb">
	      <span class="title">%s</span>
	      <span class="description">%s</span>
	    </div>
	  </a>
	  <div class="summary">%s</div>
</div>`, itemGUID, categoryBadge, post.URL, post.Title, post.Description, summary)
}

// processFeedCategory fetches and processes all items from a single feed category
func processFeedCategory(name, feedURL string, p *gofeed.Parser, stats map[string]Feed) ([]byte, []*Post, *Feed) {
	stat, ok := stats[name]
	if !ok {
		stat = Feed{Name: name, URL: feedURL}
		mutex.Lock()
		status[name] = &stat
		mutex.Unlock()
	}

	// Check if we should retry based on backoff
	if stat.Attempts > 0 && time.Until(stat.Backoff) > 0 {
		return nil, nil, &stat
	}

	if stat.Attempts > 0 {
		fmt.Println("Reattempting to pull", feedURL)
	}

	// Parse the feed
	f, err := p.ParseURL(feedURL)
	if err != nil {
		stat.Attempts++
		stat.Error = err
		stat.Backoff = time.Now().Add(backoff(stat.Attempts))
		fmt.Printf("Error parsing %s: %v, attempt %d backoff until %v\n", feedURL, err, stat.Attempts, stat.Backoff)
		mutex.Lock()
		status[name] = &stat
		mutex.Unlock()
		return nil, nil, &stat
	}

	// Successful pull - reset stats
	stat.Attempts = 0
	stat.Backoff = time.Time{}
	stat.Error = nil
	mutex.Lock()
	status[name] = &stat
	mutex.Unlock()

	// Build HTML and collect posts
	var content []byte
	var posts []*Post
	var firstPost *Post

	content = append(content, []byte(`<div class=section>`)...)
	content = append(content, []byte(`<hr id="`+name+`" class="anchor">`)...)
	content = append(content, []byte(`<h1>`+name+`</h1>`)...)

	for i, item := range f.Items {
		if i >= 10 {
			break
		}

		post, err := parseFeedItem(item, name)
		if err != nil {
			app.Log("news", "Error parsing item from %s: %v", feedURL, err)
			continue
		}

		posts = append(posts, post)
		itemHTML := formatFeedItemHTML(post, item.GUID)
		content = append(content, []byte(itemHTML)...)

		if i == 0 {
			firstPost = post
		}
	}

	content = append(content, []byte(`</div>`)...)

	var headlines []*Post
	if firstPost != nil {
		headlines = append(headlines, firstPost)
	}

	return content, headlines, &stat
}

// generateMarketsHTML creates the markets HTML from price data
func generateMarketsHTML(prices map[string]float64) string {
	var sb strings.Builder
	sb.WriteString(`<div class="item"><div id="tickers">`)

	for _, ticker := range tickers {
		price := prices[ticker]
		fmt.Fprintf(&sb, `<span class="ticker"><span class="highlight">%s</span>&nbsp;&nbsp;$%.2f</span>`, ticker, price)
	}

	sb.WriteString(`</div>`)

	sb.WriteString(`<div id="futures">`)
	for _, ticker := range futuresKeys {
		price := prices[ticker]
		fmt.Fprintf(&sb, `<span class="ticker"><span class="highlight">%s</span>&nbsp;&nbsp;$%.2f</span>`, ticker, price)
	}
	sb.WriteString(`</div></div>`)

	return sb.String()
}

// indexMarketPrices indexes all market prices for search/RAG
func indexMarketPrices(prices map[string]float64) {
	app.Log("news", "Indexing %d market prices", len(prices))
	timestamp := time.Now().Format(time.RFC3339)
	for ticker, price := range prices {
		data.Index(
			"market_"+ticker,
			"market",
			ticker,
			fmt.Sprintf("$%.2f", price),
			map[string]interface{}{
				"ticker":  ticker,
				"price":   price,
				"updated": timestamp,
			},
		)
	}
}

// generateHeadlinesHTML creates the headlines HTML section
func generateHeadlinesHTML(headlines []*Post) string {
	var sb strings.Builder
	sb.WriteString(`<div class=section>`)

	for _, h := range headlines {
		categoryBadge := ""
		if h.Category != "" {
			categoryBadge = fmt.Sprintf(`<div class="category-header"><a href="/news#%s" class="category">%s</a></div>`, h.Category, h.Category)
		}
		summary := getSummary(h)
		fmt.Fprintf(&sb, `
			<div class="headline">
			   %s
			  <a href="%s" rel="noopener noreferrer" target="_blank">
			   <span class="title">%s</span>
			  </a>
			 <span class="description">%s</span>
			 <div class="summary">%s</div>
			`, categoryBadge, h.URL, h.Title, h.Description, summary)
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

func parseFeed() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic in feed parser: %v\n", r)
			debug.PrintStack()
			fmt.Println("Relaunching feed parser in 1 minute")
			time.Sleep(time.Minute)
			go parseFeed()
		}
	}()

	fmt.Println("Parsing feed at", time.Now().String())
	p := gofeed.NewParser()
	p.UserAgent = "Mu/0.1"

	// Collect feed URLs and stats
	var sorted []string
	urls := map[string]string{}
	stats := map[string]Feed{}

	mutex.RLock()
	for name, url := range feeds {
		sorted = append(sorted, name)
		urls[name] = url
		if stat, ok := status[name]; ok {
			stats[name] = *stat
		}
	}
	mutex.RUnlock()

	sort.Strings(sorted)

	// Process all feeds
	var allContent []byte
	var allNews []*Post
	var allHeadlines []*Post

	for _, name := range sorted {
		feedURL := urls[name]
		content, headlines, _ := processFeedCategory(name, feedURL, p, stats)
		if content != nil {
			allContent = append(allContent, content...)
		}
		if headlines != nil {
			allHeadlines = append(allHeadlines, headlines...)
			allNews = append(allNews, headlines...)
		}
	}

	// Get crypto prices and generate markets HTML
	newPrices := getPrices()
	app.Log("news", "Finished getting prices")

	if newPrices != nil {
		mutex.Lock()
		cachedPrices = newPrices
		mutex.Unlock()

		marketsHtml = generateMarketsHTML(newPrices)
		indexMarketPrices(newPrices)
	}

	// Generate headlines HTML
	sort.Slice(allHeadlines, func(i, j int) bool {
		return allHeadlines[i].PostedAt.After(allHeadlines[j].PostedAt)
	})

	headlineHtml := generateHeadlinesHTML(allHeadlines)
	allContent = append([]byte(headlineHtml), allContent...)

	// Save everything
	head := []byte(app.Head("news", sorted))
	mutex.Lock()
	feed = allNews
	headlinesHtml = headlineHtml
	saveHtml(head, allContent)
	data.SaveFile("headlines.html", headlinesHtml)
	data.SaveFile("markets.html", marketsHtml)
	data.SaveJSON("prices.json", cachedPrices)
	data.SaveJSON("feed.json", feed)
	mutex.Unlock()

	// Wait an hour and go again
	time.Sleep(time.Hour)
	go parseFeed()
}

func Load() {
	// Subscribe to refresh events
	sub := data.Subscribe(data.EventRefreshHNComments)
	go func() {
		for event := range sub.Chan {
			if url, ok := event.Data["url"].(string); ok {
				app.Log("news", "Received refresh request for: %s", url)
				RefreshHNMetadata(url)
			}
		}
	}()

	// Subscribe to summary generation responses
	summarySub := data.Subscribe(data.EventSummaryGenerated)
	go func() {
		for event := range summarySub.Chan {
			uri, okUri := event.Data["uri"].(string)
			summary, okSummary := event.Data["summary"].(string)
			eventType, okType := event.Data["type"].(string)

			if okUri && okSummary && okType && eventType == "news" {
				app.Log("news", "Received generated summary for: %s", uri)

				// Load existing metadata
				md, exists := loadCachedMetadata(uri)
				if exists {
					// Update with summary
					md.Summary = summary
					md.Created = time.Now().UnixNano()
					saveCachedMetadata(uri, md)

					// Re-index with the new summary
					// Get the itemID from URI
					itemID := fmt.Sprintf("%x", md5.Sum([]byte(uri)))[:16]

					// Get existing index entry to preserve metadata
					existing := data.GetByID(itemID)
					metadata := map[string]interface{}{
						"url": uri,
					}
					title := md.Title
					if existing != nil {
						// Preserve existing metadata fields
						for k, v := range existing.Metadata {
							metadata[k] = v
						}
						// Preserve existing title if metadata title is empty
						// RSS feeds often have better titles than scraped metadata
						if title == "" && existing.Title != "" {
							title = existing.Title
							app.Log("news", "Preserving existing RSS title: '%s'", title)
						}
					}
					// Update metadata with summary
					metadata["summary"] = summary

					// Re-index with summary as content
					data.Index(
						itemID,
						"news",
						title,
						summary, // Use summary as content for chat context
						metadata,
					)

					app.Log("news", "Updated and re-indexed article with summary: %s", uri)
				}
			}
		}
	}()

	// load headlines
	b, _ := data.LoadFile("headlines.html")
	headlinesHtml = string(b)

	// load markets
	b, _ = data.LoadFile("markets.html")
	marketsHtml = string(b)

	// load cached prices
	b, _ = data.LoadFile("prices.json")
	if len(b) > 0 {
		var prices map[string]float64
		if err := json.Unmarshal(b, &prices); err == nil {
			mutex.Lock()
			cachedPrices = prices
			mutex.Unlock()
		}
	}

	// load cached feed
	b, _ = data.LoadFile("feed.json")
	if len(b) > 0 {
		var cachedFeed []*Post
		if err := json.Unmarshal(b, &cachedFeed); err == nil {
			mutex.Lock()
			feed = cachedFeed
			mutex.Unlock()
		}
	}

	b, _ = data.LoadFile("reminder.html")

	reminderHtml = string(b)

	// load news body and html
	b, _ = data.LoadFile("news.html")
	html = string(b)
	// Extract body content from saved HTML for serving
	// The newsBodyHtml will be rebuilt by parseFeed, but load from file for immediate serving
	if len(html) > 0 {
		// Parse out just the body content between the main content divs
		// For now just set newsBodyHtml from the full html - parseFeed will update it
		newsBodyHtml = html
	}

	// load the feeds
	loadFeed()

	go parseFeed()

	go getReminder()
}

func Headlines() string {
	// Generate fresh HTML with current timestamps
	return generateHeadlinesHtml()
}

func Markets() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return marketsHtml
}

func Reminder() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return reminderHtml
}

func formatSummary(text string) string {
	// Split by double newlines for paragraphs
	paragraphs := strings.Split(text, "\n\n")
	var formatted []string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// Check if it's a bullet point list (lines starting with -, *, or •)
		lines := strings.Split(para, "\n")
		isList := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "• ") {
				isList = true
				break
			}
		}

		if isList {
			// Format as HTML list
			formatted = append(formatted, "<ul style=\"margin: 10px 0; padding-left: 20px;\">")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				// Remove bullet markers
				trimmed = strings.TrimPrefix(trimmed, "- ")
				trimmed = strings.TrimPrefix(trimmed, "* ")
				trimmed = strings.TrimPrefix(trimmed, "• ")
				formatted = append(formatted, fmt.Sprintf("<li>%s</li>", trimmed))
			}
			formatted = append(formatted, "</ul>")
		} else {
			// Regular paragraph
			formatted = append(formatted, fmt.Sprintf("<p style=\"margin: 10px 0;\">%s</p>", para))
		}
	}

	return strings.Join(formatted, "")
}

func handleArticleView(w http.ResponseWriter, r *http.Request, articleID string) {
	// Get article from index
	entry := data.GetByID(articleID)
	if entry == nil {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// Extract metadata
	url := ""
	category := ""
	image := ""
	summary := ""
	description := ""
	var postedAt time.Time

	if v, ok := entry.Metadata["url"].(string); ok {
		url = v
	}
	if v, ok := entry.Metadata["category"].(string); ok {
		category = v
	}
	if v, ok := entry.Metadata["image"].(string); ok {
		image = v
	}
	if v, ok := entry.Metadata["description"].(string); ok {
		description = v
	}
	if v, ok := entry.Metadata["summary"].(string); ok {
		summary = v
	}
	if v, ok := entry.Metadata["posted_at"].(time.Time); ok {
		postedAt = v
	}

	title := entry.Title

	// Debug logging
	app.Log("news", "Article view: ID=%s, Title='%s', URL='%s'", articleID, title, url)

	// If title or description is empty, try to fetch fresh metadata
	// But only use metadata values if they're actually better than what we have
	if (title == "" || description == "") && url != "" {
		app.Log("news", "Fetching metadata because title='%s' desc='%s'", title, description)
		md, _, err := getMetadata(url, postedAt)
		if err == nil {
			app.Log("news", "Got metadata: Title='%s', Desc='%s'", md.Title, md.Description)
			// Only use metadata title if our current title is empty AND metadata has one
			if title == "" && md.Title != "" {
				title = md.Title
			}
			// Only use metadata description if our current description is empty AND metadata has one
			if description == "" && md.Description != "" {
				description = md.Description
			}
			// Always use metadata image and summary if available (these are enhancements)
			if image == "" && md.Image != "" {
				image = md.Image
			}
			if summary == "" && md.Summary != "" {
				summary = md.Summary
			}
		} else {
			app.Log("news", "Error fetching metadata: %v", err)
		}
	}

	app.Log("news", "Final title='%s', desc='%s'", title, description)

	// Use description from metadata if available, otherwise fall back to indexed content
	if description == "" {
		description = htmlToText(entry.Content)
	}

	// Build the article page
	imageSection := ""
	if image != "" {
		imageSection = fmt.Sprintf(`<img src="%s" class="article-image">`, image)
	}

	summarySection := ""
	if summary != "" {
		// Format the summary: split by double newlines into paragraphs, handle bullet points
		formattedSummary := formatSummary(summary)
		summarySection = fmt.Sprintf(`
			<div class="article-summary">
				<h3>Summary</h3>
				<div>%s</div>
			</div>`, formattedSummary)
	}

	categoryBadge := ""
	if category != "" {
		categoryBadge = fmt.Sprintf(` · <a href="/news#%s" class="category">%s</a>`, category, category)
	}

	// Build description section
	descriptionSection := ""
	if description != "" {
		descriptionSection = fmt.Sprintf(`<div class="article-description"><p>%s</p></div>`, description)
	}

	articleHtml := fmt.Sprintf(`
		<div id="news-article">
			%s
			<h1>%s</h1>
			<div class="article-meta">
				<span>%s · Source: <i>%s</i>%s</span>
			</div>
			%s
			%s
			<div class="article-actions">
				<a href="%s" target="_blank" rel="noopener noreferrer">Read Original →</a>
				<span style="margin: 0 8px;">·</span>
				<a href="/chat?id=news_%s">Discuss with AI →</a>
			</div>
			<div class="article-back">
				<a href="/news">← Back to news</a>
			</div>
		</div>
	`, imageSection, title, app.TimeAgo(postedAt), getDomain(url), categoryBadge, descriptionSection, summarySection, url, articleID)

	w.Write([]byte(app.RenderHTML("", "", articleHtml)))
}

func Handler(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")

	// Handle viewing individual news article
	if articleID := r.URL.Query().Get("id"); articleID != "" {
		handleArticleView(w, r, articleID)
		return
	}

	// Handle POST with JSON (API endpoint)
	if r.Method == "POST" && ct == "application/json" {
		// Require authentication for search
		if _, err := auth.GetSession(r); err != nil {
			http.Error(w, "Authentication required to search", http.StatusUnauthorized)
			return
		}

		var reqData map[string]interface{}
		b, _ := ioutil.ReadAll(r.Body)
		json.Unmarshal(b, &reqData)

		query := ""
		if v := reqData["query"]; v != nil {
			query = fmt.Sprintf("%v", v)
		}

		if query == "" {
			http.Error(w, "query required", 400)
			return
		}

		// Search indexed news articles with type filter
		results := data.Search(query, 20, data.WithType("news"))

		// Format results for JSON response
		var articles []map[string]interface{}
		for _, entry := range results {
			article := map[string]interface{}{
				"id":          entry.ID,
				"title":       entry.Title,
				"description": htmlToText(entry.Content),
				"url":         entry.Metadata["url"],
				"category":    entry.Metadata["category"],
				"image":       entry.Metadata["image"],
				"posted_at":   entry.Metadata["posted_at"],
			}
			articles = append(articles, article)
		}

		resp := map[string]interface{}{
			"query":   query,
			"results": articles,
			"count":   len(articles),
		}

		b, _ = json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	// Handle search query
	query := r.URL.Query().Get("query")
	if query != "" {
		// Require authentication for search
		if _, err := auth.GetSession(r); err != nil {
			http.Error(w, "Authentication required to search", http.StatusUnauthorized)
			return
		}

		// Limit query length to prevent abuse
		if len(query) > 256 {
			http.Error(w, "Search query must not exceed 256 characters", http.StatusBadRequest)
			return
		}
		handleSearch(w, r, query)
		return
	}

	mutex.RLock()
	hasContent := len(feed) > 0
	mutex.RUnlock()

	if accept := r.Header.Get("Accept"); accept == "application/json" {
		mutex.RLock()
		resp := map[string]interface{}{
			"feed": feed,
		}
		mutex.RUnlock()
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	// Generate HTML on-demand with fresh timestamps
	body := newsBodyHtml // fallback to cached for initial load
	if hasContent {
		body = generateNewsHtml()
	}
	renderedHtml := app.RenderHTMLForRequest("News", "Latest news headlines", body, r)
	w.Write([]byte(renderedHtml))
}

// formatSearchResult formats a single search result entry as HTML
func formatSearchResult(entry *data.IndexEntry) string {
	title := entry.Title
	description := htmlToText(entry.Content)
	if len(description) > 300 {
		description = description[:300] + "..."
	}

	// Extract metadata
	url, _ := entry.Metadata["url"].(string)
	category, _ := entry.Metadata["category"].(string)
	image, _ := entry.Metadata["image"].(string)
	
	var postedAt time.Time
	if v, ok := entry.Metadata["posted_at"].(time.Time); ok {
		postedAt = v
	} else if v, ok := entry.Metadata["posted_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			postedAt = parsed
		}
	}

	// Create a Post struct to use getSummary()
	post := &Post{
		ID:       entry.ID,
		Title:    title,
		URL:      url,
		Category: category,
		PostedAt: postedAt,
	}
	summary := getSummary(post)

	categoryBadge := ""
	if category != "" {
		categoryBadge = fmt.Sprintf(`<div class="category-header"><span class="category">%s</span></div>`, category)
	}

	if image != "" {
		return fmt.Sprintf(`
<div id="%s" class="news">
  <a href="%s" rel="noopener noreferrer" target="_blank">
    <img class="cover" src="%s">
    <div class="blurb">
      %s
      <span class="title">%s</span>
      <span class="description">%s</span>
    </div>
  </a>
  <div class="summary">%s</div>
</div>`, entry.ID, url, image, categoryBadge, title, description, summary)
	}

	return fmt.Sprintf(`
<div id="%s" class="news">
  <a href="%s" rel="noopener noreferrer" target="_blank">
    <img class="cover">
    <div class="blurb">
      %s
      <span class="title">%s</span>
      <span class="description">%s</span>
    </div>
  </a>
  <div class="summary">%s</div>
</div>`, entry.ID, url, categoryBadge, title, description, summary)
}

func handleSearch(w http.ResponseWriter, r *http.Request, query string) {
	results := data.Search(query, 20, data.WithType("news"))

	var searchResults []byte
	searchResults = append(searchResults, []byte(`<form id="news-search" action="/news" method="GET">
  <input name="query" value="`+query+`" placeholder="Search news">
  <button>Search</button>
  <a href="/news" style="margin-left: 10px; color: #666; text-decoration: none;">Clear</a>
</form>`)...)

	if len(results) == 0 {
		searchResults = append(searchResults, []byte("<p>No results found</p>")...)
	} else {
		searchResults = append(searchResults, []byte("<h2>Results</h2>")...)
		for _, entry := range results {
			searchResults = append(searchResults, []byte(formatSearchResult(entry))...)
		}
	}

	html := app.RenderHTMLForRequest("News", query, string(searchResults), r)
	w.Write([]byte(html))
}

// GetAllPrices returns all cached prices
func GetAllPrices() map[string]float64 {
	mutex.RLock()
	defer mutex.RUnlock()

	// Return a copy to avoid concurrent map access
	prices := make(map[string]float64)
	if cachedPrices != nil {
		for k, v := range cachedPrices {
			prices[k] = v
		}
	}
	return prices
}

// GetHomepageTickers returns the list of tickers displayed on homepage
func GetHomepageTickers() []string {
	return append([]string{}, tickers...)
}

// GetHomepageFutures returns the list of futures displayed on homepage
func GetHomepageFutures() []string {
	return append([]string{}, futuresKeys...)
}

// RefreshHNMetadata forces a refresh of HN article metadata with fresh comments
// Returns the updated metadata with new comments, or nil if not an HN article
func RefreshHNMetadata(uri string) (*Metadata, error) {
	if !strings.Contains(uri, "news.ycombinator.com/item?id=") {
		return nil, fmt.Errorf("not a HackerNews article")
	}

	// Extract HN story ID
	hnID := ""
	if idx := strings.Index(uri, "id="); idx != -1 {
		hnID = uri[idx+3:]
		if idx := strings.IndexAny(hnID, "&?#"); idx != -1 {
			hnID = hnID[:idx]
		}
	}

	if hnID == "" {
		return nil, fmt.Errorf("could not extract HN story ID from URL")
	}

	// Fetch fresh comments
	comments, err := FetchHNComments(hnID)
	if err != nil {
		app.Log("news", "Error fetching fresh HN comments: %v", err)
		// Don't fail the whole request, just return without comments
	}

	// Load cached metadata
	md, exists := loadCachedMetadata(uri)
	if !exists {
		// If no cache, fetch full metadata (use zero time since we don't have publish date here)
		md, _, err := getMetadata(uri, time.Time{})
		return md, err
	}

	// Update comments and timestamp
	md.Comments = comments
	md.Created = time.Now().UnixNano()

	// Save updated metadata
	saveCachedMetadata(uri, md)

	// Reindex with fresh comments so RAG can find them
	if exists {
		fullContent := md.Description + " " + md.Content
		if len(comments) > 0 {
			fullContent += " " + comments
		}
		data.Index(
			fmt.Sprintf("%x", md5.Sum([]byte(uri)))[:16],
			"news",
			md.Title,
			fullContent,
			map[string]interface{}{
				"url":      uri,
				"category": "Dev",
			},
		)
		app.Log("news", "Reindexed HN article with fresh comments for RAG: %s", uri)
	}

	app.Log("news", "Refreshed HN metadata for %s with %d chars of comments", uri, len(comments))
	return md, nil
}
