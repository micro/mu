package agent

import (
	"strings"
	"testing"
	"time"
)

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

func TestFormatMarketsResult_WithResults(t *testing.T) {
	result := `{"category":"crypto","data":[{"symbol":"BTC","price":97000.12,"change_24h":1.23,"type":"crypto"},{"symbol":"ETH","price":3456.78,"change_24h":-0.45,"type":"crypto"}]}`
	got := formatMarketsResult(result)
	if !strings.Contains(got, "BTC") {
		t.Errorf("expected 'BTC' in output, got %q", got)
	}
	if !strings.Contains(got, "97000.12") {
		t.Errorf("expected BTC price in output, got %q", got)
	}
	if !strings.Contains(got, "ETH") {
		t.Errorf("expected 'ETH' in output, got %q", got)
	}
	if !strings.Contains(got, "+1.23%") {
		t.Errorf("expected positive change in output, got %q", got)
	}
	if !strings.Contains(got, "-0.45%") {
		t.Errorf("expected negative change in output, got %q", got)
	}
	if !strings.Contains(got, "crypto") {
		t.Errorf("expected category in output, got %q", got)
	}
}

func TestFormatMarketsResult_EmptyData(t *testing.T) {
	result := `{"category":"crypto","data":[]}`
	got := formatMarketsResult(result)
	if got != "No market data available." {
		t.Errorf("expected 'No market data available.', got %q", got)
	}
}

func TestFormatMarketsResult_InvalidJSON(t *testing.T) {
	result := `not json`
	got := formatMarketsResult(result)
	if got != result {
		t.Errorf("expected original result as fallback, got %q", got)
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

func TestFormatWeatherResult_WithData(t *testing.T) {
	result := `{"forecast":{"Location":"London","Current":{"TempC":15.0,"FeelsLikeC":13.0,"Description":"Partly cloudy","Humidity":65,"WindKph":20.0},"DailyItems":[{"MaxTempC":18.0,"MinTempC":12.0,"Description":"Cloudy","WillRain":true,"RainMM":2.5}]}}`
	got := formatWeatherResult(result)
	if !strings.Contains(got, "London") {
		t.Errorf("expected location, got %q", got)
	}
	if !strings.Contains(got, "15") {
		t.Errorf("expected temperature, got %q", got)
	}
	if !strings.Contains(got, "Partly cloudy") {
		t.Errorf("expected description, got %q", got)
	}
	if !strings.Contains(got, "65") {
		t.Errorf("expected humidity, got %q", got)
	}
	if !strings.Contains(got, "18") {
		t.Errorf("expected max temp in forecast, got %q", got)
	}
}

func TestFormatWeatherResult_Empty(t *testing.T) {
	result := `{"forecast":{"Location":"","Current":{"TempC":0,"Description":""},"DailyItems":[]}}`
	got := formatWeatherResult(result)
	if got != "Weather data unavailable." {
		t.Errorf("expected 'Weather data unavailable.', got %q", got)
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

func TestFormatBlogResult_WithPosts(t *testing.T) {
	result := `[{"title":"My first post","author":"alice","tags":"tech,golang","content":"This is a blog post about Go programming.","created_at":"2025-01-01T00:00:00Z"},{"title":"Second post","author":"bob","tags":"news","content":"Another post here.","created_at":"2025-01-02T00:00:00Z"}]`
	got := formatBlogResult(result)
	if !strings.Contains(got, "Recent blog posts") {
		t.Errorf("expected header, got %q", got)
	}
	if !strings.Contains(got, "My first post") {
		t.Errorf("expected post title, got %q", got)
	}
	if !strings.Contains(got, "alice") {
		t.Errorf("expected author, got %q", got)
	}
	if !strings.Contains(got, "tech,golang") {
		t.Errorf("expected tags, got %q", got)
	}
}

func TestFormatBlogResult_Empty(t *testing.T) {
	result := `[]`
	got := formatBlogResult(result)
	if got != "No blog posts available." {
		t.Errorf("expected 'No blog posts available.', got %q", got)
	}
}

func TestFormatWebSearchResult_WithResults(t *testing.T) {
	result := `{"query":"bitcoin price","results":[{"title":"Bitcoin Price Today","url":"https://example.com","snippet":"BTC is trading at $97,000"},{"title":"Crypto markets","url":"https://crypto.com","snippet":"Latest crypto prices"}]}`
	got := formatWebSearchResult(result)
	if !strings.Contains(got, "bitcoin price") {
		t.Errorf("expected query in header, got %q", got)
	}
	if !strings.Contains(got, "Bitcoin Price Today") {
		t.Errorf("expected result title, got %q", got)
	}
	if !strings.Contains(got, "BTC is trading") {
		t.Errorf("expected snippet, got %q", got)
	}
}

func TestFormatWebSearchResult_Empty(t *testing.T) {
	result := `{"results":[],"query":"nothing"}`
	got := formatWebSearchResult(result)
	if got != "No web results found." {
		t.Errorf("expected 'No web results found.', got %q", got)
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
		got := formatAge(c.d)
		if got != c.want {
			t.Errorf("formatAge(%v) = %q, want %q", c.d, got, c.want)
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

	got := listFlows("user-1")
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
