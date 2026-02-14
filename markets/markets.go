package markets

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mu/app"
	"mu/widgets"
)

// Categories for market data
const (
	CategoryCrypto      = "crypto"
	CategoryFutures     = "futures"
	CategoryCommodities = "commodities"
)

// Crypto assets to display
var cryptoAssets = []string{"BTC", "ETH", "UNI", "PAXG", "SOL", "ADA", "DOT", "LINK", "MATIC", "AVAX"}

// Futures/Commodities to display
var futuresAssets = []string{"OIL", "GOLD", "SILVER", "COPPER"}
var commoditiesAssets = []string{"COFFEE", "WHEAT", "CORN", "SOYBEANS", "OATS"}

// MarketData represents market data for display
type MarketData struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	Type   string  `json:"type"`
}

// Handler handles /markets requests
func Handler(w http.ResponseWriter, r *http.Request) {
	// Get current category from query param, default to crypto
	category := r.URL.Query().Get("category")
	if category == "" {
		category = CategoryCrypto
	}

	// Validate category
	if category != CategoryCrypto && category != CategoryFutures && category != CategoryCommodities {
		category = CategoryCrypto
	}

	// JSON response for API
	if app.WantsJSON(r) {
		handleJSON(w, r, category)
		return
	}

	// HTML response for browser
	handleHTML(w, r, category)
}

// handleJSON returns market data as JSON
func handleJSON(w http.ResponseWriter, r *http.Request, category string) {
	prices := widgets.GetAllPrices()
	
	var data []MarketData
	assets := getAssetsForCategory(category)
	
	for _, symbol := range assets {
		if price, ok := prices[symbol]; ok {
			data = append(data, MarketData{
				Symbol: symbol,
				Price:  price,
				Type:   category,
			})
		}
	}

	app.RespondJSON(w, map[string]interface{}{
		"category": category,
		"data":     data,
	})
}

// handleHTML returns market data as HTML page
func handleHTML(w http.ResponseWriter, r *http.Request, category string) {
	prices := widgets.GetAllPrices()
	
	// Generate HTML for the selected category
	body := generateMarketsPage(prices, category)
	
	app.Respond(w, r, app.Response{
		Title:       "Markets",
		Description: "Live cryptocurrency, futures, and commodity market prices",
		HTML:        body,
	})
}

// getAssetsForCategory returns the list of assets for a given category
func getAssetsForCategory(category string) []string {
	switch category {
	case CategoryFutures:
		return futuresAssets
	case CategoryCommodities:
		return commoditiesAssets
	default:
		return cryptoAssets
	}
}

// generateMarketsPage generates the full markets page HTML
func generateMarketsPage(prices map[string]float64, activeCategory string) string {
	var sb strings.Builder
	
	// Page header
	sb.WriteString(`<div class="markets-page">`)
	sb.WriteString(`<h1>Markets</h1>`)
	sb.WriteString(`<p class="description">Live market data for cryptocurrencies, futures, and commodities</p>`)
	
	// Category tabs
	sb.WriteString(`<div class="markets-tabs">`)
	sb.WriteString(generateTab("Crypto", CategoryCrypto, activeCategory))
	sb.WriteString(generateTab("Futures", CategoryFutures, activeCategory))
	sb.WriteString(generateTab("Commodities", CategoryCommodities, activeCategory))
	sb.WriteString(`</div>`)
	
	// Market data grid
	sb.WriteString(`<div class="markets-grid">`)
	assets := getAssetsForCategory(activeCategory)
	
	// Sort assets alphabetically
	sort.Strings(assets)
	
	for _, symbol := range assets {
		price, ok := prices[symbol]
		if !ok {
			price = 0
		}
		
		sb.WriteString(generateMarketCard(symbol, price))
	}
	
	sb.WriteString(`</div>`)
	
	// Data source information
	sb.WriteString(`<div class="markets-footer">`)
	sb.WriteString(`<p class="markets-source">Data sources: Coinbase, Yahoo Finance</p>`)
	sb.WriteString(`<p class="markets-note">Prices update hourly. For real-time trading, visit official exchanges.</p>`)
	sb.WriteString(`</div>`)
	
	sb.WriteString(`</div>`)
	
	return sb.String()
}

// generateTab generates HTML for a category tab
func generateTab(label, category, activeCategory string) string {
	activeClass := ""
	if category == activeCategory {
		activeClass = " active"
	}
	return fmt.Sprintf(`<a href="/markets?category=%s" class="markets-tab%s">%s</a>`, 
		category, activeClass, label)
}

// generateMarketCard generates HTML for a single market item
func generateMarketCard(symbol string, price float64) string {
	priceStr := formatPrice(price)
	
	return fmt.Sprintf(`
		<div class="market-card">
			<div class="market-card-header">
				<span class="market-symbol">%s</span>
			</div>
			<div class="market-card-body">
				<span class="market-price">%s</span>
			</div>
		</div>`, symbol, priceStr)
}

// formatPrice formats a price value for display
func formatPrice(price float64) string {
	if price <= 0 {
		return "N/A"
	}
	
	// Format based on price magnitude
	if price >= 1 {
		return fmt.Sprintf("$%.2f", price)
	} else if price >= 0.01 {
		return fmt.Sprintf("$%.4f", price)
	} else {
		return fmt.Sprintf("$%.6f", price)
	}
}
