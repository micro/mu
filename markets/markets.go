package markets

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/data"

	"github.com/piquette/finance-go/future"
)

// PriceData holds price and 24h change for an asset
type PriceData struct {
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h"`
}

var (
	marketsMutex    sync.RWMutex
	marketsHTML     string
	cachedPrices    map[string]float64
	cachedPriceData map[string]PriceData
)

// cryptoGeckoIDs maps ticker symbols to CoinGecko asset IDs
var cryptoGeckoIDs = map[string]string{
	"BTC":   "bitcoin",
	"ETH":   "ethereum",
	"UNI":   "uniswap",
	"PAXG":  "pax-gold",
	"SOL":   "solana",
	"ADA":   "cardano",
	"DOT":   "polkadot",
	"LINK":  "chainlink",
	"POL":   "polygon-ecosystem-token",
	"AVAX":  "avalanche-2",
}

var tickers = []string{"GBP", "UNI", "ETH", "BTC", "PAXG"}

var futuresSymbols = map[string]string{
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

// Load initializes the markets data
func Load() {
	// Load cached prices
	b, err := data.LoadFile("prices.json")
	if err == nil {
		var prices map[string]float64
		if json.Unmarshal(b, &prices) == nil {
			marketsMutex.Lock()
			cachedPrices = prices
			marketsHTML = generateMarketsCardHTML(prices)
			marketsMutex.Unlock()
		}
	}

	// Load cached price data (with 24h changes)
	b, err = data.LoadFile("price_data.json")
	if err == nil {
		var pd map[string]PriceData
		if json.Unmarshal(b, &pd) == nil {
			marketsMutex.Lock()
			cachedPriceData = pd
			marketsMutex.Unlock()
		}
	}

	// Load cached HTML
	b, err = data.LoadFile("markets.html")
	if err == nil {
		marketsMutex.Lock()
		marketsHTML = string(b)
		marketsMutex.Unlock()
	}

	// Start background refresh
	go refreshMarkets()
}

func refreshMarkets() {
	for {
		prices, priceData := fetchPrices()
		if prices != nil {
			marketsMutex.Lock()
			cachedPrices = prices
			cachedPriceData = priceData
			marketsHTML = generateMarketsCardHTML(prices)
			marketsMutex.Unlock()

			indexMarketPrices(prices)
			data.SaveFile("markets.html", marketsHTML)
			data.SaveJSON("prices.json", cachedPrices)
			data.SaveJSON("price_data.json", cachedPriceData)
		}

		time.Sleep(time.Hour)
	}
}

func fetchPrices() (map[string]float64, map[string]PriceData) {
	app.Log("markets", "Fetching prices")

	rsp, err := http.Get("https://api.coinbase.com/v2/exchange-rates?currency=USD")
	if err != nil {
		app.Log("markets", "Error getting crypto prices: %v", err)
		return nil, nil
	}
	defer rsp.Body.Close()

	b, _ := ioutil.ReadAll(rsp.Body)
	var res map[string]interface{}
	json.Unmarshal(b, &res)
	if res == nil {
		return nil, nil
	}

	rates := res["data"].(map[string]interface{})["rates"].(map[string]interface{})
	prices := map[string]float64{}
	priceData := map[string]PriceData{}

	for k, t := range rates {
		val, err := strconv.ParseFloat(t.(string), 64)
		if err != nil {
			continue
		}
		prices[k] = 1 / val
	}

	// Fetch 24h changes from CoinGecko for crypto assets
	app.Log("markets", "Fetching 24h changes from CoinGecko")
	geckoChanges := fetchCoinGeckoChanges()
	for symbol, geckoID := range cryptoGeckoIDs {
		if price, ok := prices[symbol]; ok {
			pd := PriceData{Price: price}
			if change, ok := geckoChanges[geckoID]; ok {
				pd.Change24h = change
			}
			priceData[symbol] = pd
		}
	}

	// Get futures prices
	app.Log("markets", "Fetching futures prices")
	for key, ftr := range futuresSymbols {
		func() {
			defer func() {
				if r := recover(); r != nil {
					app.Log("markets", "Panic getting future %s: %v", key, r)
				}
			}()

			f, err := future.Get(ftr)
			if err != nil {
				app.Log("markets", "Failed to get future %s: %v", key, err)
				return
			}
			if f == nil {
				return
			}
			price := f.Quote.RegularMarketPrice
			if price > 0 {
				prices[key] = price
				priceData[key] = PriceData{
					Price:     price,
					Change24h: f.Quote.RegularMarketChangePercent,
				}
			}
		}()
	}

	app.Log("markets", "Finished fetching prices")
	return prices, priceData
}

// fetchCoinGeckoChanges fetches 24h price changes from CoinGecko for all crypto assets
func fetchCoinGeckoChanges() map[string]float64 {
	ids := make([]string, 0, len(cryptoGeckoIDs))
	for _, id := range cryptoGeckoIDs {
		ids = append(ids, id)
	}
	url := "https://api.coingecko.com/api/v3/simple/price?ids=" + strings.Join(ids, ",") +
		"&vs_currencies=usd&include_24hr_change=true"

	rsp, err := http.Get(url)
	if err != nil {
		app.Log("markets", "Error getting CoinGecko data: %v", err)
		return nil
	}
	defer rsp.Body.Close()

	b, _ := ioutil.ReadAll(rsp.Body)
	var result map[string]map[string]float64
	if err := json.Unmarshal(b, &result); err != nil {
		app.Log("markets", "Error parsing CoinGecko data: %v", err)
		return nil
	}

	changes := map[string]float64{}
	for geckoID, data := range result {
		if change, ok := data["usd_24h_change"]; ok {
			changes[geckoID] = change
		}
	}
	return changes
}

func generateMarketsCardHTML(prices map[string]float64) string {
	var sb strings.Builder
	sb.WriteString(`<div class="market-grid">`)

	allTickers := append([]string{}, tickers...)
	allTickers = append(allTickers, futuresKeys...)
	sort.Slice(allTickers, func(i, j int) bool {
		if len(allTickers[i]) != len(allTickers[j]) {
			return len(allTickers[i]) < len(allTickers[j])
		}
		return allTickers[i] < allTickers[j]
	})

	for _, ticker := range allTickers {
		price := prices[ticker]
		fmt.Fprintf(&sb, `<div class="market-item"><span class="market-symbol">%s</span><span class="market-price">$%.2f</span></div>`, ticker, price)
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

func indexMarketPrices(prices map[string]float64) {
	app.Log("markets", "Indexing %d prices", len(prices))
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

// MarketsHTML returns the rendered markets card HTML
func MarketsHTML() string {
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()
	return marketsHTML
}

// GetAllPrices returns all cached prices
func GetAllPrices() map[string]float64 {
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()

	result := make(map[string]float64)
	for k, v := range cachedPrices {
		result[k] = v
	}
	return result
}

// GetAllPriceData returns all cached price data including 24h changes
func GetAllPriceData() map[string]PriceData {
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()

	result := make(map[string]PriceData)
	for k, v := range cachedPriceData {
		result[k] = v
	}
	// Fall back to plain prices for any symbol not in priceData
	for k, price := range cachedPrices {
		if _, ok := result[k]; !ok {
			result[k] = PriceData{Price: price}
		}
	}
	return result
}

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
	priceData := GetAllPriceData()

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
	priceData := GetAllPriceData()

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
func generateMarketsPage(priceData map[string]PriceData, activeCategory string) string {
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
