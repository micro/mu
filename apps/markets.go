package apps

import (
	"context"
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
	"mu/tools"

	"github.com/piquette/finance-go/future"
)

var (
	marketsMutex  sync.RWMutex
	marketsHTML   string
	cachedPrices  map[string]float64
)

var tickers = []string{"GBP", "UNI", "ETH", "BTC", "PAXG"}

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

// LoadMarkets initializes the markets data
func LoadMarkets() {
	// Register tools
	RegisterMarketTools()

	// Load cached prices
	b, err := data.LoadFile("prices.json")
	if err == nil {
		var prices map[string]float64
		if json.Unmarshal(b, &prices) == nil {
			marketsMutex.Lock()
			cachedPrices = prices
			marketsHTML = generateMarketsHTML(prices)
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
		prices := fetchPrices()
		if prices != nil {
			marketsMutex.Lock()
			cachedPrices = prices
			marketsHTML = generateMarketsHTML(prices)
			marketsMutex.Unlock()

			indexMarketPrices(prices)
			data.SaveFile("markets.html", marketsHTML)
			data.SaveJSON("prices.json", cachedPrices)
		}

		time.Sleep(time.Hour)
	}
}

func fetchPrices() map[string]float64 {
	app.Log("markets", "Fetching prices")
	
	rsp, err := http.Get("https://api.coinbase.com/v2/exchange-rates?currency=USD")
	if err != nil {
		app.Log("markets", "Error getting crypto prices: %v", err)
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

	// Get futures prices
	app.Log("markets", "Fetching futures prices")
	for key, ftr := range futures {
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
			}
		}()
	}

	app.Log("markets", "Finished fetching prices")
	return prices
}

func generateMarketsHTML(prices map[string]float64) string {
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

// GetTickers returns the crypto tickers
func GetTickers() []string {
	return append([]string{}, tickers...)
}

// GetFuturesKeys returns the futures keys
func GetFuturesKeys() []string {
	return append([]string{}, futuresKeys...)
}

// RegisterTools registers market tools with the tools registry
func RegisterMarketTools() {
	tools.Register(tools.Tool{
		Name:        "markets.get_price",
		Description: "Get current price for a crypto or commodity ticker (BTC, ETH, GOLD, OIL, etc.)",
		Category:    "markets",
		Path:        "/api/markets/price",
		Method:      "GET",
		Input: map[string]tools.Param{
			"symbol": {Type: "string", Description: "Ticker symbol (BTC, ETH, GOLD, OIL, WHEAT, etc.)", Required: true},
		},
		Output: map[string]tools.Param{
			"symbol": {Type: "string", Description: "The ticker symbol"},
			"price":  {Type: "number", Description: "Current price in USD"},
		},
		Handler: handleGetPrice,
	})

	tools.Register(tools.Tool{
		Name:        "markets.list",
		Description: "List all available market tickers with their current prices",
		Category:    "markets",
		Path:        "/api/markets/list",
		Method:      "GET",
		Output: map[string]tools.Param{
			"prices": {Type: "object", Description: "Map of ticker symbols to prices"},
			"count":  {Type: "number", Description: "Number of tickers"},
		},
		Handler: handleListPrices,
	})
}

func handleGetPrice(ctx context.Context, params map[string]any) (any, error) {
	symbol, _ := params["symbol"].(string)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	prices := GetAllPrices()
	symbol = strings.ToUpper(symbol)

	// Try exact match first
	if price, ok := prices[symbol]; ok {
		return map[string]any{"symbol": symbol, "price": price}, nil
	}

	// Try partial match
	for k, v := range prices {
		if strings.Contains(strings.ToUpper(k), symbol) {
			return map[string]any{"symbol": k, "price": v}, nil
		}
	}

	return nil, fmt.Errorf("price not found for %s", symbol)
}

func handleListPrices(ctx context.Context, params map[string]any) (any, error) {
	prices := GetAllPrices()
	return map[string]any{
		"prices": prices,
		"count":  len(prices),
	}, nil
}
