package widgets

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
	Price    float64 `json:"price"`
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
	"MATIC": "matic-network",
	"AVAX":  "avalanche-2",
}

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
			marketsHTML = generateMarketsHTML(prices)
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
				priceData[key] = PriceData{
					Price:    price,
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
