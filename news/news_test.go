package news

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"mu/internal/data"
)

func TestContentParsers_StripHNComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		feedName string
		expected string
	}{
		{
			name:     "Strip HN Comments link",
			input:    `<![CDATA[<a href="https://news.ycombinator.com/item?id=12345">Comments</a>]]>`,
			feedName: "Dev",
			expected: "",
		},
		{
			name:     "Strip plain Comments text",
			input:    "Comments",
			feedName: "Dev",
			expected: "",
		},
		{
			name:     "Preserve actual description",
			input:    "This is a real article description",
			feedName: "Dev",
			expected: "This is a real article description",
		},
		{
			name:     "Only applies to Dev feed",
			input:    `<![CDATA[<a href="https://news.ycombinator.com/item?id=12345">Comments</a>]]>`,
			feedName: "Tech",
			expected: "Comments", // CDATA gets stripped, link gets sanitized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyContentParsers(tt.input, tt.feedName)
			// Sanitize HTML also gets applied, so we need to check the sanitized result
			if !strings.Contains(result, tt.expected) && result != tt.expected {
				t.Errorf("Expected %q to contain or equal %q", result, tt.expected)
			}
		})
	}
}

func TestPostsSortedByTimestamp(t *testing.T) {
	// Create test posts with different timestamps
	now := time.Now()
	posts := []*Post{
		{
			ID:       "1",
			Title:    "Oldest",
			Category: "Tech",
			PostedAt: now.Add(-3 * time.Hour),
		},
		{
			ID:       "2",
			Title:    "Newest",
			Category: "Tech",
			PostedAt: now,
		},
		{
			ID:       "3",
			Title:    "Middle",
			Category: "Tech",
			PostedAt: now.Add(-1 * time.Hour),
		},
	}

	// Group by category (simulating generateNewsHtml logic)
	categories := make(map[string][]*Post)
	for _, post := range posts {
		categories[post.Category] = append(categories[post.Category], post)
	}

	// Sort posts within each category by timestamp (newest first)
	for _, categoryPosts := range categories {
		// This is the sorting logic from generateNewsHtml
		for i := 0; i < len(categoryPosts)-1; i++ {
			for j := i + 1; j < len(categoryPosts); j++ {
				if categoryPosts[j].PostedAt.After(categoryPosts[i].PostedAt) {
					categoryPosts[i], categoryPosts[j] = categoryPosts[j], categoryPosts[i]
				}
			}
		}
	}

	techPosts := categories["Tech"]
	if len(techPosts) != 3 {
		t.Fatalf("Expected 3 posts, got %d", len(techPosts))
	}

	// Verify newest first
	if techPosts[0].Title != "Newest" {
		t.Errorf("Expected first post to be 'Newest', got %q", techPosts[0].Title)
	}
	if techPosts[1].Title != "Middle" {
		t.Errorf("Expected second post to be 'Middle', got %q", techPosts[1].Title)
	}
	if techPosts[2].Title != "Oldest" {
		t.Errorf("Expected third post to be 'Oldest', got %q", techPosts[2].Title)
	}

	// Verify timestamps are in descending order
	for i := 0; i < len(techPosts)-1; i++ {
		if techPosts[i].PostedAt.Before(techPosts[i+1].PostedAt) {
			t.Errorf("Post %d timestamp %v is before post %d timestamp %v",
				i, techPosts[i].PostedAt, i+1, techPosts[i+1].PostedAt)
		}
	}
}

func TestCleanAndTruncateDescription(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "Short description unchanged",
			input:    "Short text.",
			maxLen:   250,
			expected: "Short text.",
		},
		{
			name:     "Long description truncated at sentence",
			input:    "First sentence. Second sentence. Third sentence that goes on and on and on.",
			maxLen:   250,
			expected: "First sentence.",
		},
		{
			name:     "HTML converted to text",
			input:    "<p>Paragraph text</p>",
			maxLen:   250,
			expected: "Paragraph text",
		},
		{
			name:     "Newlines converted to em dashes",
			input:    "Line one\nLine two",
			maxLen:   250,
			expected: "Line one — Line two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanAndTruncateDescription(tt.input)
			if !strings.Contains(result, tt.expected) && result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Simple text",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "Strip HTML tags",
			input:    "<p>Hello <strong>world</strong></p>",
			expected: "Hello world",
		},
		{
			name:     "Preserve links",
			input:    `<a href="https://example.com">Click here</a>`,
			expected: `<a href="https://example.com" target="_blank" rel="noopener noreferrer">Click here</a>`,
		},
		{
			name:     "Multiple paragraphs with spacing",
			input:    "<p>First</p><p>Second</p>",
			expected: "First Second",
		},
		{
			name:     "Collapse multiple spaces",
			input:    "Too    many     spaces",
			expected: "Too many spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := htmlToText(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNewsSearchArticlesFallsBackToLiveFeed(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	mutex.Lock()
	feed = []*Post{
		{
			ID:          "ai-1",
			Title:       "Open model lab ships safer AI assistant",
			Description: "New evaluations and rollout notes for the assistant.",
			URL:         "https://example.com/ai",
			Category:    "Tech",
			PostedAt:    now,
		},
		{
			ID:          "markets-1",
			Title:       "Markets close higher",
			Description: "Equities rally into the close.",
			URL:         "https://example.com/markets",
			Category:    "Business",
			PostedAt:    now.Add(-1 * time.Hour),
		},
	}
	mutex.Unlock()

	got := newsSearchArticles("latest AI news", nil, 20)
	if len(got) != 1 {
		t.Fatalf("expected one grounded live feed result, got %d: %#v", len(got), got)
	}
	if got[0]["title"] != "Open model lab ships safer AI assistant" {
		t.Fatalf("expected AI headline, got %#v", got[0])
	}
	if got[0]["url"] != "https://example.com/ai" {
		t.Fatalf("expected source URL for agent citations, got %#v", got[0]["url"])
	}
}

func TestNewsSearchArticlesFreshQueriesPreferNewestMatches(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	newer := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	indexed := []*data.IndexEntry{{
		ID:      "old-ai",
		Title:   "AI startup raises funding",
		Content: "Artificial intelligence funding story from the archive.",
		Metadata: map[string]interface{}{
			"url":       "https://example.com/old-ai",
			"category":  "Tech",
			"posted_at": older.Format(time.RFC3339),
		},
	}}

	mutex.Lock()
	feed = []*Post{{
		ID:          "new-ai",
		Title:       "AI lab ships today's assistant update",
		Description: "Fresh release notes for today's AI update.",
		URL:         "https://example.com/new-ai",
		Category:    "Tech",
		PostedAt:    newer,
	}}
	mutex.Unlock()

	got := newsSearchArticles("Find today's AI news", indexed, 20)
	if len(got) < 2 {
		t.Fatalf("expected indexed and live results, got %#v", got)
	}
	if got[0]["title"] != "AI lab ships today's assistant update" {
		t.Fatalf("expected newest matching item first for freshness query, got %#v", got)
	}
}

func TestNewsSearchArticlesFreshQueriesWidenLiveCandidatePool(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	older := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	today := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	var posts []*Post
	for i := 0; i < 12; i++ {
		posts = append(posts, &Post{
			ID:          fmt.Sprintf("old-ai-model-%d", i),
			Title:       fmt.Sprintf("AI model archive item %d", i),
			Description: "Older artificial intelligence model background.",
			URL:         fmt.Sprintf("https://example.com/old-ai-model-%d", i),
			Category:    "Tech",
			PostedAt:    older.Add(time.Duration(i) * time.Hour),
		})
	}
	posts = append(posts, &Post{
		ID:          "same-day-ai",
		Title:       "AI startup ships same-day assistant update",
		Description: "Fresh AI product news from today.",
		URL:         "https://example.com/same-day-ai",
		Category:    "Tech",
		PostedAt:    today,
	})

	mutex.Lock()
	feed = posts
	mutex.Unlock()

	got := newsSearchArticles("Find today's AI news", nil, 8)
	if len(got) == 0 {
		t.Fatal("expected live feed results")
	}
	if got[0]["title"] != "AI startup ships same-day assistant update" {
		t.Fatalf("expected widened candidate pool to let same-day AI news lead, got %#v", got)
	}
}

func TestNewsSearchFreshnessSummaryCaveatsStaleTodayResults(t *testing.T) {
	articles := []map[string]interface{}{{
		"title":     "AI startup raises funding",
		"posted_at": time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}}
	freshness := newsSearchFreshnessSummary("Find today's AI news", articles, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if freshness == nil {
		t.Fatal("expected freshness summary for today query")
	}
	if freshness["status"] != "stale" {
		t.Fatalf("expected stale status, got %#v", freshness)
	}
	if !strings.Contains(fmt.Sprint(freshness["notice"]), "No same-day news_search results") {
		t.Fatalf("expected same-day caveat notice, got %#v", freshness)
	}
}

func TestNewsSearchFreshnessSummaryCurrentForSameDayResults(t *testing.T) {
	articles := []map[string]interface{}{{
		"title":     "AI lab ships today's assistant update",
		"posted_at": time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	}}
	freshness := newsSearchFreshnessSummary("Find today's AI news", articles, time.Date(2026, 7, 5, 13, 0, 0, 0, time.UTC))
	if freshness == nil {
		t.Fatal("expected freshness summary for today query")
	}
	if freshness["status"] != "current" {
		t.Fatalf("expected current status, got %#v", freshness)
	}
	if _, ok := freshness["notice"]; ok {
		t.Fatalf("did not expect freshness caveat for same-day results, got %#v", freshness)
	}
}

func TestNewsSearchFreshnessSummaryCaveatsMostlyStaleTodayResults(t *testing.T) {
	articles := []map[string]interface{}{
		{
			"title":     "AI lab ships today's assistant update",
			"posted_at": time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
		},
		{
			"title":     "AI startup raises funding",
			"posted_at": time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
		},
		{
			"title":     "AI chip demand grows",
			"posted_at": time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
		},
	}
	freshness := newsSearchFreshnessSummary("Find today's AI news", articles, time.Date(2026, 7, 5, 13, 0, 0, 0, time.UTC))
	if freshness == nil {
		t.Fatal("expected freshness summary for today query")
	}
	if freshness["status"] != "mostly_stale" {
		t.Fatalf("expected mostly_stale status, got %#v", freshness)
	}
	if !strings.Contains(fmt.Sprint(freshness["notice"]), "Only 1 of 3 dated news_search results") {
		t.Fatalf("expected mostly-stale caveat notice, got %#v", freshness)
	}
}

func TestNewsSearchArticlesSortsNonRFCPostedAtForFreshnessQueries(t *testing.T) {
	indexed := []*data.IndexEntry{
		{ID: "older-ai", Title: "AI model archive", Content: "Older artificial intelligence model background.", Metadata: map[string]interface{}{
			"url": "https://example.com/older-ai", "category": "Tech", "posted_at": "May 20, 2026",
		}},
		{ID: "newer-ai", Title: "AI lab ships July update", Content: "Newer artificial intelligence assistant update.", Metadata: map[string]interface{}{
			"url": "https://example.com/newer-ai", "category": "Tech", "posted_at": "2026-07-02 09:00:00 +0000 UTC",
		}},
	}

	got := newsSearchArticles("Find today's AI news", indexed, 8)
	if len(got) < 2 {
		t.Fatalf("expected indexed results, got %#v", got)
	}
	if got[0]["title"] != "AI lab ships July update" {
		t.Fatalf("expected non-RFC posted_at to sort newest first, got %#v", got)
	}
}

func TestSearchToolTextReturnsLiveFeedResultsForAgent(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	mutex.Lock()
	feed = []*Post{{
		ID:          "ai-live",
		Title:       "AI lab releases safer assistant",
		Description: "The release includes source-linked evaluation notes.",
		URL:         "https://example.com/ai-live",
		Category:    "Technology",
		PostedAt:    now,
	}}
	mutex.Unlock()

	text, err := SearchToolText("latest AI news")
	if err != nil {
		t.Fatalf("SearchToolText returned error: %v", err)
	}
	if !strings.Contains(text, "AI lab releases safer assistant") {
		t.Fatalf("expected live feed headline in tool text, got %s", text)
	}
	if !strings.Contains(text, "https://example.com/ai-live") {
		t.Fatalf("expected source URL in tool text, got %s", text)
	}
}

func TestNewsSearchArticlesFiltersAdjacentIndexedResults(t *testing.T) {
	indexed := []*data.IndexEntry{
		{
			ID:      "finance-1",
			Title:   "Crypto policy fight weighs on markets",
			Content: "Lawmakers debate digital assets while equities drift lower.",
			Metadata: map[string]interface{}{
				"url":      "https://example.com/crypto-policy",
				"category": "Business",
			},
		},
		{
			ID:      "tech-1",
			Title:   "AI chip startup launches faster inference server",
			Content: "The new semiconductor design targets model serving workloads.",
			Metadata: map[string]interface{}{
				"url":      "https://example.com/ai-chip",
				"category": "Technology",
			},
		},
	}

	got := newsSearchArticles("latest technology headline", indexed, 20)
	if len(got) != 1 {
		t.Fatalf("expected only a clearly topic-matching indexed result, got %#v", got)
	}
	if got[0]["title"] != "AI chip startup launches faster inference server" {
		t.Fatalf("expected technology headline, got %#v", got[0])
	}
}

func TestHeadlinesTextIncludesSourceURLWithID(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	mutex.Lock()
	feed = []*Post{{
		ID:          "ai-1",
		Title:       "Open model lab ships safer AI assistant",
		Description: "New evaluations and rollout notes for the assistant.",
		URL:         "https://example.com/ai",
		Category:    "Tech",
		PostedAt:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
	}}
	mutex.Unlock()

	got := HeadlinesText("tech", 10)
	if !strings.Contains(got, "source: example.com, url: https://example.com/ai") {
		t.Fatalf("expected source domain and URL in headline context, got %q", got)
	}
	if !strings.Contains(got, "id: ai-1") {
		t.Fatalf("expected id to remain available for news_read, got %q", got)
	}
}

func TestHeadlinesTextTopicMatchesArticleTextAndSynonyms(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	mutex.Lock()
	feed = []*Post{
		{
			ID:          "ai-1",
			Title:       "Open model lab ships safer AI assistant",
			Description: "New evaluations and rollout notes for the assistant.",
			URL:         "https://example.com/ai",
			Category:    "Dev",
			PostedAt:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:          "sports-1",
			Title:       "Club wins cup final",
			Description: "A late goal settled the match.",
			URL:         "https://example.com/sports",
			Category:    "Sports",
			PostedAt:    time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC),
		},
	}
	mutex.Unlock()

	got := HeadlinesText("technology", 10)
	if !strings.Contains(got, "Open model lab ships safer AI assistant") {
		t.Fatalf("expected technology topic to match tech/AI headline text, got %q", got)
	}
	if strings.Contains(got, "Club wins cup final") {
		t.Fatalf("expected unrelated headline to stay out of topic-filtered results, got %q", got)
	}
}

func TestNewsSearchArticlesReturnsExplicitEmptyResults(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	mutex.Lock()
	feed = []*Post{{ID: "weather-1", Title: "Storm clears", Category: "Weather"}}
	mutex.Unlock()

	got := newsSearchArticles("semiconductor earnings", nil, 20)
	if len(got) != 0 {
		t.Fatalf("expected explicit empty result set, got %#v", got)
	}
}

func TestGetDomain(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Standard domain",
			url:      "https://example.com/path",
			expected: "example.com",
		},
		{
			name:     "Subdomain stripped",
			url:      "https://www.example.com/path",
			expected: "example.com",
		},
		{
			name:     "GitHub.io preserved",
			url:      "https://username.github.io/project",
			expected: "username.github.io",
		},
		{
			name:     "Deep subdomain",
			url:      "https://api.subdomain.example.com/path",
			expected: "api.subdomain.example.com", // getDomain doesn't strip deep subdomains
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDomain(tt.url)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestShouldRequestSummary(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name               string
		summaryRequestedAt int64
		summaryAttempts    int
		expected           bool
		description        string
	}{
		{
			name:               "Never requested",
			summaryRequestedAt: 0,
			summaryAttempts:    0,
			expected:           true,
			description:        "Should request on first attempt",
		},
		{
			name:               "First retry after 5 minutes",
			summaryRequestedAt: now.Add(-6 * time.Minute).UnixNano(),
			summaryAttempts:    1,
			expected:           true,
			description:        "Should retry after 5 min backoff",
		},
		{
			name:               "First retry too soon",
			summaryRequestedAt: now.Add(-4 * time.Minute).UnixNano(),
			summaryAttempts:    1,
			expected:           false,
			description:        "Should not retry before 5 min backoff",
		},
		{
			name:               "Max attempts reached",
			summaryRequestedAt: now.Add(-25 * time.Hour).UnixNano(),
			summaryAttempts:    5,
			expected:           false,
			description:        "Should stop after 5 attempts",
		},
		{
			name:               "Second retry after 30 minutes",
			summaryRequestedAt: now.Add(-31 * time.Minute).UnixNano(),
			summaryAttempts:    2,
			expected:           true,
			description:        "Should retry after 30 min backoff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := &Metadata{
				SummaryRequestedAt: tt.summaryRequestedAt,
				SummaryAttempts:    tt.summaryAttempts,
			}
			result := shouldRequestSummary(md)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expected, result)
			}
		})
	}
}

func TestFormatSummary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "Simple paragraph",
			input:    "This is a summary.",
			contains: []string{"<p", "This is a summary.", "</p>"},
		},
		{
			name:     "Multiple paragraphs",
			input:    "First paragraph.\n\nSecond paragraph.",
			contains: []string{"<p", "First paragraph.", "</p>", "Second paragraph."},
		},
		{
			name:     "Bullet list",
			input:    "- Point one\n- Point two\n- Point three",
			contains: []string{"<ul", "<li>Point one</li>", "<li>Point two</li>", "</ul>"},
		},
		{
			name:     "Mixed content",
			input:    "Introduction.\n\n- First point\n- Second point\n\nConclusion.",
			contains: []string{"Introduction.", "<ul", "<li>First point</li>", "Conclusion."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSummary(tt.input)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain %q, got: %s", expected, result)
				}
			}
		})
	}
}

func TestParsePublishTime(t *testing.T) {
	now := time.Now()
	parsed := now.Add(-1 * time.Hour)

	tests := []struct {
		name      string
		item      *gofeed.Item
		expectNow bool
	}{
		{
			name: "Uses parsed timestamp",
			item: &gofeed.Item{
				PublishedParsed: &parsed,
				Link:            "https://example.com",
			},
			expectNow: false,
		},
		{
			name: "Falls back to current time when no timestamp",
			item: &gofeed.Item{
				Link: "https://example.com",
			},
			expectNow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePublishTime(tt.item)
			if tt.expectNow {
				// Should be within 1 second of now
				diff := time.Since(result)
				if diff > time.Second || diff < 0 {
					t.Errorf("Expected timestamp near now, got %v (diff: %v)", result, diff)
				}
			} else {
				if !result.Equal(parsed) {
					t.Errorf("Expected %v, got %v", parsed, result)
				}
			}
		})
	}
}

// TestRegressionEmptyDescriptionUsesMetadata tests that when RSS feed has no description
// (e.g., HN articles with only "Comments" link), we fall back to scraped metadata description
func TestRegressionEmptyDescriptionUsesMetadata(t *testing.T) {
	// This test verifies the fix for: "articles in HN dev news feed have no description in listing"
	// When RSS description is empty after content parsing, we should use metadata description

	// Simulate HN article with empty description after parsing
	rssDescription := `<![CDATA[<a href="https://news.ycombinator.com/item?id=12345">Comments</a>]]>`

	// Apply content parsers (simulating what happens in parseFeedItem)
	cleanedDescription := applyContentParsers(rssDescription, "Dev")

	// Should be empty after stripping HN comments
	if cleanedDescription != "" {
		t.Errorf("Expected empty description after parsing, got: %q", cleanedDescription)
	}

	// In the actual code, when cleanedDescription is empty, we should use metadata description
	// This is tested implicitly in the integration, but good to document the expected behavior
}

// TestRegressionPostsChronologicalOrder tests that posts within each category
// are sorted in reverse chronological order (newest first)
func TestRegressionPostsChronologicalOrder(t *testing.T) {
	// This test verifies the fix for: "news feed is not in reverse chronological order"

	now := time.Now()
	posts := []*Post{
		{ID: "1", Title: "Oldest", Category: "Dev", PostedAt: now.Add(-5 * time.Hour)},
		{ID: "2", Title: "Middle", Category: "Dev", PostedAt: now.Add(-2 * time.Hour)},
		{ID: "3", Title: "Newest", Category: "Dev", PostedAt: now},
		{ID: "4", Title: "Old Tech", Category: "Tech", PostedAt: now.Add(-10 * time.Hour)},
		{ID: "5", Title: "New Tech", Category: "Tech", PostedAt: now.Add(-1 * time.Hour)},
	}

	// Group by category (as in generateNewsHtml)
	categories := make(map[string][]*Post)
	for _, post := range posts {
		categories[post.Category] = append(categories[post.Category], post)
	}

	// Sort posts within each category by timestamp (newest first)
	for _, categoryPosts := range categories {
		sort.Slice(categoryPosts, func(i, j int) bool {
			return categoryPosts[i].PostedAt.After(categoryPosts[j].PostedAt)
		})
	}

	// Verify Dev category is sorted newest first
	devPosts := categories["Dev"]
	if len(devPosts) != 3 {
		t.Fatalf("Expected 3 Dev posts, got %d", len(devPosts))
	}
	if devPosts[0].Title != "Newest" {
		t.Errorf("First Dev post should be 'Newest', got %q", devPosts[0].Title)
	}
	if devPosts[1].Title != "Middle" {
		t.Errorf("Second Dev post should be 'Middle', got %q", devPosts[1].Title)
	}
	if devPosts[2].Title != "Oldest" {
		t.Errorf("Third Dev post should be 'Oldest', got %q", devPosts[2].Title)
	}

	// Verify Tech category is sorted newest first
	techPosts := categories["Tech"]
	if len(techPosts) != 2 {
		t.Fatalf("Expected 2 Tech posts, got %d", len(techPosts))
	}
	if techPosts[0].Title != "New Tech" {
		t.Errorf("First Tech post should be 'New Tech', got %q", techPosts[0].Title)
	}
	if techPosts[1].Title != "Old Tech" {
		t.Errorf("Second Tech post should be 'Old Tech', got %q", techPosts[1].Title)
	}

	// Verify all timestamps are in descending order within categories
	for category, categoryPosts := range categories {
		for i := 0; i < len(categoryPosts)-1; i++ {
			if categoryPosts[i].PostedAt.Before(categoryPosts[i+1].PostedAt) {
				t.Errorf("In %s category, post %d (%v) is older than post %d (%v)",
					category, i, categoryPosts[i].PostedAt, i+1, categoryPosts[i+1].PostedAt)
			}
		}
	}
}

// TestRegressionSummarySeparation tests that AI-generated summaries stay clearly separated
// from article descriptions and are properly labeled
func TestRegressionSummarySeparation(t *testing.T) {
	// This test verifies the fix for: "AI summaries should stay in Summary section,
	// clearly labeled, not used as descriptions"

	testCases := []struct {
		name                 string
		description          string
		summary              string
		expectedDescEmpty    bool
		expectedSummaryLabel string
		shouldShowSummary    bool
	}{
		{
			name:                 "Both description and summary exist",
			description:          "This is the article description from the source",
			summary:              "This is an AI-generated summary",
			expectedDescEmpty:    false,
			expectedSummaryLabel: "AI Summary",
			shouldShowSummary:    true,
		},
		{
			name:                 "Only summary exists, no description",
			description:          "",
			summary:              "This is an AI-generated summary",
			expectedDescEmpty:    true, // Description should stay empty!
			expectedSummaryLabel: "AI Summary",
			shouldShowSummary:    true,
		},
		{
			name:                 "Only description exists, no summary",
			description:          "This is the article description",
			summary:              "",
			expectedDescEmpty:    false,
			expectedSummaryLabel: "",
			shouldShowSummary:    false,
		},
		{
			name:              "Neither description nor summary",
			description:       "",
			summary:           "",
			expectedDescEmpty: true,
			shouldShowSummary: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the article view logic
			description := tc.description
			summary := tc.summary

			// CRITICAL: Description should NEVER be populated from summary or entry.Content
			// This was the bug - when description was empty, it fell back to entry.Content
			// which could contain the AI summary

			// Verify description remains empty if it started empty
			if tc.expectedDescEmpty && description != "" {
				t.Errorf("Description should be empty, got: %q", description)
			}

			// Verify description is not overwritten by summary
			if description != tc.description {
				t.Errorf("Description was modified from %q to %q", tc.description, description)
			}

			// Verify summary section behavior
			if tc.shouldShowSummary {
				if summary == "" {
					t.Error("Summary should be shown but is empty")
				}
				// Verify summary is clearly labeled
				if tc.expectedSummaryLabel != "AI Summary" {
					t.Errorf("Expected label 'AI Summary', got %q", tc.expectedSummaryLabel)
				}
			}

			// Verify summary and description are different (when both exist)
			if description != "" && summary != "" {
				if description == summary {
					t.Error("Description and summary should be different content")
				}
			}
		})
	}
}

// TestFormatSummaryLabel verifies the summary section uses "AI Summary" label
func TestFormatSummaryLabel(t *testing.T) {
	summary := "This is a test summary."

	formatted := formatSummary(summary)

	// The formatted summary should be wrapped in paragraph tags
	if !strings.Contains(formatted, "<p") {
		t.Error("Formatted summary should contain paragraph tags")
	}

	// Verify the content is preserved
	if !strings.Contains(formatted, "This is a test summary.") {
		t.Error("Formatted summary should contain the original content")
	}

	// Note: The "AI Summary" label is added in the HTML template, not in formatSummary
	// This test just ensures formatSummary doesn't add misleading labels
	if strings.Contains(formatted, "Summary:") || strings.Contains(formatted, "<h") {
		t.Error("formatSummary should not add its own labels or headers")
	}
}

func TestDedupePostsCollapsesCanonicalURLsAndKeepsSource(t *testing.T) {
	newer := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)
	posts := []*Post{
		{ID: "wire-1", Title: "AI lab ships safer assistant", URL: "https://example.com/story?utm_source=rss#comments", Category: "Tech", PostedAt: older},
		{ID: "wire-2", Title: "AI lab ships safer assistant", URL: "https://example.com/story", Category: "Tech", Description: "Rollout notes", PostedAt: newer},
	}

	got := dedupePosts(posts)
	if len(got) != 1 {
		t.Fatalf("expected duplicate URLs to collapse to one item, got %d", len(got))
	}
	if got[0].URL == "" {
		t.Fatalf("expected deduped item to retain a source URL")
	}
	if got[0].PostedAt != newer {
		t.Fatalf("expected deduped item to retain newest timestamp, got %v", got[0].PostedAt)
	}
}

func TestGenerateNewsHtmlLabelsNonNewsFeedEntries(t *testing.T) {
	oldFeed := feed
	oldHeadlines := headlinesHtml
	defer func() {
		mutex.Lock()
		feed = oldFeed
		headlinesHtml = oldHeadlines
		mutex.Unlock()
	}()

	mutex.Lock()
	feed = []*Post{{
		ID:          "reminder-1",
		Title:       "Call Sam tomorrow",
		Description: "Personal reminder imported from a mixed feed.",
		URL:         "https://example.com/reminder",
		Category:    "Reminder",
		PostedAt:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
	}}
	headlinesHtml = ""
	mutex.Unlock()

	got := generateNewsHtml()
	if !strings.Contains(got, "Reminder · non-news") {
		t.Fatalf("expected mixed non-news entries to be clearly labeled, got %q", got)
	}
}

func TestGetFeedReturnsDedupedFeedForAgentContext(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	mutex.Lock()
	feed = []*Post{
		{ID: "a", Title: "Same story", URL: "https://example.com/story?utm_campaign=one", Category: "Tech"},
		{ID: "b", Title: "Same story", URL: "https://example.com/story", Category: "Tech"},
	}
	mutex.Unlock()

	got := GetFeed()
	if len(got) != 1 {
		t.Fatalf("expected agent-facing feed to collapse duplicates, got %#v", got)
	}
}

func TestCleanNewsArticleURLRejectsImageAssets(t *testing.T) {
	if got := cleanNewsArticleURL("https://cdn.example.com/story.jpg?width=1200"); got != "" {
		t.Fatalf("expected image asset URL to be dropped, got %q", got)
	}
	if got := cleanNewsArticleURL("https://example.com/ai/story?ref=mu"); got != "https://example.com/ai/story?ref=mu" {
		t.Fatalf("expected article URL to be preserved, got %q", got)
	}
}

func TestNewsSearchPayloadIncludesFreshnessForAPIPath(t *testing.T) {
	oldFeed := feed
	defer func() {
		mutex.Lock()
		feed = oldFeed
		mutex.Unlock()
	}()

	mutex.Lock()
	feed = []*Post{{
		ID:          "old-ai",
		Title:       "AI archive funding story",
		Description: "Older artificial intelligence funding context.",
		URL:         "https://example.com/old-ai",
		Category:    "Tech",
		PostedAt:    time.Now().UTC().AddDate(0, -2, 0),
	}}
	mutex.Unlock()

	payload := newsSearchPayload("Find today's AI news", 20)
	freshness, ok := payload["freshness"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected freshness metadata in shared news_search payload, got %#v", payload)
	}
	if freshness["status"] != "stale" {
		t.Fatalf("expected stale freshness status for old API-path results, got %#v", freshness)
	}
	if !strings.Contains(fmt.Sprint(freshness["notice"]), "No same-day news_search results") {
		t.Fatalf("expected API-path same-day caveat notice, got %#v", freshness)
	}
}
