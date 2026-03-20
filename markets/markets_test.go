package markets

import (
	"strings"
	"testing"
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
	if !strings.Contains(html, "market-grid") {
		t.Error("expected market-grid class")
	}
	if !strings.Contains(html, "BTC") {
		t.Error("expected BTC in output")
	}
}

func TestGetAllPrices_ReturnsDefensiveCopy(t *testing.T) {
	marketsMutex.Lock()
	cachedPrices = map[string]float64{"BTC": 97000}
	marketsMutex.Unlock()

	prices := GetAllPrices()
	prices["BTC"] = 0 // Modify the copy

	marketsMutex.RLock()
	original := cachedPrices["BTC"]
	marketsMutex.RUnlock()

	if original != 97000 {
		t.Error("modifying returned map should not affect cache")
	}
}

func TestGetAllPriceData_ReturnsDefensiveCopy(t *testing.T) {
	marketsMutex.Lock()
	cachedPriceData = map[string]PriceData{
		"ETH": {Price: 3500, Change24h: 1.5},
	}
	cachedPrices = map[string]float64{"ETH": 3500, "BTC": 97000}
	marketsMutex.Unlock()

	data := GetAllPriceData()
	if data["ETH"].Price != 3500 {
		t.Errorf("expected ETH price 3500, got %v", data["ETH"].Price)
	}
	// BTC should fall back to plain price
	if data["BTC"].Price != 97000 {
		t.Errorf("expected BTC fallback price 97000, got %v", data["BTC"].Price)
	}
}

func TestMarketsHTML(t *testing.T) {
	marketsMutex.Lock()
	marketsHTML = "<div>test html</div>"
	marketsMutex.Unlock()

	got := MarketsHTML()
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
}
