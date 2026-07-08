package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	gmai "go-micro.dev/v6/ai"
)

func quoteJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestPlacesMapURL_QueryAndNear(t *testing.T) {
	args := map[string]any{"q": "cafe", "near": "Hampton, UK"}
	items := []placeItem{{Name: "Test Cafe", Lat: 51.4, Lon: -0.37}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	if !strings.Contains(got, "cafe") {
		t.Errorf("expected query 'cafe' in URL, got %q", got)
	}
	if !strings.Contains(got, "Hampton") {
		t.Errorf("expected 'Hampton' in URL, got %q", got)
	}
}

func TestPlacesMapURL_QueryOnly(t *testing.T) {
	args := map[string]any{"q": "pharmacy"}
	items := []placeItem{{Name: "Boots", Lat: 51.5, Lon: -0.1}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	if !strings.Contains(got, "pharmacy") {
		t.Errorf("expected 'pharmacy' in URL, got %q", got)
	}
}

func TestPlacesMapURL_AddressArg(t *testing.T) {
	// places_nearby uses "address" instead of "near"; without a keyword
	// query the function should fall back to coordinate-based centering.
	args := map[string]any{"address": "London"}
	items := []placeItem{{Name: "Park", Lat: 51.5, Lon: -0.1}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	// Coordinate-based fallback should embed the place's lat/lon.
	if !strings.Contains(got, "51.5") {
		t.Errorf("expected latitude in coordinate fallback URL, got %q", got)
	}
}

func TestPlacesMapURL_FallbackToCoordinates(t *testing.T) {
	args := map[string]any{}
	items := []placeItem{{Name: "Mystery Place", Lat: 51.4, Lon: -0.37}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	if !strings.Contains(got, "51.4") {
		t.Errorf("expected latitude in URL, got %q", got)
	}
}

func TestPlacesMapURL_FallbackToPlacesPage(t *testing.T) {
	// No args, no coordinate data → /places
	got := placesMapURL(nil, []placeItem{{Name: "No Coords"}})
	if got != "/places" {
		t.Errorf("expected /places fallback, got %q", got)
	}
}

func TestFormatPlacesResult_WithResults(t *testing.T) {
	result := `{"results":[{"name":"Blue Cafe","category":"cafe","address":"12 High St"},{"name":"Red Cafe","category":"cafe","address":"5 Market St"}],"count":2}`
	args := map[string]any{"q": "cafe", "near": "Hampton, UK"}
	got := formatPlacesResult(result, args)
	if !strings.Contains(got, "Blue Cafe") {
		t.Errorf("expected 'Blue Cafe' in output, got %q", got)
	}
	if !strings.Contains(got, "Red Cafe") {
		t.Errorf("expected 'Red Cafe' in output, got %q", got)
	}
	if !strings.Contains(got, "Hampton") {
		t.Errorf("expected location in header, got %q", got)
	}
	if !strings.Contains(got, "cafe") {
		t.Errorf("expected query in header, got %q", got)
	}
}

func TestFormatPlacesResult_EmptyResults(t *testing.T) {
	result := `{"results":[],"count":0}`
	got := formatPlacesResult(result, nil)
	if got != "No places found." {
		t.Errorf("expected 'No places found.', got %q", got)
	}
}

func TestFormatPlacesResult_InvalidJSON(t *testing.T) {
	result := `not json`
	got := formatPlacesResult(result, nil)
	// Should fall back to original result
	if got != result {
		t.Errorf("expected original result as fallback, got %q", got)
	}
}

func TestRenderPlacesCard_MapLink(t *testing.T) {
	result := `{"results":[{"name":"Hampton Cafe","category":"cafe","address":"1 High St, Hampton"}],"count":1}`
	args := map[string]any{"q": "cafe", "near": "Hampton, UK"}
	card := renderPlacesCard(result, args)
	if !strings.Contains(card, "google.com/maps") {
		t.Errorf("expected google maps link in card, got %q", card)
	}
	if !strings.Contains(card, "Open in Google Maps ↗") {
		t.Errorf("expected 'Open in Google Maps ↗' link text, got %q", card)
	}
	if strings.Contains(card, `href="/places"`) {
		t.Errorf("card should not contain generic /places link, got %q", card)
	}
}

func TestRenderPlacesCard_Empty(t *testing.T) {
	got := renderPlacesCard(`{"results":[],"count":0}`, nil)
	if got != "" {
		t.Errorf("expected empty string for empty results, got %q", got)
	}
}

func TestFormatNewsResult_Feed(t *testing.T) {
	result := `{"feed":[{"title":"Bitcoin hits new high","description":"BTC reaches $100k","category":"crypto","url":"/news?id=1"},{"title":"Tech stocks rise","description":"Markets rally","category":"tech","url":"/news?id=2"}]}`
	got := formatNewsResult(result)
	if !strings.Contains(got, "Latest news") {
		t.Errorf("expected 'Latest news' header, got %q", got)
	}
	if !strings.Contains(got, "Bitcoin hits new high") {
		t.Errorf("expected article title, got %q", got)
	}
	if !strings.Contains(got, "crypto") {
		t.Errorf("expected category, got %q", got)
	}
	if !strings.Contains(got, "BTC reaches $100k") {
		t.Errorf("expected description, got %q", got)
	}
}

func TestFormatNewsResult_Search(t *testing.T) {
	result := `{"query":"bitcoin","results":[{"title":"Bitcoin price analysis","description":"BTC analysis","category":"crypto","url":"/news?id=1"}],"count":1}`
	got := formatNewsResult(result)
	if !strings.Contains(got, "bitcoin") {
		t.Errorf("expected query in header, got %q", got)
	}
	if !strings.Contains(got, "Bitcoin price analysis") {
		t.Errorf("expected article title, got %q", got)
	}
}

func TestFormatNewsResult_Empty(t *testing.T) {
	result := `{"feed":[]}`
	got := formatNewsResult(result)
	if got != "No news available." {
		t.Errorf("expected 'No news available.', got %q", got)
	}
}

func TestFormatVideoResult_WithResults(t *testing.T) {
	result := `{"results":[{"title":"Bitcoin explained","channel":"CryptoChannel","url":"https://youtube.com/watch?v=1"},{"title":"ETH 2.0","channel":"EthereumTV","url":"https://youtube.com/watch?v=2"}]}`
	got := formatVideoResult(result)
	if !strings.Contains(got, "Video results") {
		t.Errorf("expected 'Video results' header, got %q", got)
	}
	if !strings.Contains(got, "Bitcoin explained") {
		t.Errorf("expected video title, got %q", got)
	}
	if !strings.Contains(got, "CryptoChannel") {
		t.Errorf("expected channel name, got %q", got)
	}
}

func TestFormatVideoResult_Empty(t *testing.T) {
	result := `{"results":[]}`
	got := formatVideoResult(result)
	if got != "No videos found." {
		t.Errorf("expected 'No videos found.', got %q", got)
	}
}

func TestFormatReminderResult_WithData(t *testing.T) {
	result := `{"verse":"In the name of Allah","name":"Al-Rahman","hadith":"Narrated Abu Hurairah","message":"Be mindful of Allah"}`
	got := formatReminderResult(result)
	if !strings.Contains(got, "Daily Islamic reminder") {
		t.Errorf("expected header, got %q", got)
	}
	if !strings.Contains(got, "Al-Rahman") {
		t.Errorf("expected name of Allah, got %q", got)
	}
	if !strings.Contains(got, "In the name of Allah") {
		t.Errorf("expected verse, got %q", got)
	}
	if !strings.Contains(got, "Be mindful of Allah") {
		t.Errorf("expected message, got %q", got)
	}
}

func TestFormatReminderResult_Empty(t *testing.T) {
	result := `{"verse":"","name":"","hadith":"","message":""}`
	got := formatReminderResult(result)
	if got != "Reminder data unavailable." {
		t.Errorf("expected 'Reminder data unavailable.', got %q", got)
	}
}

func TestFormatSearchResult_HTML(t *testing.T) {
	result := `<html><body><div class="card"><a href="/news?id=1">Bitcoin price today</a><p>BTC analysis</p></div></body></html>`
	got := formatSearchResult(result)
	if strings.Contains(got, "<html>") || strings.Contains(got, "<body>") {
		t.Errorf("expected HTML tags stripped, got %q", got)
	}
	if !strings.Contains(got, "Bitcoin price today") {
		t.Errorf("expected text content preserved, got %q", got)
	}
}

func TestFormatSearchResult_JSON(t *testing.T) {
	result := `{"query":"bitcoin","results":[{"title":"Bitcoin news","content":"Latest BTC updates","type":"news"}]}`
	got := formatSearchResult(result)
	if !strings.Contains(got, "bitcoin") {
		t.Errorf("expected query, got %q", got)
	}
	if !strings.Contains(got, "Bitcoin news") {
		t.Errorf("expected result title, got %q", got)
	}
}

func TestCurrentDateContextIncludesISORequestDate(t *testing.T) {
	got := currentDateContext(time.Date(2026, 6, 30, 23, 59, 0, 0, time.FixedZone("BST", 3600)))
	if !strings.Contains(got, "Tuesday, 30 June 2026") {
		t.Fatalf("expected human-readable UTC date, got %q", got)
	}
	if !strings.Contains(got, "2026-06-30") {
		t.Fatalf("expected ISO date anchor, got %q", got)
	}
}

func TestFormatToolResultAddsCurrentDateToLiveResults(t *testing.T) {
	for _, tt := range []struct {
		name   string
		tool   string
		result string
	}{
		{name: "news", tool: "news", result: `{"feed":[{"title":"Test headline","category":"tech"}]}`},
		{name: "weather", tool: "weather_forecast", result: "Weather for London.\nNow: 18°C, cloudy."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToolResult(tt.tool, tt.result, nil)
			if !strings.Contains(got, "Current request date:") {
				t.Fatalf("expected current request date in %s tool context, got %q", tt.tool, got)
			}
			if !strings.Contains(got, time.Now().UTC().Format("2006-01-02")) {
				t.Fatalf("expected today's ISO date in %s tool context, got %q", tt.tool, got)
			}
		})
	}
}

func TestFormatToolResultKeepsExistingCurrentDateContext(t *testing.T) {
	result := "Current request date: Tuesday, 30 June 2026 (2026-06-30, UTC).\nWeather for London."
	got := formatToolResult("weather_forecast", result, nil)
	if strings.Count(got, "Current request date:") != 1 {
		t.Fatalf("expected one current request date context, got %q", got)
	}
	if !strings.Contains(got, "2026-06-30") {
		t.Fatalf("expected existing request date to be preserved, got %q", got)
	}
}

func TestFormatToolResultMarketsTextStaysReadable(t *testing.T) {
	result := `{"text":"Current request date: Tuesday, 30 June 2026 (2026-06-30, UTC).\nLive crypto prices:\nBTC: $100000.00 (+2.50% 24h)\nETH: $3000.00 (-1.25% 24h)"}`
	got := formatToolResult("markets", result, nil)
	if strings.Contains(got, `{"text"`) {
		t.Fatalf("expected readable markets context instead of raw JSON, got %q", got)
	}
	if strings.Count(got, "Current request date:") != 1 {
		t.Fatalf("expected one current request date context, got %q", got)
	}
	for _, want := range []string{"BTC: $100000.00 (+2.50% 24h)", "ETH: $3000.00 (-1.25% 24h)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in readable markets context, got %q", want, got)
		}
	}
}

func TestFormatToolResultMarketsRESTDataStaysReadable(t *testing.T) {
	result := `{"category":"crypto","data":[{"symbol":"BTC","price":100000,"change_24h":2.5},{"symbol":"DOGE","price":0.1234567,"change_24h":-4}]}`
	got := formatToolResult("markets", result, nil)
	if strings.Contains(got, `{"category"`) {
		t.Fatalf("expected readable markets context instead of raw JSON, got %q", got)
	}
	for _, want := range []string{"Current request date:", "Live crypto prices:", "BTC: $100000.00 (+2.50% 24h)", "DOGE: $0.123457 (-4.00% 24h)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in readable markets context, got %q", want, got)
		}
	}
}

func TestFormatToolResultMarketsRESTDataIncludesFreshnessDisclosure(t *testing.T) {
	result := `{"category":"crypto","updated_at":"2026-07-01T12:00:00Z","stale":true,"partial":true,"freshness":"Last refresh: 2026-07-01 12:00 UTC; data may be stale; some symbols are unavailable from the current source","data":[{"symbol":"BTC","price":97000,"change_24h":1.23,"source":"Coinbase + CoinGecko"}]}`
	got := formatToolResult("markets", result, nil)
	for _, want := range []string{"Last refresh: 2026-07-01 12:00 UTC", "market data may be stale", "some requested symbols are unavailable", "BTC: $97000.00 (+1.23% 24h)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in readable markets context, got %q", want, got)
		}
	}
}

func TestToolCallKeyDedupesEquivalentArgs(t *testing.T) {
	first := toolCallKey("markets", map[string]any{"category": "crypto", "limit": float64(10)})
	second := toolCallKey("markets", map[string]any{"limit": float64(10), "category": "crypto"})
	if first != second {
		t.Fatalf("expected equivalent tool args to share a dedupe key: %q vs %q", first, second)
	}
	if first == toolCallKey("markets", map[string]any{"category": "futures", "limit": float64(10)}) {
		t.Fatal("expected distinct market categories to keep distinct dedupe keys")
	}
}

func TestShortcutToolCallsMarketMoversAvoidsNewsPlanning(t *testing.T) {
	for _, prompt := range []string{
		"What is moving in markets today?",
		"What is moving in markets?",
		"Which stocks are moving?",
	} {
		got := shortcutToolCalls(prompt)
		if len(got) != 1 {
			t.Fatalf("%q: expected one shortcut tool call, got %#v", prompt, got)
		}
		if got[0].Tool != "markets" {
			t.Fatalf("%q: expected markets-only shortcut, got %#v", prompt, got)
		}
	}
}

func TestShortcutToolCallsMarketMoverExplanationUsesPlanner(t *testing.T) {
	if got := shortcutToolCalls("Why is Bitcoin moving today?"); len(got) != 0 {
		t.Fatalf("expected explanation prompt to use planner, got %#v", got)
	}
	if got := shortcutToolCalls("What is moving in markets, and what news explains it?"); len(got) != 0 {
		t.Fatalf("expected cross-source explanation prompt to use planner, got %#v", got)
	}
}

func TestUseFastToolFallbackOnlyForGuestMarketMovers(t *testing.T) {
	rag := []string{"### markets\nLive crypto prices:\nBTC: $97000.00 (+1.23% 24h)"}
	if !useFastToolFallback("What is moving in markets today?", true, true, false, false, false, false, rag) {
		t.Fatal("expected guest market-mover prompts with market data to use the fast fallback")
	}
	if useFastToolFallback("What is moving in markets today?", false, true, false, false, false, false, rag) {
		t.Fatal("authenticated users should keep full synthesis")
	}
	if useFastToolFallback("Why is Bitcoin moving today?", true, true, false, false, false, false, rag) {
		t.Fatal("explanation prompts should keep full synthesis")
	}
	if useFastToolFallback("What is moving in markets today?", true, false, false, false, false, false, rag) {
		t.Fatal("fallback requires a market tool result")
	}
	if useFastToolFallback("What is moving in markets today?", true, true, false, false, false, false, nil) {
		t.Fatal("fallback requires result context")
	}
}

func TestUseFastToolFallbackForGuestSimpleWeather(t *testing.T) {
	rag := []string{"### weather_forecast\nWeather for New York.\nNow: 21°C, partly cloudy.\nFreshness/source: source Google Weather; generated at 2026-07-02 12:00 UTC."}
	if !useFastToolFallback("Weather in New York today", true, false, true, false, false, false, rag) {
		t.Fatal("expected simple guest weather prompts with weather data to use the fast fallback")
	}
	if useFastToolFallback("Compare weather in New York and London", true, false, true, false, false, false, rag) {
		t.Fatal("comparative weather prompts should keep full synthesis")
	}
	if useFastToolFallback("Weather in New York today", false, false, true, false, false, false, rag) {
		t.Fatal("authenticated users should keep full synthesis")
	}
	if useFastToolFallback("Weather in New York today", true, false, false, false, false, false, rag) {
		t.Fatal("fallback requires a weather tool result")
	}
}

func TestUseFastToolFallbackForGuestUnavailableNewsWebFallback(t *testing.T) {
	rag := []string{
		"### web_search\nCurrent date context: request date is 2026-07-02 UTC.\nSearch results for \"AI news\":\n1. AI story — snippet grounded in the web result (https://example.com/ai)",
		"### news_search\nnews_search is unavailable right now. Use any other available live results to answer, and mention this unavailable source briefly without exposing internal payloads.",
	}
	if !useFastToolFallback("Find today's AI news", true, false, false, true, false, true, rag) {
		t.Fatal("expected guest latest-news web fallback to skip LLM synthesis")
	}
	if useFastToolFallback("Find today's AI news", false, false, false, true, false, true, rag) {
		t.Fatal("authenticated users should keep full synthesis")
	}
	if useFastToolFallback("Find today's AI news", true, false, false, true, false, false, rag) {
		t.Fatal("fallback requires unavailable news_search disclosure")
	}
	if useFastToolFallback("Find today's AI news", true, false, false, false, false, true, rag) {
		t.Fatal("fallback requires available web_search context")
	}
	if useFastToolFallback("Find saved AI article", true, false, false, true, false, true, rag) {
		t.Fatal("non-current news prompts should keep full synthesis")
	}

	if !useFastToolFallback("Find today's AI news", true, false, false, false, true, false, []string{"### news_search\nNews results for \"AI news\":\n1. AI story https://example.com/ai"}) {
		t.Fatal("expected successful guest news_search results to skip LLM synthesis")
	}
}

func TestShortcutToolCallsLatestTechnologyNews(t *testing.T) {
	for _, tt := range []struct {
		prompt string
		query  string
	}{
		{prompt: "latest technology news", query: "technology news"},
		{prompt: "What is the latest AI news today?", query: "AI news"},
		{prompt: "current artificial intelligence news", query: "artificial intelligence news"},
	} {
		got := shortcutToolCalls(tt.prompt)
		if len(got) != 1 {
			t.Fatalf("expected one shortcut tool call for %q, got %#v", tt.prompt, got)
		}
		if got[0].Tool != "news_search" {
			t.Fatalf("expected news_search shortcut for %q, got %#v", tt.prompt, got)
		}
		if got[0].Args["query"] != tt.query {
			t.Fatalf("expected %q query for %q, got %#v", tt.query, tt.prompt, got[0].Args)
		}
	}
}

func TestFallbackNewsSearchToolCallUsesWebSearchForLatestAINews(t *testing.T) {
	got, ok := fallbackNewsSearchToolCall("Find today's AI news", "news_search", map[string]any{"query": "technology news"})
	if !ok {
		t.Fatal("expected latest AI news prompts to fall back when news_search is unavailable")
	}
	if got.Tool != "web_search" {
		t.Fatalf("expected web_search fallback, got %#v", got)
	}
	if got.Args["q"] != "AI news" {
		t.Fatalf("expected fallback to preserve prompt topic, got %#v", got.Args)
	}
}

func TestFallbackNewsSearchToolCallIgnoresNonNewsSearchFailures(t *testing.T) {
	if got, ok := fallbackNewsSearchToolCall("Find today's AI news", "weather_forecast", nil); ok {
		t.Fatalf("expected non-news tool failures to be ignored, got %#v", got)
	}
	if got, ok := fallbackNewsSearchToolCall("old saved AI article", "news_search", map[string]any{"query": "AI"}); ok {
		t.Fatalf("expected non-current news prompts to be ignored, got %#v", got)
	}
}

func TestSkipMarketMoverCompanionToolFiltersUnrequestedNews(t *testing.T) {
	prompt := "What is moving in markets today?"
	for _, tool := range []string{"news", "news_headlines", "news_search", "web_search", "recall"} {
		if !skipMarketMoverCompanionTool(prompt, tool) {
			t.Fatalf("expected %s to be skipped for market-mover prompt", tool)
		}
	}
	if skipMarketMoverCompanionTool(prompt, "markets") {
		t.Fatal("expected markets tool to remain available")
	}
}

func TestSkipMarketMoverCompanionToolFiltersAssetMoverPrompts(t *testing.T) {
	for _, prompt := range []string{
		"What crypto is moving today?",
		"Which stocks are up today?",
		"Any Bitcoin rally today?",
	} {
		if !skipMarketMoverCompanionTool(prompt, "news_headlines") {
			t.Fatalf("expected unrequested news to be skipped for %q", prompt)
		}
	}
}

func TestSkipMarketMoverCompanionToolAllowsExplanatoryNews(t *testing.T) {
	prompt := "What is moving in markets today, and what news explains the moves?"
	if skipMarketMoverCompanionTool(prompt, "news_headlines") {
		t.Fatal("expected explanatory market-mover prompt to allow news")
	}
}

func TestFormatToolResult_Dispatch(t *testing.T) {
	// Ensure the dispatcher calls the right formatter
	newsResult := `{"feed":[{"title":"Test headline","category":"tech"}]}`
	got := formatToolResult("news", newsResult, nil)
	if !strings.Contains(got, "Test headline") {
		t.Errorf("expected news formatter to be called, got %q", got)
	}

	unknownResult := `{"foo":"bar"}`
	got = formatToolResult("unknown_tool", unknownResult, nil)
	if got != unknownResult {
		t.Errorf("expected raw result for unknown tool, got %q", got)
	}
}

func TestFormatWalletBalanceResult_WithBalance(t *testing.T) {
	result := `{"balance":1550}`
	got := formatWalletBalanceResult(result)
	if !strings.Contains(got, "1550 credits") {
		t.Errorf("expected credits in output, got %q", got)
	}
	if !strings.Contains(got, "£15.50") {
		t.Errorf("expected formatted pounds in output, got %q", got)
	}
	if !strings.Contains(got, "/wallet/topup") {
		t.Errorf("expected topup link in output, got %q", got)
	}
}

func TestFormatWalletBalanceResult_ZeroBalance(t *testing.T) {
	result := `{"balance":0}`
	got := formatWalletBalanceResult(result)
	if !strings.Contains(got, "0 credits") {
		t.Errorf("expected zero credits in output, got %q", got)
	}
	if !strings.Contains(got, "£0.00") {
		t.Errorf("expected £0.00 in output, got %q", got)
	}
}

func TestFormatWalletBalanceResult_InvalidJSON(t *testing.T) {
	result := `not json`
	got := formatWalletBalanceResult(result)
	if got != result {
		t.Errorf("expected original result as fallback, got %q", got)
	}
}

func TestFormatWalletTopupResult_WithMethods(t *testing.T) {
	result := `{"methods":[{"type":"card","tiers":[{"amount":1000,"credits":1000,"label":"£10"},{"amount":5000,"credits":5000,"label":"£50"}]}]}`
	got := formatWalletTopupResult(result)
	if !strings.Contains(got, "/wallet/topup") {
		t.Errorf("expected topup URL in output, got %q", got)
	}
	if !strings.Contains(got, "card payment") && !strings.Contains(got, "card") {
		t.Errorf("expected card payment label in output, got %q", got)
	}
	if !strings.Contains(got, "£10") {
		t.Errorf("expected tier label in output, got %q", got)
	}
	if !strings.Contains(got, "1000 credits") {
		t.Errorf("expected credits in output, got %q", got)
	}
}

func TestFormatWalletTopupResult_NoMethods(t *testing.T) {
	result := `{"methods":[]}`
	got := formatWalletTopupResult(result)
	if !strings.Contains(got, "/wallet/topup") {
		t.Errorf("expected topup URL in no-methods output, got %q", got)
	}
}

func TestFormatWalletTopupResult_InvalidJSON(t *testing.T) {
	result := `not json`
	got := formatWalletTopupResult(result)
	if got != result {
		t.Errorf("expected original result as fallback, got %q", got)
	}
}

func TestFormatNewsResult_WithTimestamps(t *testing.T) {
	result := `{"feed":[{"title":"Iran crisis","description":"Conflict escalates","category":"world","url":"/news?id=1","posted_at":"2026-03-02T10:00:00Z","published":"Sun, 02 Mar 2026 10:00:00 +0000"},{"title":"Peace talks","description":"Negotiations begin","category":"world","url":"/news?id=2","posted_at":"2026-03-01T08:00:00Z"}]}`
	got := formatNewsResult(result)
	if !strings.Contains(got, "Iran crisis") {
		t.Errorf("expected title, got %q", got)
	}
	if !strings.Contains(got, "2 Mar 2026") {
		t.Errorf("expected formatted date, got %q", got)
	}
	if !strings.Contains(got, "1 Mar 2026") {
		t.Errorf("expected second article date, got %q", got)
	}
}

func TestFormatNewsResult_SearchWithTimestamp(t *testing.T) {
	result := `{"query":"iran","results":[{"title":"Iran news","description":"Latest","category":"world","url":"/news?id=1","posted_at":"2026-03-02T12:00:00Z"}],"count":1}`
	got := formatNewsResult(result)
	if !strings.Contains(got, "iran") {
		t.Errorf("expected query, got %q", got)
	}
	if !strings.Contains(got, "2 Mar 2026") {
		t.Errorf("expected formatted date, got %q", got)
	}
}

func TestFormatNewsResultUsesCleanMetadataLabels(t *testing.T) {
	result := `{"query":"AI news","results":[{"title":"AI lab ships update","description":"Fresh details","category":"Tech","url":"https://example.com/ai","posted_at":"2026-07-05T12:00:00Z"}],"count":1}`
	got := formatNewsResult(result)
	if !strings.Contains(got, "category: Tech; posted: 5 Jul 2026 12:00 UTC; source: https://example.com/ai") {
		t.Fatalf("expected category/date/source metadata labels, got %q", got)
	}
	if strings.Contains(got, "[Tech]") {
		t.Fatalf("expected metadata not to use markdown-link-like category brackets, got %q", got)
	}
}

func TestRenderToolCallRef_NewsSearch(t *testing.T) {
	args := map[string]any{"query": "Iran"}
	formatted := "News results for \"Iran\":\n1. Iran crisis [world] (2 Mar 2026 10:00) — Conflict escalates\n"
	got := renderToolCallRef("news_search", args, formatted)
	if !strings.Contains(got, "<details") {
		t.Errorf("expected <details> element, got %q", got)
	}
	if !strings.Contains(got, "<summary") {
		t.Errorf("expected <summary> element, got %q", got)
	}
	if !strings.Contains(got, "Iran") {
		t.Errorf("expected query in summary, got %q", got)
	}
	if !strings.Contains(got, "Iran crisis") {
		t.Errorf("expected formatted result content, got %q", got)
	}
}

func TestRenderToolCallRef_NoArgs(t *testing.T) {
	got := renderToolCallRef("news", nil, "Latest news:\n1. Test headline\n")
	if !strings.Contains(got, "<details") {
		t.Errorf("expected <details> element, got %q", got)
	}
	if !strings.Contains(got, "Test headline") {
		t.Errorf("expected content, got %q", got)
	}
}

func TestRenderToolCallRef_Category(t *testing.T) {
	args := map[string]any{"category": "crypto"}
	got := renderToolCallRef("markets", args, "Live crypto market prices:\n- BTC: $97000\n")
	if !strings.Contains(got, "crypto") {
		t.Errorf("expected category in label, got %q", got)
	}
}

func TestFormatToolResult_WalletDispatch(t *testing.T) {
	balanceResult := `{"balance":500}`
	got := formatToolResult("wallet_balance", balanceResult, nil)
	if !strings.Contains(got, "500 credits") {
		t.Errorf("expected wallet_balance formatter to be called, got %q", got)
	}

	topupResult := `{"methods":[{"type":"card","tiers":[{"amount":1000,"credits":1000,"label":"£10"}]}]}`
	got = formatToolResult("wallet_topup", topupResult, nil)
	if !strings.Contains(got, "topup") {
		t.Errorf("expected wallet_topup formatter to be called, got %q", got)
	}
}

func TestStripHTMLTags(t *testing.T) {
	html := `<div class="card"><h1>Title</h1><p>Some <b>bold</b> text.</p></div>`
	got := stripHTMLTags(html)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("expected HTML tags stripped, got %q", got)
	}
	if !strings.Contains(got, "Title") || !strings.Contains(got, "Some") || !strings.Contains(got, "bold") {
		t.Errorf("expected text content preserved, got %q", got)
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{90 * time.Second, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{48 * time.Hour, "2 days ago"},
	}
	for _, c := range cases {
		got := FormatAge(c.d)
		if got != c.want {
			t.Errorf("FormatAge(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestSaveAndGetFlow(t *testing.T) {
	// Reset in-memory store to avoid cross-test pollution.
	flowMu.Lock()
	flowStore = map[string]*Flow{}
	flowMu.Unlock()

	f := &Flow{
		ID:        "test-flow-1",
		AccountID: "user-123",
		Prompt:    "What is the weather in London?",
		Answer:    "It is cloudy.",
		Steps: []FlowStep{
			{Tool: "weather_forecast", Args: map[string]any{"lat": 51.5, "lon": -0.1}, Result: `{"forecast":{}}`},
		},
	}

	// Directly insert into the store to avoid disk I/O in unit tests.
	// The persistFlows path relies on the data package which writes to $HOME/.mu;
	// integration coverage for that is handled by the data package tests.
	flowMu.Lock()
	flowStore[f.ID] = f
	flowMu.Unlock()

	got := getFlow("test-flow-1")
	if got == nil {
		t.Fatal("expected flow to be found after save")
	}
	if got.Prompt != f.Prompt {
		t.Errorf("expected prompt %q, got %q", f.Prompt, got.Prompt)
	}
	if got.AccountID != f.AccountID {
		t.Errorf("expected accountID %q, got %q", f.AccountID, got.AccountID)
	}
	if len(got.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(got.Steps))
	}
	if got.Steps[0].Tool != "weather_forecast" {
		t.Errorf("expected tool 'weather_forecast', got %q", got.Steps[0].Tool)
	}
}

func TestListFlows(t *testing.T) {
	flowMu.Lock()
	flowStore = map[string]*Flow{}
	flowMu.Unlock()

	now := time.Now()
	flows := []*Flow{
		{ID: "a", AccountID: "user-1", Prompt: "Q1", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "b", AccountID: "user-1", Prompt: "Q2", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "c", AccountID: "user-2", Prompt: "Q3", CreatedAt: now},
	}
	flowMu.Lock()
	for _, f := range flows {
		flowStore[f.ID] = f
	}
	flowMu.Unlock()

	got := ListFlows("user-1")
	if len(got) != 2 {
		t.Fatalf("expected 2 flows for user-1, got %d", len(got))
	}
	// Should be newest first: b then a.
	if got[0].ID != "b" {
		t.Errorf("expected first flow to be 'b' (newest), got %q", got[0].ID)
	}
	if got[1].ID != "a" {
		t.Errorf("expected second flow to be 'a', got %q", got[1].ID)
	}
}

func TestDeleteFlow(t *testing.T) {
	flowMu.Lock()
	flowStore = map[string]*Flow{
		"del-1": {ID: "del-1", AccountID: "owner"},
		"del-2": {ID: "del-2", AccountID: "other"},
	}
	flowMu.Unlock()

	// Should not delete a flow owned by a different account.
	deleteFlow("owner", "del-2") //nolint:errcheck
	if getFlow("del-2") == nil {
		t.Error("deleteFlow should not remove a flow owned by a different account")
	}

	// Should delete the owner's own flow.
	deleteFlow("owner", "del-1") //nolint:errcheck
	if getFlow("del-1") != nil {
		t.Error("deleteFlow should remove the owner's flow")
	}
}

func TestGetFlow_NotFound(t *testing.T) {
	flowMu.Lock()
	flowStore = map[string]*Flow{}
	flowMu.Unlock()

	if got := getFlow("nonexistent"); got != nil {
		t.Errorf("expected nil for nonexistent flow, got %+v", got)
	}
}

func TestNewFlowID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id := newFlowID()
		if ids[id] {
			t.Errorf("newFlowID returned duplicate ID: %q", id)
		}
		ids[id] = true
	}
}

func TestFormatAppsRunResult_ValidJSON(t *testing.T) {
	result := `{"id":"abc123","url":"/apps/run?id=abc123","run":"/apps/run?id=abc123&raw=1"}`
	got := formatAppsRunResult(result)
	if !strings.Contains(got, "/apps/run?id=abc123") {
		t.Errorf("expected URL in formatted result, got %q", got)
	}
	if !strings.Contains(got, "sandbox") {
		t.Errorf("expected sandbox mention, got %q", got)
	}
}

func TestFormatAppsRunResult_InvalidJSON(t *testing.T) {
	result := `not json`
	got := formatAppsRunResult(result)
	if got != result {
		t.Errorf("expected original result as fallback, got %q", got)
	}
}

func TestRenderRunCard_ValidJSON(t *testing.T) {
	result := `{"id":"abc123","url":"/apps/run?id=abc123","run":"/apps/run?id=abc123&raw=1"}`
	card := renderRunCard(result)
	if !strings.Contains(card, "<iframe") {
		t.Errorf("expected iframe in card, got %q", card)
	}
	if !strings.Contains(card, `sandbox="allow-scripts" allow="geolocation"`) {
		t.Errorf("expected sandboxed iframe with geolocation, got %q", card)
	}
	if !strings.Contains(card, "Result") {
		t.Errorf("expected Result heading, got %q", card)
	}
}

func TestRenderRunCard_InvalidJSON(t *testing.T) {
	got := renderRunCard(`not json`)
	if got != "" {
		t.Errorf("expected empty string for invalid JSON, got %q", got)
	}
}

func TestRenderRunCard_EmptyRun(t *testing.T) {
	got := renderRunCard(`{"id":"abc","run":""}`)
	if got != "" {
		t.Errorf("expected empty string for missing run URL, got %q", got)
	}
}

func TestFormatToolResult_AppsRunDispatch(t *testing.T) {
	result := `{"id":"abc","url":"/apps/run?id=abc","run":"/apps/run?id=abc&raw=1"}`
	got := formatToolResult("apps_run", result, nil)
	if !strings.Contains(got, "sandbox") {
		t.Errorf("expected apps_run formatter to be called, got %q", got)
	}
}

func TestRenderResultCard_AppsRun(t *testing.T) {
	result := `{"id":"abc","run":"/apps/run?id=abc&raw=1"}`
	card := renderResultCard("apps_run", result, nil)
	if !strings.Contains(card, "<iframe") {
		t.Errorf("expected iframe in result card, got %q", card)
	}
}

func TestCompleteToolAnswerReplacesProgressOnlyWithResults(t *testing.T) {
	rag := []string{"### markets\nBTC: $100,000", "### news\n- Bitcoin reaches new high"}
	got := completeToolAnswer("Let me pull the latest market and news data for you.", rag)
	if strings.Contains(strings.ToLower(got), "let me pull") {
		t.Fatalf("expected progress narration to be replaced, got %q", got)
	}
	if strings.Contains(strings.ToLower(got), "couldn't synthesize") {
		t.Fatalf("expected synthesized fallback instead of generic incomplete-answer copy, got %q", got)
	}
	if strings.Contains(got, "**markets**") || strings.Contains(got, "**news**") || !strings.Contains(got, "BTC: $100,000") || !strings.Contains(got, "Bitcoin reaches new high") || strings.Contains(got, "Here's what I found") {
		t.Fatalf("expected answer-first fallback to include results without implementation headings, got %q", got)
	}
}

func TestCompleteToolAnswerReplacesSearchProgressWithNewsResults(t *testing.T) {
	rag := []string{`### news
News results for "AI":
1. Open model lab ships safer assistant [tech] /news?id=ai-1 — New evals and rollout notes.
2. Chipmakers expand data-center capacity [business] /news?id=ai-2 — Suppliers report stronger demand.`}
	got := completeToolAnswer("Let me search the web for more AI stories to round this out.", rag)
	lower := strings.ToLower(got)
	if strings.Contains(lower, "let me search") || strings.Contains(lower, "round this out") {
		t.Fatalf("expected search progress narration to be replaced, got %q", got)
	}
	if !strings.Contains(got, "Open model lab ships safer assistant") || !strings.Contains(got, "/news?id=ai-1") {
		t.Fatalf("expected fallback to include news headlines and sources, got %q", got)
	}
}

func TestCompleteToolAnswerNamesUnavailableSlices(t *testing.T) {
	rag := []string{"### weather\nNo weather data unavailable right now.", "### news\nLatest news:\n1. Useful headline"}
	got := completeToolAnswer("I'll check that now.", rag)
	if !strings.Contains(got, "Useful headline") {
		t.Fatalf("expected available slice in fallback, got %q", got)
	}
	if !strings.Contains(got, "Unavailable right now: weather.") {
		t.Fatalf("expected unavailable slice to be named clearly, got %q", got)
	}
}

func TestCompleteToolAnswerPrefersSuccessfulWeatherOverUnavailableMarker(t *testing.T) {
	rag := []string{
		"### weather_forecast\nWeather for New York.\nNow: 21°C, partly cloudy.\nFreshness/source: Google Weather; generated at 2026-07-02 12:00 UTC.",
		"### weather_forecast\n" + unavailableToolMessage("weather_forecast"),
	}
	got := completeToolAnswer("I'll check the weather now.", rag)
	if !strings.Contains(got, "Now: 21°C, partly cloudy") {
		t.Fatalf("expected successful weather result in fallback, got %q", got)
	}
	if strings.Contains(got, "Unavailable: weather_forecast") || strings.Contains(got, "Unavailable: weather") {
		t.Fatalf("did not expect unavailable weather disclosure when weather data is usable, got %q", got)
	}
}

func TestCompleteToolAnswerKeepsUsableWeatherWithInlineUnavailableMarker(t *testing.T) {
	rag := []string{
		"### weather_forecast\nWeather for New York today.\nNow: 21°C, partly cloudy.\nForecast: Thu 2026-07-02: high 24°C, low 18°C.\nFreshness/source: Google Weather; generated at 2026-07-02 12:00 UTC.\nUnavailable: weather_forecast.",
	}
	got := completeToolAnswer("I'll check the weather now.", rag)
	if !strings.Contains(got, "Weather for New York today") || !strings.Contains(got, "Freshness/source: Google Weather") {
		t.Fatalf("expected usable weather and freshness details in fallback, got %q", got)
	}
	if strings.Contains(got, "Unavailable: weather_forecast") || strings.Contains(got, "Unavailable: weather") {
		t.Fatalf("did not expect inline unavailable marker when weather data is usable, got %q", got)
	}
}

func TestCompleteToolAnswerUsesAvailableWebWhenNewsUnavailable(t *testing.T) {
	rag := []string{
		"### news\n" + unavailableToolMessage("news"),
		`### web_search
Web results for "latest AI news":
Query intent: answer the user's original query "latest AI news"; do not replace it with a broader or different meaning.
Confidence: high — synthesize only what the listed sources support.
Sources:
1. AI lab releases new model — Company announced a new assistant release. (https://example.com/ai-model)
2. Chip supplier expands AI capacity — Demand for AI chips is rising. (https://example.com/ai-chips)`,
	}
	got := completeToolAnswer("Let me search the web for more AI stories to round this out.", rag)
	if strings.Contains(got, `{"`) || strings.Contains(got, "Query intent:") {
		t.Fatalf("expected readable fallback without raw/internal search context, got %q", got)
	}
	if !strings.Contains(got, "AI lab releases new model") || !strings.Contains(got, "https://example.com/ai-model") {
		t.Fatalf("expected available web sources in fallback, got %q", got)
	}
	if !strings.Contains(got, "Unavailable right now: news.") {
		t.Fatalf("expected unavailable news disclosure, got %q", got)
	}
}

func TestCompleteToolAnswerPolishesNewsWebFallback(t *testing.T) {
	rag := []string{
		"### news_search\n" + unavailableToolMessage("news_search"),
		`### web_search
Current date context: request date is 2026-07-02 UTC.
Search results for "AI news":
Grounding rule: only use source-backed snippets.
Query intent: answer the user's original query "AI news"; do not replace it with a broader or different meaning.
Confidence: high — synthesize only what the listed sources support.
Sources:
1. AI lab releases new model — Company announced a new assistant release. (https://example.com/ai-model)
2. Chip supplier expands AI capacity — Demand for AI chips is rising. (https://example.com/ai-chips)`,
	}

	got := completeToolAnswer("Let me search the web for more AI stories to round this out.", rag)
	for _, internal := range []string{"Grounding rule:", "Query intent:", "Search results for", "Confidence:", "Sources:"} {
		if strings.Contains(got, internal) {
			t.Fatalf("expected polished fallback to hide %q, got %q", internal, got)
		}
	}
	if strings.Contains(got, "**Web sources**") || !strings.Contains(got, "AI lab releases new model") || !strings.Contains(got, "https://example.com/ai-model") {
		t.Fatalf("expected concise source-backed web summary, got %q", got)
	}
	if !strings.Contains(got, "Unavailable right now: news.") {
		t.Fatalf("expected human-readable unavailable news disclosure, got %q", got)
	}
}

func TestCompleteToolAnswerDatesTodayNewsWebFallback(t *testing.T) {
	rag := []string{
		"### news_search\n" + unavailableToolMessage("news_search"),
		`### web_search
Current request date: Saturday, 4 July 2026 (2026-07-04, UTC).
Search results for "AI news":
Grounding rule: only use source-backed snippets.
Sources:
1. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-directory)
2. AI lab releases new model — Company announced a new assistant release today. (https://example.com/ai-model)
3. Chip supplier expands AI capacity — Demand for AI chips is rising this week. (https://example.com/ai-chips)`,
	}

	got := completeToolAnswer("Here are the search results I found.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items for Saturday, 4 July 2026 (2026-07-04): AI lab releases new model; Chip supplier expands AI capacity.") {
		t.Fatalf("expected date-specific source-backed news lead, got %q", got)
	}
	if strings.Contains(firstLine, "Artificial Intelligence News") || strings.Contains(got, "Current request date:") {
		t.Fatalf("expected generic directory and raw request-date metadata hidden from the lead, got %q", got)
	}
}

func TestCompleteToolAnswerFiltersGenericAINewsDirectoryFallbacks(t *testing.T) {
	rag := []string{
		"### news_search\n" + unavailableToolMessage("news_search"),
		`### web_search
Current request date: Saturday, 4 July 2026 (2026-07-04, UTC).
Search results for "AI news":
Grounding rule: only use source-backed snippets.
Sources:
1. Artificial Intelligence News — Latest AI news and directory links. (https://artificialintelligence-news.com/)
2. AI News - Artificial Intelligence — Breaking news, analysis and category coverage. (https://www.yahoo.com/news/tag/artificial-intelligence/)
3. Artificial Intelligence | TechCrunch — AI startup and product coverage. (https://techcrunch.com/category/artificial-intelligence/)
4. Model lab ships safer assistant — The company announced a new model release today. (https://example.com/model-release)
5. Chip supplier expands AI capacity — Demand for inference hardware is rising this week. (https://example.com/chip-capacity)`,
	}

	got := completeToolAnswer("Here are the search results I found.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Model lab ships safer assistant; Chip supplier expands AI capacity") {
		t.Fatalf("expected story-like AI news lead after directory filtering, got %q", got)
	}
	for _, generic := range []string{"artificialintelligence-news.com", "yahoo.com/news/tag/artificial-intelligence", "techcrunch.com/category/artificial-intelligence", "Artificial Intelligence | TechCrunch"} {
		if strings.Contains(got, generic) {
			t.Fatalf("expected generic AI directory result %q to be filtered, got %q", generic, got)
		}
	}
}

func TestCompleteToolAnswerRepairsOperationalWeatherLead(t *testing.T) {
	rag := []string{`### weather_forecast
Current request date: Friday, 3 July 2026 (2026-07-03, UTC).
Weather for New York today.
Observation: Google Weather current conditions are available.
Now: 30°C, Sunny.
Forecast: Fri 2026-07-03: high 33°C, low 24°C.
Provider timestamp: 2026-07-03 12:00 UTC.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("- Observation: Google Weather data is available.\n- Provider timestamp: 2026-07-03 12:00 UTC.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 30°C, Sunny; today: Fri 2026-07-03: high 33°C, low 24°C.") {
		t.Fatalf("expected weather fallback to repair operational lead with current condition, got %q", got)
	}
	if strings.Contains(firstLine, "Observation:") || strings.Contains(firstLine, "Provider timestamp:") {
		t.Fatalf("expected operational context below the answer, got %q", got)
	}
}

func TestCompleteToolAnswerRepairsSearchResultLead(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. AI lab releases new model — Company announced a new assistant release. (https://example.com/ai-model)
2. Chip supplier expands AI capacity — Demand for AI chips is rising. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Search results for Top results:\n1. AI lab releases new model", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items:") || strings.Contains(firstLine, "Search results") || strings.Contains(firstLine, "Top results:") {
		t.Fatalf("expected search-result lead to be repaired into concise synthesis, got %q", got)
	}
	if !strings.Contains(got, "Unavailable right now: news.") {
		t.Fatalf("expected unavailable news disclosure to remain, got %q", got)
	}
}

func TestCompleteToolAnswerLeadsWeatherWithCurrentCondition(t *testing.T) {
	rag := []string{`### weather_forecast
Current request date: Friday, 3 July 2026 (2026-07-03, UTC).
Weather for New York today.
Now: 32°C, clear.
Forecast: Fri 2026-07-03: high 33°C, low 24°C.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("I'll check the weather now.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 32°C, clear; today: Fri 2026-07-03: high 33°C, low 24°C.") {
		t.Fatalf("expected weather fallback to lead with the current condition and today's forecast, got %q", got)
	}
	if !strings.Contains(got, "Weather for New York today") || !strings.Contains(got, "Freshness/source: Google Weather") {
		t.Fatalf("expected location and freshness context to remain below the answer, got %q", got)
	}
	if strings.Contains(firstLine, "Current request date:") || strings.Contains(firstLine, "Freshness/source:") {
		t.Fatalf("expected operational context below current condition, got %q", got)
	}
}

func TestCompleteToolAnswerLeadsWeatherWithTemperatureLabel(t *testing.T) {
	rag := []string{`### weather_forecast
Weather for New York today.
Observation: Google Weather current conditions are available.
Temperature: 30°C, sunny.
Today: high 33°C, low 24°C.
Provider: Google Weather.
Provider timestamp: 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("- Provider: Google Weather.\n- Provider timestamp: 2026-07-03 12:00 UTC.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 30°C, sunny; today: high 33°C, low 24°C.") {
		t.Fatalf("expected temperature-labelled weather to lead with current conditions, got %q", got)
	}
	if strings.Contains(firstLine, "Provider:") || strings.Contains(firstLine, "Provider timestamp:") || strings.Contains(firstLine, "Observation:") {
		t.Fatalf("expected provider metadata below the answer, got %q", got)
	}
}

func TestCompleteToolAnswerRepairsProviderPrefaceWeatherLead(t *testing.T) {
	rag := []string{`### weather_forecast
Weather for New York today.
Provider: Google Weather.
Provider timestamp: 2026-07-03 12:00 UTC.
Feels like: 31°C, sunny.
High: 33°C, low 24°C.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("As of provider timestamp 2026-07-03 12:00 UTC, Google Weather has data for New York.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 31°C, sunny; today: 33°C, low 24°C.") {
		t.Fatalf("expected provider-prefaced weather answer to be repaired into current conditions, got %q", got)
	}
	if strings.Contains(firstLine, "Provider") || strings.Contains(firstLine, "timestamp") {
		t.Fatalf("expected provider metadata below the answer lead, got %q", got)
	}
}

func TestCompleteToolAnswerSkipsMarketSectionLabels(t *testing.T) {
	rag := []string{`### markets
Current request date: Friday, 3 July 2026 (2026-07-03, UTC).
Live crypto prices:
BTC: $97000.00 (+1.23% 24h)
ETH: $3500.00 (-0.42% 24h)
Last refresh: 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("Let me pull the latest market data.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "BTC: $97000.00") {
		t.Fatalf("expected market fallback to lead with prices, got %q", got)
	}
	if strings.Contains(got, "Live crypto prices:") || strings.Contains(firstLine, "Current request date:") {
		t.Fatalf("expected market section labels and request context not to lead, got %q", got)
	}
}

func TestCompleteToolAnswerMovesMarketsFreshnessAfterPrices(t *testing.T) {
	rag := []string{`### markets
Last refresh: 2026-07-03 12:00 UTC.
Disclosure: market data may be stale.
Live crypto prices:
BTC: $97000.00 (+1.23% 24h)
ETH: $3500.00 (-0.42% 24h)`}
	got := completeToolAnswer("Let me pull the latest market data.", rag)
	btc := strings.Index(got, "BTC: $97000.00")
	freshness := strings.Index(got, "Last refresh:")
	if btc < 0 || freshness < 0 || freshness < btc {
		t.Fatalf("expected market fallback to lead with prices and keep freshness later, got %q", got)
	}
	if strings.HasPrefix(got, "- Last refresh:") {
		t.Fatalf("expected first bullet to answer the market prompt, got %q", got)
	}
}

func TestCompleteToolAnswerRemovesWebResultNumbering(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. AI lab releases new model — Company announced a new assistant release. (https://example.com/ai-model)
2. Chip supplier expands AI capacity — Demand for AI chips is rising. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Let me search the web for more AI stories.", rag)
	if strings.Contains(got, "- 1. AI lab") || strings.Contains(got, "- 2. Chip") {
		t.Fatalf("expected web fallback to read like synthesized bullets, not numbered search results, got %q", got)
	}
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items:") || strings.Contains(firstLine, "Search results") || strings.Contains(firstLine, "Top results:") {
		t.Fatalf("expected web fallback to open with concise synthesis, got %q", got)
	}
	if !strings.Contains(got, "- AI lab releases new model") || !strings.Contains(got, "Unavailable right now: news.") {
		t.Fatalf("expected concise source-backed bullets and unavailable disclosure, got %q", got)
	}
}

func TestCompleteToolAnswerLeadsWeatherWithCurrentConditionsLabel(t *testing.T) {
	rag := []string{`### weather_forecast
Current request date: Friday, 3 July 2026 (2026-07-03, UTC).
Weather for New York today.
Current conditions: 30°C, sunny.
Today: high 33°C, low 24°C.
Provider timestamp: 2026-07-03 12:00 UTC.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("- Observation: Google Weather current conditions are available.\n- Provider timestamp: 2026-07-03 12:00 UTC.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 30°C, sunny; today: high 33°C, low 24°C.") {
		t.Fatalf("expected weather fallback to use current-conditions/today labels as the answer lead, got %q", got)
	}
	if strings.Contains(firstLine, "Observation:") || strings.Contains(firstLine, "Provider timestamp:") {
		t.Fatalf("expected operational context below current conditions, got %q", got)
	}
}

func TestCompleteToolAnswerTreatsObservationReadingAsCurrentCondition(t *testing.T) {
	rag := []string{`### weather_forecast
Weather for New York today.
Observation: 30°C, sunny.
Provider timestamp: 2026-07-03 12:00 UTC.
Today: high 33°C, low 24°C.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("- Observation: 30°C, sunny.\n- Provider timestamp: 2026-07-03 12:00 UTC.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 30°C, sunny; today: high 33°C, low 24°C.") {
		t.Fatalf("expected observation reading to become the current-condition answer lead, got %q", got)
	}
	if strings.Contains(firstLine, "Provider timestamp:") {
		t.Fatalf("expected provider timestamp below the answer lead, got %q", got)
	}
}

func TestCompleteToolAnswerRepairsGenericSearchResultIntro(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. AI lab releases new model — Company announced a new assistant release. (https://example.com/ai-model)
2. Chip supplier expands AI capacity — Demand for AI chips is rising. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Here are the search results I found:\n1. AI lab releases new model\n2. Chip supplier expands AI capacity", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items:") || strings.Contains(firstLine, "Here are") {
		t.Fatalf("expected generic search-result intro to be repaired into concise synthesis, got %q", got)
	}
	if strings.Contains(got, "- 1.") || !strings.Contains(got, "Unavailable right now: news.") {
		t.Fatalf("expected de-numbered source bullets and unavailable disclosure, got %q", got)
	}
}

func TestCompleteToolAnswerPrefersSnippetBackedNewsOverGenericWebPages(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-category)
2. AI archive — Archive of articles about AI. (https://example.com/ai-archive)
3. AI lab releases new model — Company announced a new assistant release today. (https://example.com/ai-model)
4. Chip supplier expands AI capacity — Demand for AI chips is rising this week. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Here are the search results I found.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items: AI lab releases new model; Chip supplier expands AI capacity.") {
		t.Fatalf("expected snippet-backed current news to lead over generic web pages, got %q", got)
	}
	if strings.Contains(firstLine, "Artificial Intelligence News") || strings.Contains(firstLine, "AI archive") {
		t.Fatalf("expected generic category pages moved below source-backed stories, got %q", got)
	}
	if !strings.Contains(got, "Unavailable right now: news.") {
		t.Fatalf("expected unavailable news disclosure to remain, got %q", got)
	}
}

func TestCompleteToolAnswerPrefersCurrentStoriesOverOfficialNewsPages(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. OpenAI News — Company news and announcements. (https://openai.com/news/)
2. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-category)
3. AI safety bill advances — Lawmakers moved the proposal after new model-risk hearings. (https://example.com/ai-bill)
4. Startup launches healthcare AI tool — The company says hospitals are piloting the assistant this week. (https://example.com/ai-health)`}
	got := completeToolAnswer("Here are some search results for AI news.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items: Startup launches healthcare AI tool; AI safety bill advances.") {
		t.Fatalf("expected current story snippets to lead over generic news pages, got %q", got)
	}
	if strings.Contains(firstLine, "OpenAI News") || strings.Contains(firstLine, "Artificial Intelligence News") {
		t.Fatalf("expected generic pages below the answer lead, got %q", got)
	}
}

func TestCompleteToolAnswerPrefersDatedNewsStories(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Current request date: Saturday, 4 July 2026 (2026-07-04, UTC)
Search results for "AI news":
Sources:
1. AI newsletter roundup — A broad collection of AI links and analysis. (https://example.com/ai-roundup)
2. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-directory)
3. AI lab releases new reasoning model — July 4, 2026: the company announced a model update for enterprise assistants. (https://example.com/ai-model)
4. Chipmaker signs AI supply deal — 2026-07-04: the supplier expanded capacity for accelerator customers. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Here are the search results I found.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items for Saturday, 4 July 2026 (2026-07-04): AI lab releases new reasoning model; Chipmaker signs AI supply deal.") {
		t.Fatalf("expected dated story sources to lead generic or undated AI pages, got %q", got)
	}
	if strings.Contains(firstLine, "AI newsletter roundup") || strings.Contains(firstLine, "Artificial Intelligence News") {
		t.Fatalf("expected dated stories above generic and undated pages, got %q", got)
	}
}

func TestCompleteToolAnswerFramesSnippetOnlyAINewsCategoryPageAsLimitedEvidence(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Current request date: Saturday, 4 July 2026 (2026-07-04, UTC).
Search results for "AI news":
Sources:
1. AI News, Updates, Products and Reviews | Yahoo Tech — Meta announced new AI glasses today with an on-device assistant for live translation. (https://www.yahoo.com/tech/tag/artificial-intelligence/)
2. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-category)`}

	got := completeToolAnswer("Here are the search results I found.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "limited source-backed evidence") {
		t.Fatalf("expected category-page snippet evidence to stay limited instead of becoming a lead story, got %q", got)
	}
	if strings.Contains(firstLine, "Latest source-backed items") || strings.Contains(firstLine, "Meta announced new AI glasses") {
		t.Fatalf("expected no latest-story lead from category-only evidence, got %q", got)
	}
}

func TestCompleteToolAnswerKeepsYahooAICategorySnippetLimitedAndUnduplicated(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Current request date: Sunday, 5 July 2026 (2026-07-05, UTC).
Search results for "AI news":
Sources:
1. AI News, Updates, Products and Reviews | Yahoo Tech — Meta announced new AI glasses today with an on-device assistant for live translation. (https://tech.yahoo.com/ai/)`}

	got := completeToolAnswer("Top results:\n1. AI News, Updates, Products and Reviews | Yahoo Tech", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "limited source-backed evidence") {
		t.Fatalf("expected Yahoo AI category fallback to use limited-evidence framing, got %q", got)
	}
	if strings.Contains(firstLine, "Latest source-backed items") {
		t.Fatalf("expected no promoted lead story from category URL evidence, got %q", got)
	}
	if strings.Contains(got, "Meta announced new AI glasses today with an on-device assistant for live translation — Meta announced new AI glasses today") {
		t.Fatalf("expected category snippet not to be duplicated around an em dash, got %q", got)
	}
}

func TestCompleteToolAnswerDisclosesLimitedWebEvidence(t *testing.T) {
	rag := []string{
		`### web_search
Confidence: low — generic category pages only.
1. AI category page — A broad archive of AI articles. (https://example.com/ai)`,
	}

	got := completeToolAnswer("I'll search the web.", rag)
	if !strings.Contains(got, "limited source-backed evidence") {
		t.Fatalf("expected limited-evidence disclosure, got %q", got)
	}
	if !strings.Contains(got, "AI category page") {
		t.Fatalf("expected available web result to remain visible, got %q", got)
	}
}

func TestCompleteToolAnswerKeepsLimitedEvidenceURLsReadable(t *testing.T) {
	longSnippet := strings.Repeat("detailed AI category context with broad archive evidence ", 8)
	rag := []string{
		`### web_search
Confidence: low — generic category pages only.
1. AI news archive — ` + longSnippet + `(https://www.yahoo.com/tech/tag/artificial-intelligence/)`,
	}

	got := completeToolAnswer("I'll search the web.", rag)
	if !strings.Contains(got, "limited source-backed evidence") {
		t.Fatalf("expected limited-evidence disclosure, got %q", got)
	}
	if !strings.Contains(got, "https://www.yahoo.com/tech/tag/artificial-intelligence/") {
		t.Fatalf("expected full usable URL in limited-evidence fallback, got %q", got)
	}
	if strings.Contains(got, "https://www.…") || strings.Contains(got, "https://www.yahoo.com/tech/tag/artificial-intelligence/…") {
		t.Fatalf("expected URL not to be ellipsized, got %q", got)
	}
}

func TestCompleteToolAnswerTreatsDirectoryOnlyAINewsAsLimitedEvidence(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Current request date: Sunday, 5 July 2026 (2026-07-05, UTC).
Search results for "AI news":
Sources:
1. AI | MIT News — News and campus articles tagged artificial intelligence. (https://news.mit.edu/topic/artificial-intelligence2)
2. AI Magazine — Artificial intelligence industry coverage and topic pages. (https://aimagazine.com/)
3. AI News, Updates, Products and Reviews | Yahoo Tech — The latest AI product coverage and reviews. (https://tech.yahoo.com/ai/)`}

	got := completeToolAnswer("Top results:\n1. AI | MIT News\n2. AI Magazine", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "limited source-backed evidence") {
		t.Fatalf("expected directory-only AI news fallback to use limited-evidence framing, got %q", got)
	}
	if strings.Contains(firstLine, "Latest source-backed items") {
		t.Fatalf("expected no latest-news lead from directory-only evidence, got %q", got)
	}
}

func TestCompleteToolAnswerStripsInlineHTMLFromFallbackBullets(t *testing.T) {
	rag := []string{`### web_search
Search results for "AI news":
Sources:
1. <strong>AI roundup</strong> — <strong><strong>OpenAI released a model update today</strong></strong> for enterprise assistants. (https://example.com/ai-roundup)
2. Chip supplier expands AI capacity — Demand for AI chips is rising this week. (https://example.com/ai-chips)`}

	got := completeToolAnswer("Here are the search results I found.", rag)
	if strings.Contains(got, "<strong>") || strings.Contains(got, "</strong>") {
		t.Fatalf("expected fallback bullets to strip inline strong markup, got %q", got)
	}
	if !strings.Contains(got, "OpenAI released a model update today") {
		t.Fatalf("expected cleaned snippet-backed story to remain, got %q", got)
	}
}

func TestCompleteToolAnswerReplacesRawMixedSourcePayload(t *testing.T) {
	rag := []string{
		`### blog_list
Recent blog posts:
1. Mu daily note — Agent reliability improved. (/blog/mu-note)`,
		"### social\n" + unavailableToolMessage("social"),
		`### web_search
Web results for "mu agent reliability":
Sources:
1. Reliability patterns for agents — Fallbacks should produce readable summaries. (https://example.com/agents)`,
	}
	raw := `{"results":[{"title":"Reliability patterns for agents","url":"https://example.com/agents"}],"error":"social unavailable","status":"partial"}`

	got := completeToolAnswer(raw, rag)
	if strings.Contains(got, `{"results"`) || strings.Contains(got, `"status":"partial"`) {
		t.Fatalf("expected raw payload to be replaced, got %q", got)
	}
	if !strings.Contains(got, "Mu daily note") || !strings.Contains(got, "https://example.com/agents") {
		t.Fatalf("expected available mixed-source context in fallback, got %q", got)
	}
	if !strings.Contains(got, "Unavailable right now: social.") {
		t.Fatalf("expected unavailable source disclosure, got %q", got)
	}
}

func TestCompleteToolAnswerRepairsLookUpProgressNarration(t *testing.T) {
	rag := []string{`### web_search
Search results for "AI news":
Sources:
1. AI lab releases new model — Company announced a new assistant release today. (https://example.com/ai-model)
2. Chip supplier expands AI capacity — Demand for AI chips is rising this week. (https://example.com/ai-chips)`}

	got := completeToolAnswer("I'll look up the latest AI news now.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if strings.Contains(strings.ToLower(got), "look up") {
		t.Fatalf("expected lookup progress narration to be replaced, got %q", got)
	}
	if !strings.Contains(firstLine, "Latest source-backed items: AI lab releases new model; Chip supplier expands AI capacity.") {
		t.Fatalf("expected source-backed web fallback answer, got %q", got)
	}
}

func TestFormatToolResultNewsHeadlinesStaysReadable(t *testing.T) {
	result := "Latest headlines — 1 across 1 topics.\n\n[tech] AI lab releases a new model — Useful summary (source: Example, url: https://example.com/ai)"
	got := formatToolResult("news_headlines", result, nil)
	if !strings.Contains(got, "Current request date:") {
		t.Fatalf("expected current date context, got %q", got)
	}
	if !strings.Contains(got, "AI lab releases a new model") || strings.Contains(got, `{"`) {
		t.Fatalf("expected readable news_headlines context, got %q", got)
	}
}

func TestFormatToolResultNewsSearchUnwrapsNativeServiceText(t *testing.T) {
	inner := `{"query":"AI news","freshness":{"status":"stale","requested_date":"2026-07-07","freshest_posted_at":"2026-05-20T12:00:00Z","notice":"No same-day news_search results were available for 2026-07-07; the freshest result is from 2026-05-20, so lead with a freshness caveat instead of presenting older stories as today's news."},"results":[{"title":"AI startup raises funding","description":"Archive context.","category":"Tech","url":"https://example.com/old-ai","posted_at":"2026-05-20T12:00:00Z"}]}`
	result := `{"text":` + quoteJSONString(inner) + `}`
	got := formatToolResult("news_search", result, nil)
	if !strings.Contains(got, "Freshness caveat: No same-day news_search results were available for 2026-07-07") {
		t.Fatalf("expected native service text wrapper freshness caveat to surface, got %q", got)
	}
	if strings.Index(got, "Freshness caveat:") > strings.Index(got, "AI startup raises funding") {
		t.Fatalf("expected freshness caveat before stale story, got %q", got)
	}
}

func TestFormatToolResultNewsSearchSurfacesFreshnessCaveat(t *testing.T) {
	result := `{"query":"AI news","freshness":{"status":"stale","requested_date":"2026-07-05","freshest_posted_at":"2026-05-20T12:00:00Z","notice":"No same-day news_search results were available for 2026-07-05; the freshest result is from 2026-05-20, so lead with a freshness caveat instead of presenting older stories as today's news."},"results":[{"title":"AI startup raises funding","description":"Artificial intelligence funding story from the archive.","category":"Tech","url":"https://example.com/old-ai","posted_at":"2026-05-20T12:00:00Z"}]}`
	got := formatToolResult("news_search", result, nil)
	firstCaveat := strings.Index(got, "Freshness caveat:")
	firstStory := strings.Index(got, "1. AI startup raises funding")
	if firstCaveat < 0 {
		t.Fatalf("expected freshness caveat in news_search context, got %q", got)
	}
	if firstStory < 0 || firstStory < firstCaveat {
		t.Fatalf("expected freshness caveat before older fallback stories, got %q", got)
	}
}

func TestFormatToolResultNewsSearchSurfacesMostlyStaleFreshnessCaveat(t *testing.T) {
	result := `{"query":"AI news","freshness":{"status":"mostly_stale","requested_date":"2026-07-05","same_day_results":1,"dated_results":3,"freshest_posted_at":"2026-07-05T12:00:00Z","notice":"Only 1 of 3 dated news_search results are from 2026-07-05; lead with a freshness caveat before listing older context as today's news."},"results":[{"title":"AI lab ships today's assistant update","description":"Current AI story.","category":"Tech","url":"https://example.com/current-ai","posted_at":"2026-07-05T12:00:00Z"},{"title":"AI startup raises funding","description":"Archive story.","category":"Tech","url":"https://example.com/old-ai","posted_at":"2026-05-20T12:00:00Z"}]}`
	got := formatToolResult("news_search", result, nil)
	firstCaveat := strings.Index(got, "Freshness caveat:")
	firstStory := strings.Index(got, "1. AI lab ships today's assistant update")
	if firstCaveat < 0 || !strings.Contains(got, "Only 1 of 3 dated news_search results") {
		t.Fatalf("expected mostly-stale freshness caveat in news_search context, got %q", got)
	}
	if firstStory < 0 || firstStory < firstCaveat {
		t.Fatalf("expected mostly-stale caveat before story list, got %q", got)
	}
}

func TestCompleteToolAnswerPrependsStaleNewsCaveatToSubstantiveAnswer(t *testing.T) {
	rag := []string{`### news_search
News results for "AI news":
Freshness caveat: No same-day news_search results were available for 2026-07-06; the freshest result is from 2026-05-20, so lead with a freshness caveat before older items.
1. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`}
	answer := "1. AI startup raises funding — an older archive item from May."
	got := completeToolAnswer(answer, rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "No current news_search results:") || !strings.Contains(firstLine, "No same-day news_search results were available for 2026-07-06") {
		t.Fatalf("expected stale-only disclosure before story list, got %q", got)
	}
	if strings.Index(got, "No current news_search results:") > strings.Index(got, "Background: AI startup raises funding") {
		t.Fatalf("expected stale-only disclosure to precede background-labeled stories, got %q", got)
	}
	if !strings.Contains(got, "Background: AI startup raises funding") {
		t.Fatalf("expected stale-only substantive answer stories to be labeled as background, got %q", got)
	}
}

func TestCompleteToolAnswerPrependsMostlyStaleNewsCaveatToSubstantiveAnswer(t *testing.T) {
	rag := []string{`### news_search
News results for "AI news":
Freshness caveat: Only 1 of 3 dated news_search results are from 2026-07-07; lead with a freshness caveat before listing older context as today's news.
1. AI lab ships current update (category: Tech; posted: 07 Jul 2026 12:00 UTC; source: https://example.com/current-ai) — Current story.
2. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`}
	answer := "1. AI lab ships current update — current story.\n2. AI startup raises funding — older archive context."
	got := completeToolAnswer(answer, rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Mostly stale news_search results:") || !strings.Contains(firstLine, "Only 1 of 3 dated news_search results") {
		t.Fatalf("expected mostly-stale disclosure before story list, got %q", got)
	}
	if strings.Index(got, "Mostly stale news_search results:") > strings.Index(got, "Background: AI lab ships current update") {
		t.Fatalf("expected mostly-stale disclosure to precede background-labeled stories, got %q", got)
	}
}

func TestCompleteToolAnswerLabelsExistingStaleNewsCaveatStories(t *testing.T) {
	rag := []string{`### news_search
News results for "AI news":
Freshness caveat: No same-day news_search results were available for 2026-07-06; the freshest result is from 2026-05-20, so lead with a freshness caveat before older items.
1. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`}
	answer := "No same-day news_search results were available for 2026-07-06. Older items only:\n- 1. AI startup raises funding — an archive item from May."
	got := completeToolAnswer(answer, rag)
	if strings.Count(got, "No current news_search results:") != 0 {
		t.Fatalf("did not expect duplicate no-current caveat, got %q", got)
	}
	if !strings.Contains(got, "- Background: 1. AI startup raises funding") {
		t.Fatalf("expected existing caveat answer stories to be labeled as background, got %q", got)
	}
}

func TestRenderNewsCardSurfacesMostlyStaleFreshnessCaveat(t *testing.T) {
	result := `{"query":"AI news","freshness":{"status":"mostly_stale","notice":"Only 1 of 3 dated news_search results are from 2026-07-07; lead with a freshness caveat before listing older context as today's news."},"results":[{"title":"AI lab ships current update","category":"Tech","url":"https://example.com/current-ai"}]}`
	got := renderNewsCard(result)
	if !strings.Contains(got, "Mostly stale news_search results: Only 1 of 3 dated news_search results") {
		t.Fatalf("expected mostly-stale caveat in news card html, got %q", got)
	}
	if strings.Index(got, "Mostly stale news_search results:") > strings.Index(got, "AI lab ships current update") {
		t.Fatalf("expected news card caveat before story links, got %q", got)
	}
}

func TestSynthesizeToolFallbackLeadsStaleNewsWithCaveat(t *testing.T) {
	rag := []string{`### news_search
News results for "AI news":
Freshness caveat: No same-day news_search results were available for 2026-07-06; the freshest result is from 2026-05-20, so lead with a freshness caveat before older items.
1. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`}
	got := synthesizeToolFallback(rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "No current news_search results:") {
		t.Fatalf("expected stale-only disclosure to lead fallback answer, got %q", got)
	}
	if strings.Contains(got, "- News results for") {
		t.Fatalf("did not expect tool-style news heading in fallback answer, got %q", got)
	}
	if !strings.Contains(got, "Background: AI startup raises funding") {
		t.Fatalf("expected older stale stories to be labeled as background, got %q", got)
	}
}

func TestShouldBufferNativeTokenForStaleNewsCaveat(t *testing.T) {
	recorder := newNativeToolRecorder()
	if shouldBufferNativeToken(recorder) {
		t.Fatal("empty recorder should not buffer native tokens")
	}
	recorder.parts = append(recorder.parts, `### news
News results for "AI news":
Freshness caveat: No same-day news_search results were available for 2026-07-06; the freshest result is from 2026-05-20, so lead with a freshness caveat before older items.
1. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`)
	if !shouldBufferNativeToken(recorder) {
		t.Fatal("stale-only news recorder should buffer native tokens until the guarded final response")
	}
}

func TestShouldReplayFinalNativeAnswerForBufferedLatestNews(t *testing.T) {
	if !shouldReplayFinalNativeAnswer("Find today's AI news", []string{"news"}, 0) {
		t.Fatal("expected buffered native latest-news answers to replay guarded final text")
	}
	if !shouldReplayFinalNativeAnswer("Weather in New York today", []string{"weather"}, 12) {
		t.Fatal("expected captured native text to be replayed")
	}
	if shouldReplayFinalNativeAnswer("Weather in New York today", []string{"weather"}, 0) {
		t.Fatal("did not expect unrelated empty native captures to replay")
	}
}

func TestShouldHoldNativeNewsStreamTokensForLatestNewsPrompt(t *testing.T) {
	if !shouldHoldNativeNewsStreamTokens("Find today's AI news", nil) {
		t.Fatal("expected native latest-news tokens to be held before the news tool result is available")
	}
	if !shouldHoldNativeNewsStreamTokens("Find today's AI news", []string{"news"}) {
		t.Fatal("expected native latest-news tokens to be held once news tool starts")
	}
	if !shouldHoldNativeNewsStreamTokens("Find today's AI news", []string{"📰 Scanning headlines"}) {
		t.Fatal("expected user-facing native news labels to hold latest-news tokens")
	}
	if shouldHoldNativeNewsStreamTokens("Find today's AI news", []string{"weather"}) {
		t.Fatal("did not expect non-news native tool tokens to be held")
	}
	if shouldHoldNativeNewsStreamTokens("What is moving in markets?", []string{"news"}) {
		t.Fatal("did not expect non-latest-news prompts to hold native tokens")
	}
}

func TestCompleteToolAnswerReplaysSortedFreshnessFallbackWhenAnswerLeadsStale(t *testing.T) {
	rag := []string{`### news_search
News results for "AI news":
Freshness caveat: Only 1 of 3 dated news_search results are from 2026-07-08; lead with a freshness caveat before listing older context as today's news.
1. AI lab ships current update (category: Tech; posted: 8 Jul 2026 12:00 UTC; source: https://example.com/current-ai) — Current story.
2. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`}
	answer := "AI startup raises funding in May. AI lab ships current update today."
	got := completeToolAnswer(answer, rag)
	if !strings.HasPrefix(got, "- Mostly stale news_search results:") {
		t.Fatalf("expected guarded fallback caveat to lead replayed answer, got %q", got)
	}
	current := strings.Index(got, "Background: AI lab ships current update")
	old := strings.Index(got, "Background: AI startup raises funding")
	if current < 0 || old < 0 || old < current {
		t.Fatalf("expected replayed news context to keep current item ahead of stale background, got %q", got)
	}
}

func TestCompleteToolAnswerDoesNotDuplicateLeadingStaleNewsCaveat(t *testing.T) {
	rag := []string{`### news_search
News results for "AI news":
Freshness caveat: No same-day news_search results were available for 2026-07-06; the freshest result is from 2026-05-20, so lead with a freshness caveat before older items.
1. AI startup raises funding (category: Tech; posted: 20 May 2026 12:00 UTC; source: https://example.com/old-ai) — Archive story.`}
	answer := "No same-day news_search results were available for 2026-07-06. The freshest item is older."
	got := completeToolAnswer(answer, rag)
	if got != answer {
		t.Fatalf("expected existing leading caveat to be preserved without duplication, got %q", got)
	}
}

func TestCompleteToolAnswerKeepsSubstantiveAnswer(t *testing.T) {
	want := "BTC is at $100,000, and the latest news says it reached a new high."
	got := completeToolAnswer(want, []string{"### markets\nBTC: $100,000"})
	if got != want {
		t.Fatalf("expected substantive answer unchanged, got %q", got)
	}
}

func TestCompleteToolAnswerWithoutToolsDoesNotOverride(t *testing.T) {
	want := "Let me think about that."
	got := completeToolAnswer(want, nil)
	if got != want {
		t.Fatalf("expected no override without tool results, got %q", got)
	}
}

func TestNativeToolRecorderFeedsProgressFallback(t *testing.T) {
	recorder := newNativeToolRecorder()
	handler := recorder.wrap(func(_ context.Context, call gmai.ToolCall) gmai.ToolResult {
		return gmai.ToolResult{Content: "{\"summary\":\"Weather for London. Now: 14C, light rain.\"}"}
	})

	handler(context.Background(), gmai.ToolCall{Name: "weather_Forecast"})
	got := completeToolAnswer("Let me pull the latest for you.", recorder.ragParts())
	if strings.Contains(strings.ToLower(got), "let me pull") {
		t.Fatalf("expected weather tool payload to replace progress narration, got %q", got)
	}
	if strings.Contains(got, "**weather**") || !strings.Contains(got, "14C") {
		t.Fatalf("expected fallback to include weather result without an implementation heading, got %q", got)
	}
}

func TestNativeToolRecorderFormatsNewsPayloads(t *testing.T) {
	recorder := newNativeToolRecorder()
	handler := recorder.wrap(func(_ context.Context, call gmai.ToolCall) gmai.ToolResult {
		return gmai.ToolResult{Content: `{"query":"AI","results":[{"title":"AI headline","description":"Useful context","category":"tech","url":"/news?id=1"}],"count":1}`}
	})

	handler(context.Background(), gmai.ToolCall{Name: "news_Search"})
	got := completeToolAnswer("Let me search the web for more AI stories to round this out.", recorder.ragParts())
	if strings.Contains(strings.ToLower(got), "let me search") {
		t.Fatalf("expected native progress narration to be replaced, got %q", got)
	}
	if !strings.Contains(got, "AI headline") || !strings.Contains(got, "Useful context") || strings.Contains(got, `{"query"`) {
		t.Fatalf("expected native fallback to use formatted news results, got %q", got)
	}
}

func TestNativeToolRecorderFormatsDottedNewsSearchPayloads(t *testing.T) {
	recorder := newNativeToolRecorder()
	handler := recorder.wrap(func(_ context.Context, call gmai.ToolCall) gmai.ToolResult {
		return gmai.ToolResult{Content: `{"query":"AI news","freshness":{"status":"stale","notice":"No same-day news_search results were available for 2026-07-07; the freshest result is from 2026-05-20, so lead with a freshness caveat instead of presenting older stories as today's news."},"results":[{"title":"AI startup raises funding","description":"Archive context","category":"Tech","url":"https://example.com/old-ai","posted_at":"2026-05-20T12:00:00Z"}]}`}
	})

	handler(context.Background(), gmai.ToolCall{Name: "news.Search"})
	got := completeToolAnswer("1. AI startup raises funding — Archive context.", recorder.ragParts())
	if !strings.Contains(got, "No current news_search results: No same-day news_search results were available for 2026-07-07") {
		t.Fatalf("expected dotted native news tool name to surface stale caveat, got %q", got)
	}
	if strings.Index(got, "No current news_search results:") > strings.Index(got, "Background: AI startup raises funding") {
		t.Fatalf("expected stale caveat to precede old story, got %q", got)
	}
	if strings.Contains(got, `{"query"`) {
		t.Fatalf("expected dotted native news payload to be formatted, got %q", got)
	}
}

func TestCompleteNativeToolAnswerReplacesProgressWithUnavailableState(t *testing.T) {
	got := completeNativeToolAnswer("I'll check the latest market and news data now.", []string{"📈 Checking market prices", "📰 Scanning headlines"})
	if strings.Contains(strings.ToLower(got), "i'll check") {
		t.Fatalf("expected progress narration to be replaced, got %q", got)
	}
	if !strings.Contains(got, "couldn't produce a complete final answer") {
		t.Fatalf("expected clear unavailable state, got %q", got)
	}
}

func TestCompleteNativeToolAnswerKeepsProgressWithoutTools(t *testing.T) {
	want := "I'll check what that means."
	got := completeNativeToolAnswer(want, nil)
	if got != want {
		t.Fatalf("expected no override without native tool use, got %q", got)
	}
}

func TestCompleteToolAnswerRepairsDeployedWeatherObservedLead(t *testing.T) {
	rag := []string{`### weather_forecast
Current request date: Friday, 3 July 2026 (2026-07-03, UTC).
Weather for New York today.
Current conditions: 30°C, sunny.
Today: high 33°C, low 24°C.
Provider timestamp: 2026-07-03 12:00 UTC.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("- Current conditions observed via Google Weather at 2026-07-03 12:00 UTC.\n- Provider timestamp: 2026-07-03 12:00 UTC.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 30°C, sunny; today: high 33°C, low 24°C.") {
		t.Fatalf("expected deployed observed-conditions lead to be repaired into weather answer, got %q", got)
	}
	if strings.Contains(firstLine, "Current conditions observed") || strings.Contains(firstLine, "Provider timestamp:") {
		t.Fatalf("expected operational observation context below the direct answer, got %q", got)
	}
}

func TestCompleteToolAnswerAvoidsGenericTopResultsNewsLead(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-category)
2. AI lab releases new model — Company announced a new assistant release today. (https://example.com/ai-model)
3. Chip supplier expands AI capacity — Demand for AI chips is rising this week. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Here are the search results I found.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items: AI lab releases new model; Chip supplier expands AI capacity.") {
		t.Fatalf("expected concise source-backed news lead, got %q", got)
	}
	if strings.Contains(firstLine, "Top results") || strings.Contains(firstLine, "Artificial Intelligence News") {
		t.Fatalf("expected no generic top-results/category-page lead, got %q", got)
	}
}

func TestCompleteToolAnswerPrefersArticlesOverLiveAINewsCategoryPages(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Current request date: Sunday, 5 July 2026 (2026-07-05, UTC).
Search results for "AI news":
Sources:
1. AI (artificial intelligence) | The Guardian — Coverage of AI policy and product launches from around the world. (https://www.theguardian.com/technology/artificialintelligenceai)
2. Artificial Intelligence News | Bloomberg — Latest artificial intelligence stories, analysis and market coverage. (https://www.bloomberg.com/technology/ai)
3. Robotics startup launches home assistant — The company launched a household AI robot today after a new funding round. (https://example.com/2026/07/05/robotics-startup-home-assistant)
4. Lab releases compact AI model — July 5, 2026: researchers released a smaller model for on-device assistants. (https://example.com/2026/07/05/compact-ai-model)`}
	got := completeToolAnswer("Top results:\n1. AI (artificial intelligence) | The Guardian\n2. Artificial Intelligence News | Bloomberg", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items for Sunday, 5 July 2026 (2026-07-05): Robotics startup launches home assistant; Lab releases compact AI model.") {
		t.Fatalf("expected article-level AI news sources in the lead, got %q", got)
	}
	for _, generic := range []string{"The Guardian", "Bloomberg", "theguardian.com/technology/artificialintelligenceai", "bloomberg.com/technology/ai"} {
		if strings.Contains(got, generic) {
			t.Fatalf("expected generic AI category page %q to be filtered, got %q", generic, got)
		}
	}
}

func TestCompleteToolAnswerPrefersArticlesOverYahooAndReutersAINewsCategoryPages(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Current request date: Sunday, 5 July 2026 (2026-07-05, UTC).
Search results for "AI news":
Sources:
1. AI News, Updates, Products and Reviews | Yahoo Tech — The latest AI product coverage and reviews. (https://tech.yahoo.com/ai/)
2. Artificial Intelligence | Reuters — The latest artificial intelligence news and analysis. (https://www.reuters.com/technology/artificial-intelligence/)
3. Robotics startup launches home assistant — The company launched a household AI robot today after a new funding round. (https://example.com/2026/07/05/robotics-startup-home-assistant)
4. Lab releases compact AI model — July 5, 2026: researchers released a smaller model for on-device assistants. (https://example.com/2026/07/05/compact-ai-model)`}

	got := completeToolAnswer("Top results:\n1. AI News, Updates, Products and Reviews | Yahoo Tech\n2. Artificial Intelligence | Reuters", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items for Sunday, 5 July 2026 (2026-07-05): Robotics startup launches home assistant; Lab releases compact AI model.") {
		t.Fatalf("expected article-level AI news sources in the lead, got %q", got)
	}
	for _, generic := range []string{"Yahoo Tech", "Reuters", "tech.yahoo.com/ai", "reuters.com/technology/artificial-intelligence"} {
		if strings.Contains(got, generic) {
			t.Fatalf("expected generic AI category page %q to be filtered, got %q", generic, got)
		}
	}
}

func TestCompleteToolAnswerRepairsObservedAtWeatherLead(t *testing.T) {
	rag := []string{`### weather_forecast
Current request date: Friday, 3 July 2026 (2026-07-03, UTC).
Weather for New York today.
Current conditions: 30°C, sunny.
Today: high 33°C, low 24°C.
Observed at 2026-07-03 12:00 UTC via Google Weather.
Freshness/source: Google Weather; generated at 2026-07-03 12:00 UTC.`}
	got := completeToolAnswer("Observed at 2026-07-03 12:00 UTC via Google Weather. Forecast metadata follows.", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Right now: 30°C, sunny; today: high 33°C, low 24°C.") {
		t.Fatalf("expected observed-at weather lead to be repaired into current conditions, got %q", got)
	}
	if strings.Contains(firstLine, "Observed at") || strings.Contains(firstLine, "Google Weather") {
		t.Fatalf("expected observation metadata below the answer lead, got %q", got)
	}
}

func TestCompleteToolAnswerRepairsTopResultsLead(t *testing.T) {
	rag := []string{`### news_search
` + unavailableToolMessage("news_search"), `### web_search
Search results for "AI news":
Sources:
1. Artificial Intelligence News — Latest news and headlines about artificial intelligence. (https://example.com/ai-category)
2. AI lab releases new model — Company announced a new assistant release today. (https://example.com/ai-model)
3. Chip supplier expands AI capacity — Demand for AI chips is rising this week. (https://example.com/ai-chips)`}
	got := completeToolAnswer("Top results:\n1. Artificial Intelligence News\n2. AI lab releases new model", rag)
	firstLine, _, _ := strings.Cut(got, "\n")
	if !strings.Contains(firstLine, "Latest source-backed items: AI lab releases new model; Chip supplier expands AI capacity.") {
		t.Fatalf("expected top-results lead to be repaired into source-backed stories, got %q", got)
	}
	if strings.Contains(firstLine, "Top results") || strings.Contains(firstLine, "Artificial Intelligence News") {
		t.Fatalf("expected generic top-results/category-page lead to move below the direct answer, got %q", got)
	}
}
