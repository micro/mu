package markets

import (
	"strings"
	"testing"
	"time"
)

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		price    float64
		expected string
	}{
		{0, "N/A"},
		{-1, "N/A"},
		{97000.12, "$97000.12"},
		{1.50, "$1.50"},
		{0.05, "$0.0500"},
		{0.001, "$0.001000"},
	}
	for _, tt := range tests {
		got := formatPrice(tt.price)
		if got != tt.expected {
			t.Errorf("formatPrice(%v) = %q, want %q", tt.price, got, tt.expected)
		}
	}
}

func TestFormatChange(t *testing.T) {
	tests := []struct {
		change    float64
		wantStr   string
		wantClass string
	}{
		{0, "—", "markets-change-neutral"},
		{1.23, "+1.23%", "markets-change-up"},
		{-0.45, "-0.45%", "markets-change-down"},
	}
	for _, tt := range tests {
		str, class := formatChange(tt.change)
		if str != tt.wantStr {
			t.Errorf("formatChange(%v) str = %q, want %q", tt.change, str, tt.wantStr)
		}
		if class != tt.wantClass {
			t.Errorf("formatChange(%v) class = %q, want %q", tt.change, class, tt.wantClass)
		}
	}
}

func TestGetAssetsForCategory(t *testing.T) {
	tests := []struct {
		category string
		contains string
	}{
		{CategoryCrypto, "BTC"},
		{CategoryFutures, "OIL"},
		{CategoryCommodities, "COFFEE"},
		{CategoryCurrencies, "EUR"},
		{CategoryStocks, "TSLA"},
		{"invalid", "BTC"}, // defaults to crypto
	}
	for _, tt := range tests {
		assets := getAssetsForCategory(tt.category)
		found := false
		for _, a := range assets {
			if a == tt.contains {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("getAssetsForCategory(%q) should contain %q", tt.category, tt.contains)
		}
	}
}

func TestGenerateTab(t *testing.T) {
	active := generateTab("Crypto", CategoryCrypto, CategoryCrypto)
	if !strings.Contains(active, "active") {
		t.Error("expected active class for matching category")
	}
	if !strings.Contains(active, "Crypto") {
		t.Error("expected label")
	}

	inactive := generateTab("Futures", CategoryFutures, CategoryCrypto)
	if strings.Contains(inactive, "active") {
		t.Error("should not have active class for non-matching category")
	}
}

func TestGenerateMarketRow(t *testing.T) {
	row := generateMarketRow("BTC", 97000.50, 1.23)
	if !strings.Contains(row, "BTC") {
		t.Error("expected symbol")
	}
	if !strings.Contains(row, "$97000.50") {
		t.Error("expected price")
	}
	if !strings.Contains(row, "+1.23%") {
		t.Error("expected positive change")
	}
	if !strings.Contains(row, "markets-change-up") {
		t.Error("expected up class")
	}
}

func TestGenerateMarketRow_WithChart(t *testing.T) {
	row := generateMarketRow("BTC", 97000, 0)
	if !strings.Contains(row, "Chart ↗") {
		t.Error("expected chart link for known symbol")
	}
	if !strings.Contains(row, "coingecko.com") {
		t.Error("expected CoinGecko chart link for BTC")
	}
}

func TestGenerateMarketsCardHTML(t *testing.T) {
	prices := map[string]float64{
		"BTC":  97000,
		"ETH":  3500,
		"GOLD": 2000,
	}
	html := generateMarketsCardHTML(prices)
	if !strings.Contains(html, "<table") {
		t.Error("expected table element")
	}
	if !strings.Contains(html, "BTC") {
		t.Error("expected BTC in output")
	}
}

func TestGetAllPrices_ReturnsDefensiveCopy(t *testing.T) {
	marketsMutex.Lock()
	cachedPrices = map[string]float64{"BTC": 97000}
	marketsMutex.Unlock()

	prices := AllPrices()
	prices["BTC"] = 0 // Modify the copy

	marketsMutex.RLock()
	original := cachedPrices["BTC"]
	marketsMutex.RUnlock()

	if original != 97000 {
		t.Error("modifying returned map should not affect cache")
	}
}

func TestParseCoinbaseRates(t *testing.T) {
	rates, err := parseCoinbaseRates([]byte(`{"data":{"rates":{"BTC":"0.000010","ETH":"0.000300"}}}`))
	if err != nil {
		t.Fatalf("parseCoinbaseRates returned error: %v", err)
	}
	if rates["BTC"] != "0.000010" {
		t.Fatalf("BTC rate = %q, want %q", rates["BTC"], "0.000010")
	}
}

func TestParseCoinbaseRatesRejectsMalformedPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{`},
		{name: "missing data", body: `{}`},
		{name: "missing rates", body: `{"data":{}}`},
		{name: "wrong rate shape", body: `{"data":{"rates":[]}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCoinbaseRates([]byte(tt.body)); err == nil {
				t.Fatalf("parseCoinbaseRates(%s) returned nil error", tt.body)
			}
		})
	}
}

func TestGetAllPriceData_ReturnsDefensiveCopy(t *testing.T) {
	marketsMutex.Lock()
	cachedPriceData = map[string]PriceData{
		"ETH": {Price: 3500, Change24h: 1.5},
	}
	cachedPrices = map[string]float64{"ETH": 3500, "BTC": 97000}
	marketsMutex.Unlock()

	data := AllPriceData()
	if data["ETH"].Price != 3500 {
		t.Errorf("expected ETH price 3500, got %v", data["ETH"].Price)
	}
	// BTC should fall back to plain price
	if data["BTC"].Price != 97000 {
		t.Errorf("expected BTC fallback price 97000, got %v", data["BTC"].Price)
	}
}

func TestMarketsTextIncludesFreshnessDisclosure(t *testing.T) {
	updated := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	marketsMutex.Lock()
	cachedPriceData = map[string]PriceData{
		"BTC": {Price: 97000, Change24h: 1.5, UpdatedAt: updated, Source: "Coinbase + CoinGecko"},
	}
	cachedPrices = nil
	lastPriceRefresh = updated
	marketsMutex.Unlock()

	got := Text(CategoryCrypto)
	// How old, in words, because that is what a reader can act on — an
	// absolute UTC stamp makes them do the subtraction, and an instance was
	// found serving a price hours out with nobody noticing.
	if !strings.Contains(got, "Updated ") || !strings.Contains(got, "12:00 UTC") {
		t.Fatalf("expected freshness disclosure, got %q", got)
	}
	// And it says so plainly when it is out of date, rather than leaving the
	// reader to judge a timestamp.
	if !strings.Contains(got, "older than it should be") {
		t.Fatalf("a month-old price did not say it was stale, got %q", got)
	}
	if !strings.Contains(got, "some symbols are unavailable") {
		t.Fatalf("expected partial-data disclosure, got %q", got)
	}
}

func TestGenerateMarketsPageIncludesFreshnessDisclosure(t *testing.T) {
	updated := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	priceData := map[string]PriceData{
		"BTC": {Price: 97000, UpdatedAt: updated, Source: "Coinbase + CoinGecko"},
		"ETH": {Price: 3500, UpdatedAt: updated, Source: "Coinbase + CoinGecko"},
	}

	html := generateMarketsPage(priceData, CategoryCrypto, "")
	if !strings.Contains(html, "Updated ") || !strings.Contains(html, "12:00 UTC") {
		t.Fatalf("expected last-refresh metadata in markets page, got %q", html)
	}
	if !strings.Contains(html, "older than it should be") {
		t.Fatalf("a month-old price did not say it was stale in the page")
	}
	if !strings.Contains(html, "some symbols are unavailable") {
		t.Fatalf("expected partial-source disclosure in markets page, got %q", html)
	}
}

func TestMarketsHTML(t *testing.T) {
	marketsMutex.Lock()
	marketsHTML = "<div>test html</div>"
	marketsMutex.Unlock()

	got := HTML()
	if got != "<div>test html</div>" {
		t.Errorf("expected cached HTML, got %q", got)
	}
}

func TestCategoryConstants(t *testing.T) {
	if CategoryCrypto != "crypto" {
		t.Error("unexpected crypto constant")
	}
	if CategoryFutures != "futures" {
		t.Error("unexpected futures constant")
	}
	if CategoryCommodities != "commodities" {
		t.Error("unexpected commodities constant")
	}
	if CategoryCurrencies != "currencies" {
		t.Error("unexpected currencies constant")
	}
	if CategoryStocks != "stocks" {
		t.Error("unexpected stocks constant")
	}
}

// Every tracked stock needs a name, a chart and something fetching it. A row
// that says only "AVGO" with no price and no chart is worse than no row: the
// three lists are separate, so nothing but a test keeps them together.
func TestEveryStockIsCompletelyDescribed(t *testing.T) {
	if len(stockSymbols) == 0 {
		t.Fatal("no stocks tracked")
	}
	for _, symbol := range stockSymbols {
		if stockNames[symbol] == "" {
			t.Errorf("%s has no company name", symbol)
		}
		if chartLinks[symbol] == "" {
			t.Errorf("%s has no chart link", symbol)
		}
	}
	if len(stockAssets) != len(stockSymbols) {
		t.Errorf("the page shows %d stocks but only %d are fetched", len(stockAssets), len(stockSymbols))
	}
}

// A ticker only reads as a company to someone who already knows it.
func TestStockRowNamesTheCompany(t *testing.T) {
	row := generateMarketRow("TSLA", 412.5, -1.2)
	if !strings.Contains(row, "TSLA") {
		t.Error("expected the ticker")
	}
	if !strings.Contains(row, "Tesla") {
		t.Errorf("expected the company name alongside the ticker, got %q", row)
	}

	// Symbols that are their own name stay bare.
	if got := generateMarketRow("BTC", 97000, 0); strings.Contains(got, "markets-name") {
		t.Errorf("BTC should not carry a company name, got %q", got)
	}
}

// Stocks are only reachable if the page offers them.
func TestMarketsPageOffersStocks(t *testing.T) {
	html := generateMarketsPage(map[string]PriceData{
		"AAPL": {Price: 230.10, Change24h: 0.8, UpdatedAt: time.Now().UTC()},
	}, CategoryStocks, "")

	if !strings.Contains(html, `href="/markets?category=stocks"`) {
		t.Error("expected a Stocks tab")
	}
	if !strings.Contains(html, "Apple") {
		t.Error("expected the stocks category to render its rows")
	}
}

// The tool has to accept what the page accepts, or an agent asked for stock
// prices quietly gets crypto instead.
func TestMarketsTextAcceptsStocks(t *testing.T) {
	marketsMutex.Lock()
	cachedPriceData = map[string]PriceData{
		"TSLA": {Price: 412.5, Change24h: -1.2, UpdatedAt: time.Now().UTC(), Source: "Yahoo Finance"},
	}
	cachedPrices = nil
	marketsMutex.Unlock()

	got := Text(CategoryStocks)
	if !strings.Contains(got, "TSLA (Tesla)") {
		t.Errorf("expected the labelled stock line, got %q", got)
	}
	if strings.Contains(got, "BTC") {
		t.Errorf("asking for stocks returned crypto: %q", got)
	}
}

// Stale is relative to how often prices are meant to refresh.
//
// It was a flat two hours against an hourly refresh, so a price could be an
// hour and fifty-nine minutes old and still call itself current. That is how an
// instance came to serve BTC 2% out and ETH 3% out with nothing on the page
// saying anything was wrong.
func TestStaleIsRelativeToTheRefreshInterval(t *testing.T) {
	fresh := map[string]PriceData{
		"BTC": {Price: 1, UpdatedAt: time.Now().UTC().Add(-refreshEvery / 2)},
	}
	if _, stale, _ := marketsFreshness(fresh, []string{"BTC"}); stale {
		t.Error("a price newer than one interval called itself stale")
	}

	old := map[string]PriceData{
		"BTC": {Price: 1, UpdatedAt: time.Now().UTC().Add(-10 * refreshEvery)},
	}
	if _, stale, _ := marketsFreshness(old, []string{"BTC"}); !stale {
		t.Errorf("a price ten intervals old did not call itself stale")
	}

	// The interval itself has to stay short enough for crypto to be worth
	// showing. An hour is not: the 24h change alone moves several points.
	if refreshEvery > 15*time.Minute {
		t.Errorf("prices refresh every %s, which is too slow to put a crypto price on a page", refreshEvery)
	}
}
