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

// chartLinks maps asset symbols to their chart URLs
var chartLinks = map[string]string{
	// Crypto → CoinGecko charts
	"BTC":   "https://www.coingecko.com/en/coins/bitcoin",
	"ETH":   "https://www.coingecko.com/en/coins/ethereum",
	"UNI":   "https://www.coingecko.com/en/coins/uniswap",
	"PAXG":  "https://www.coingecko.com/en/coins/pax-gold",
	"SOL":   "https://www.coingecko.com/en/coins/solana",
	"ADA":   "https://www.coingecko.com/en/coins/cardano",
	"DOT":   "https://www.coingecko.com/en/coins/polkadot",
	"LINK":  "https://www.coingecko.com/en/coins/chainlink",
	"MATIC": "https://www.coingecko.com/en/coins/polygon",
	"AVAX":  "https://www.coingecko.com/en/coins/avalanche",
	// Futures/Commodities → Yahoo Finance charts
	"OIL":      "https://finance.yahoo.com/chart/CL%3DF",
	"GOLD":     "https://finance.yahoo.com/chart/GC%3DF",
	"SILVER":   "https://finance.yahoo.com/chart/SI%3DF",
	"COPPER":   "https://finance.yahoo.com/chart/HG%3DF",
	"COFFEE":   "https://finance.yahoo.com/chart/KC%3DF",
	"WHEAT":    "https://finance.yahoo.com/chart/KE%3DF",
	"CORN":     "https://finance.yahoo.com/chart/ZC%3DF",
	"SOYBEANS": "https://finance.yahoo.com/chart/ZS%3DF",
	"OATS":     "https://finance.yahoo.com/chart/ZO%3DF",
}

// MarketData represents market data for display
type MarketData struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h"`
	Type      string  `json:"type"`
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
	priceData := widgets.GetAllPriceData()

	var data []MarketData
	assets := getAssetsForCategory(category)

	for _, symbol := range assets {
		pd, ok := priceData[symbol]
		if !ok {
			pd.Price = 0
		}
		data = append(data, MarketData{
			Symbol:    symbol,
			Price:     pd.Price,
			Change24h: pd.Change24h,
			Type:      category,
		})
	}

	app.RespondJSON(w, map[string]interface{}{
		"category": category,
		"data":     data,
	})
}

// handleHTML returns market data as HTML page
func handleHTML(w http.ResponseWriter, r *http.Request, category string) {
	priceData := widgets.GetAllPriceData()

	// Generate HTML for the selected category
	body := generateMarketsPage(priceData, category)

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
func generateMarketsPage(priceData map[string]widgets.PriceData, activeCategory string) string {
	var sb strings.Builder

	// Page header
	sb.WriteString(`<div class="markets-page">`)
	sb.WriteString(`<p class="description">Live market data for cryptocurrencies, futures, and commodities</p>`)

	// Category tabs
	sb.WriteString(`<div class="markets-tabs">`)
	sb.WriteString(generateTab("Crypto", CategoryCrypto, activeCategory))
	sb.WriteString(generateTab("Futures", CategoryFutures, activeCategory))
	sb.WriteString(generateTab("Commodities", CategoryCommodities, activeCategory))
	sb.WriteString(`</div>`)

	// Market data table
	sb.WriteString(`<table class="markets-table">`)
	sb.WriteString(`<thead><tr><th>Symbol</th><th>Price</th><th>24h Change</th><th>Chart</th></tr></thead>`)
	sb.WriteString(`<tbody>`)

	assets := getAssetsForCategory(activeCategory)

	// Sort assets alphabetically
	sort.Strings(assets)

	for _, symbol := range assets {
		pd := priceData[symbol]
		sb.WriteString(generateMarketRow(symbol, pd.Price, pd.Change24h))
	}

	sb.WriteString(`</tbody></table>`)

	// Data source information
	sb.WriteString(`<div class="markets-footer">`)
	sb.WriteString(`<p class="markets-source">Data sources: Coinbase, CoinGecko, Yahoo Finance</p>`)
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

// generateMarketRow generates HTML for a single market table row
func generateMarketRow(symbol string, price, change24h float64) string {
	priceStr := formatPrice(price)
	changeStr, changeClass := formatChange(change24h)

	chartLink := chartLinks[symbol]
	chartHTML := ""
	if chartLink != "" {
		chartHTML = fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="markets-chart-link">Chart ↗</a>`, chartLink)
	}

	return fmt.Sprintf(`<tr>
		<td class="markets-symbol">%s</td>
		<td class="markets-price">%s</td>
		<td class="markets-change %s">%s</td>
		<td>%s</td>
	</tr>`, symbol, priceStr, changeClass, changeStr, chartHTML)
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

// formatChange formats a 24h change percentage for display, returning the string and CSS class
func formatChange(change float64) (string, string) {
	if change == 0 {
		return "—", "markets-change-neutral"
	}
	if change > 0 {
		return fmt.Sprintf("+%.2f%%", change), "markets-change-up"
	}
	return fmt.Sprintf("%.2f%%", change), "markets-change-down"
}
