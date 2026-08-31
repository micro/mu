package markets

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/service"
	"mu/internal/snapshot"

	"github.com/piquette/finance-go/future"
	"github.com/piquette/finance-go/quote"
)

// cardSnap is the go-micro read-plane channel for the markets card (store +
// broker); see internal/snapshot.
var cardSnap *snapshot.Snapshot

// PriceData holds price and 24h change for an asset
type PriceData struct {
	Price     float64   `json:"price"`
	Change24h float64   `json:"change_24h"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Source    string    `json:"source,omitempty"`
}

var (
	marketsMutex     sync.RWMutex
	marketsHTML      string
	cachedPrices     map[string]float64
	cachedPriceData  map[string]PriceData
	lastPriceRefresh time.Time
)

// cryptoGeckoIDs maps ticker symbols to CoinGecko asset IDs
var cryptoGeckoIDs = map[string]string{
	"BTC":  "bitcoin",
	"ETH":  "ethereum",
	"UNI":  "uniswap",
	"PAXG": "pax-gold",
	"SOL":  "solana",
	"ADA":  "cardano",
	"DOT":  "polkadot",
	"LINK": "chainlink",
	"POL":  "polygon-ecosystem-token",
	"AVAX": "avalanche-2",
}

var tickers = []string{"UNI", "ETH", "BTC", "PAXG"}

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

// stockSymbols are the companies most people mean when they ask about "the
// market" — the ones that turn up in a news headline and then in a question
// about whether the headline moved anything.
//
// The ticker is the key and the Yahoo symbol, because for ordinary US equities
// they are the same string; futures and forex need a mapping only because their
// symbols carry suffixes. Kept to ten: a card people read at a glance beats a
// table nobody finishes, and anything not here is one web_search away.
var stockSymbols = []string{
	"AAPL",  // Apple
	"MSFT",  // Microsoft
	"NVDA",  // Nvidia
	"GOOGL", // Alphabet
	"AMZN",  // Amazon
	"META",  // Meta
	"TSLA",  // Tesla
	"AVGO",  // Broadcom
	"NFLX",  // Netflix
	"AMD",   // AMD
}

// stockNames give a ticker the name a person would say, for the card and for
// what the agent reads back.
var stockNames = map[string]string{
	"AAPL": "Apple", "MSFT": "Microsoft", "NVDA": "Nvidia",
	"GOOGL": "Alphabet", "AMZN": "Amazon", "META": "Meta",
	"TSLA": "Tesla", "AVGO": "Broadcom", "NFLX": "Netflix", "AMD": "AMD",
}

// Load initializes the markets data
func Load() {
	// Register the go-micro service.
	if err := service.Register(Spec); err != nil {
		app.Log("markets", "service register failed: %v", err)
	}

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
			lastPriceRefresh = latestPriceUpdate(pd)
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

	// Read plane: start the snapshot channel, then warm the mirror with the
	// disk-primed card so renders are served from the go-micro data plane.
	cardSnap = snapshot.New("markets")
	marketsMutex.RLock()
	warm := marketsHTML
	marketsMutex.RUnlock()
	cardSnap.Publish(warm)

	// Start background refresh
	go refreshMarkets()
}

// TopMovers returns a short string summarising the N biggest movers
// by 24h change. Used by the agent context to give price awareness.
func TopMovers(n int) string {
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()

	if len(cachedPriceData) == 0 {
		return ""
	}

	type mover struct {
		symbol string
		price  float64
		change float64
	}
	tracked := []string{"BTC", "ETH", "SOL", "GOLD", "OIL"}
	var movers []mover
	for _, sym := range tracked {
		if pd, ok := cachedPriceData[sym]; ok {
			movers = append(movers, mover{sym, pd.Price, pd.Change24h})
		}
	}
	if len(movers) == 0 {
		return ""
	}
	if n > len(movers) {
		n = len(movers)
	}
	sort.SliceStable(movers, func(i, j int) bool {
		return math.Abs(movers[i].change) > math.Abs(movers[j].change)
	})

	var parts []string
	for _, m := range movers[:n] {
		dir := "+"
		if m.change < 0 {
			dir = ""
		}
		parts = append(parts, fmt.Sprintf("%s $%.0f (%s%.1f%%)", m.symbol, m.price, dir, m.change))
	}
	return strings.Join(parts, ", ")
}

func refreshMarkets() {
	// A failed fetch used to cost a full cycle: fetchPrices returns nil on any
	// error, the loop slept the whole interval and served the old price again,
	// and the only sign was a "Last refresh" line quietly ageing in small print
	// at the bottom of the page. An instance was found serving a price from
	// 04:54 UTC — BTC 2% out and ETH 3% — with nothing on the card saying so.
	//
	// So a failure retries soon and backs off, rather than waiting out the
	// interval as though nothing had happened.
	fail := 0
	for {
		prices, priceData := fetchPrices()
		if prices != nil {
			fail = 0
			html := generateMarketsCardHTML(prices)
			marketsMutex.Lock()
			cachedPrices = prices
			cachedPriceData = priceData
			lastPriceRefresh = time.Now().UTC()
			marketsHTML = html
			marketsMutex.Unlock()

			// Publish the new snapshot to the go-micro store + broker; the read
			// path serves it from a mirror (see internal/snapshot).
			cardSnap.Publish(html)

			indexMarketPrices(prices)
			data.SaveFile("markets.html", html)
			data.SaveJSON("prices.json", cachedPrices)
			data.SaveJSON("price_data.json", cachedPriceData)
		}

		wait := refreshEvery
		if prices == nil {
			fail++
			wait = retryAfter << min(fail-1, 4) // 30s, 1m, 2m, 4m, 8m, then hold
			if wait > refreshEvery {
				wait = refreshEvery
			}
			app.Log("markets", "fetch failed (%d in a row); retrying in %s", fail, wait)
		}
		time.Sleep(wait)
	}
}

// refreshEvery is how often prices are re-fetched.
//
// It was an hour, which is fine for a commodity future and wrong for crypto:
// the price on the card could be an hour old, and the 24h change worse than
// that, because it is a rolling window measured at fetch time rather than a
// figure that ages gracefully. A card reading +6.6% while the market says
// +2.2% is not a stale number, it reads as a wrong one.
//
// Five minutes against two keyless endpoints — Coinbase exchange-rates and
// CoinGecko's free tier — is 288 calls a day to each, well inside what either
// gives away, and nothing here is charged to a caller: the fetch is the
// instance's, and every read is served from the snapshot.
const refreshEvery = 5 * time.Minute

// retryAfter is the first wait after a failed fetch, doubling from there up to
// refreshEvery. A provider having a bad minute should not cost the whole
// interval; a provider that is down should not be hammered either.
const retryAfter = 30 * time.Second

func fetchPrices() (map[string]float64, map[string]PriceData) {
	app.Log("markets", "Fetching prices")

	rsp, err := http.Get("https://api.coinbase.com/v2/exchange-rates?currency=USD")
	if err != nil {
		app.Log("markets", "Error getting crypto prices: %v", err)
		return nil, nil
	}
	defer rsp.Body.Close()

	b, _ := ioutil.ReadAll(rsp.Body)
	rates, err := parseCoinbaseRates(b)
	if err != nil {
		app.Log("markets", "Error parsing crypto prices: %v", err)
		return nil, nil
	}
	prices := map[string]float64{}
	priceData := map[string]PriceData{}

	for k, t := range rates {
		val, err := strconv.ParseFloat(t, 64)
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
			pd.UpdatedAt = time.Now().UTC()
			pd.Source = "Coinbase + CoinGecko"
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
					UpdatedAt: time.Now().UTC(),
					Source:    "Yahoo Finance",
				}
			}
		}()
	}

	// Get stock prices. Same Yahoo quote endpoint as forex — an ordinary equity
	// ticker is its own symbol.
	app.Log("markets", "Fetching stock prices")
	for _, symbol := range stockSymbols {
		func() {
			defer func() {
				if r := recover(); r != nil {
					app.Log("markets", "Panic getting stock %s: %v", symbol, r)
				}
			}()

			q, err := quote.Get(symbol)
			if err != nil {
				app.Log("markets", "Failed to get stock %s: %v", symbol, err)
				return
			}
			if q == nil {
				return
			}
			price := q.RegularMarketPrice
			if price > 0 {
				prices[symbol] = price
				priceData[symbol] = PriceData{
					Price:     price,
					Change24h: q.RegularMarketChangePercent,
					UpdatedAt: time.Now().UTC(),
					Source:    "Yahoo Finance",
				}
			}
		}()
	}

	// Get forex 24h changes from Yahoo Finance
	app.Log("markets", "Fetching currency prices")
	for currency, yahooSymbol := range forexSymbols {
		func() {
			defer func() {
				if r := recover(); r != nil {
					app.Log("markets", "Panic getting forex %s: %v", currency, r)
				}
			}()

			q, err := quote.Get(yahooSymbol)
			if err != nil {
				app.Log("markets", "Failed to get forex %s: %v", currency, err)
				return
			}
			if q == nil {
				return
			}
			price := q.RegularMarketPrice
			if price > 0 {
				prices[currency] = price
				priceData[currency] = PriceData{
					Price:     price,
					Change24h: q.RegularMarketChangePercent,
					UpdatedAt: time.Now().UTC(),
					Source:    "Yahoo Finance",
				}
			}
		}()
	}

	app.Log("markets", "Finished fetching prices")
	return prices, priceData
}

func parseCoinbaseRates(body []byte) (map[string]string, error) {
	var res struct {
		Data struct {
			Rates map[string]string `json:"rates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	if len(res.Data.Rates) == 0 {
		return nil, fmt.Errorf("missing rates")
	}
	return res.Data.Rates, nil
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
	// Left column: crypto, Right column: commodities — each sorted alphabetically
	left := append([]string{}, tickers...)
	right := append([]string{}, futuresKeys...)
	sort.Strings(left)
	sort.Strings(right)

	// 4 rows max per column
	if len(left) > 4 {
		left = left[:4]
	}
	if len(right) > 4 {
		right = right[:4]
	}

	rows := len(left)
	if len(right) > rows {
		rows = len(right)
	}

	var sb strings.Builder
	sb.WriteString(`<table class="w-full">`)
	for i := 0; i < rows; i++ {
		sb.WriteString(`<tr>`)
		if i < len(left) {
			fmt.Fprintf(&sb, `<td class="cell"><span class="market-symbol">%s</span></td><td class="cell-right"><span class="market-price">$%.2f</span></td>`, left[i], prices[left[i]])
		} else {
			sb.WriteString(`<td></td><td></td>`)
		}
		if i < len(right) {
			fmt.Fprintf(&sb, `<td class="cell nested"><span class="market-symbol">%s</span></td><td class="cell-right"><span class="market-price">$%.2f</span></td>`, right[i], prices[right[i]])
		} else {
			sb.WriteString(`<td></td><td></td>`)
		}
		sb.WriteString(`</tr>`)
	}
	sb.WriteString(`</table>`)
	return sb.String()
}

func indexMarketPrices(prices map[string]float64) {
	app.Log("markets", "Indexing %d prices", len(prices))
	timestamp := time.Now().Format(time.RFC3339)
	for ticker, price := range prices {
		data.Index(
			"market_"+ticker,
			data.KindMarket,
			ticker,
			fmt.Sprintf("$%.2f", price),
			map[string]interface{}{
				"ticker": ticker,
				"price":  price,
				// A quote is as of now, so posted_at and the index time agree
				// today. Said anyway: the moment this stops overwriting one row
				// per ticker and starts keeping the history, the difference is
				// the whole point of the row.
				"posted_at": timestamp,
				"updated":   timestamp,
			},
		)
	}
}

// HTML returns the rendered markets card HTML. It serves the broker-fed
// snapshot mirror (the go-micro read plane); if no snapshot has arrived yet it
// falls back to the locally-generated HTML so a render never regresses.
func HTML() string {
	if s := cardSnap.Get(); s != "" {
		return s
	}
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()
	return marketsHTML
}

// AllPrices returns all cached prices
func AllPrices() map[string]float64 {
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()

	result := make(map[string]float64)
	for k, v := range cachedPrices {
		result[k] = v
	}
	return result
}

// AllPriceData returns all cached price data including 24h changes
func AllPriceData() map[string]PriceData {
	marketsMutex.RLock()
	defer marketsMutex.RUnlock()

	result := make(map[string]PriceData)
	for k, v := range cachedPriceData {
		result[k] = v
	}
	// Fall back to plain prices for any symbol not in priceData
	for k, price := range cachedPrices {
		if _, ok := result[k]; !ok {
			result[k] = PriceData{Price: price, UpdatedAt: lastPriceRefresh, Source: "cached price"}
		}
	}
	return result
}

func latestPriceUpdate(priceData map[string]PriceData) time.Time {
	var latest time.Time
	for _, pd := range priceData {
		if pd.UpdatedAt.After(latest) {
			latest = pd.UpdatedAt
		}
	}
	return latest
}

func marketsFreshness(priceData map[string]PriceData, assets []string) (time.Time, bool, bool) {
	var latest time.Time
	missing := false
	for _, symbol := range assets {
		pd, ok := priceData[symbol]
		if !ok || pd.Price == 0 {
			missing = true
			continue
		}
		if pd.UpdatedAt.After(latest) {
			latest = pd.UpdatedAt
		}
	}
	if latest.IsZero() {
		marketsMutex.RLock()
		latest = lastPriceRefresh
		marketsMutex.RUnlock()
	}
	// Stale is relative to how often it is meant to refresh. Two hours was the
	// old figure and it never fired: a price could be an hour and fifty-nine
	// minutes old and still call itself current.
	stale := latest.IsZero() || time.Since(latest) > 4*refreshEvery
	return latest, stale, missing
}

func marketsFreshnessText(updatedAt time.Time, stale, missing bool) string {
	parts := []string{}
	if updatedAt.IsZero() {
		parts = append(parts, "Last refresh: unavailable (cached fallback may be stale)")
	} else {
		parts = append(parts, fmt.Sprintf("Updated %s (%s UTC)",
			app.TimeAgo(updatedAt), updatedAt.UTC().Format("15:04")))
		if stale {
			parts = append(parts, "this is older than it should be — the last fetch did not get through")
		}
	}
	if missing {
		parts = append(parts, "some symbols are unavailable from the current source")
	}
	return strings.Join(parts, "; ")
}

// Categories for market data
const (
	CategoryCrypto      = "crypto"
	CategoryFutures     = "futures"
	CategoryCommodities = "commodities"
	CategoryCurrencies  = "currencies"
	CategoryStocks      = "stocks"
)

// Crypto assets to display
var cryptoAssets = []string{"BTC", "ETH", "UNI", "PAXG", "SOL", "ADA", "DOT", "LINK", "POL", "AVAX"}

// Futures/Commodities to display
var futuresAssets = []string{"OIL", "GOLD", "SILVER", "COPPER"}
var commoditiesAssets = []string{"COFFEE", "WHEAT", "CORN", "SOYBEANS", "OATS"}

// Stocks to display. Same list the fetch uses, so the page cannot show a
// ticker nothing is fetching.
var stockAssets = stockSymbols

// Currency assets to display (priced in USD)
var currencyAssets = []string{"EUR", "GBP", "JPY", "CAD", "AUD", "CHF", "CNY", "INR"}

// forexSymbols maps currency codes to Yahoo Finance forex ticker symbols
var forexSymbols = map[string]string{
	"EUR": "EURUSD=X",
	"GBP": "GBPUSD=X",
	"JPY": "JPYUSD=X",
	"CAD": "CADUSD=X",
	"AUD": "AUDUSD=X",
	"CHF": "CHFUSD=X",
	"CNY": "CNYUSD=X",
	"INR": "INRUSD=X",
}

// chartLinks maps asset symbols to their chart URLs
var chartLinks = map[string]string{
	// Crypto → CoinGecko charts
	"BTC":  "https://www.coingecko.com/en/coins/bitcoin",
	"ETH":  "https://www.coingecko.com/en/coins/ethereum",
	"UNI":  "https://www.coingecko.com/en/coins/uniswap",
	"PAXG": "https://www.coingecko.com/en/coins/pax-gold",
	"SOL":  "https://www.coingecko.com/en/coins/solana",
	"ADA":  "https://www.coingecko.com/en/coins/cardano",
	"DOT":  "https://www.coingecko.com/en/coins/polkadot",
	"LINK": "https://www.coingecko.com/en/coins/chainlink",
	"POL":  "https://www.coingecko.com/en/coins/polygon",
	"AVAX": "https://www.coingecko.com/en/coins/avalanche",
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
	// Stocks → Yahoo Finance charts, keyed by the ticker itself
	"AAPL":  "https://finance.yahoo.com/chart/AAPL",
	"MSFT":  "https://finance.yahoo.com/chart/MSFT",
	"NVDA":  "https://finance.yahoo.com/chart/NVDA",
	"GOOGL": "https://finance.yahoo.com/chart/GOOGL",
	"AMZN":  "https://finance.yahoo.com/chart/AMZN",
	"META":  "https://finance.yahoo.com/chart/META",
	"TSLA":  "https://finance.yahoo.com/chart/TSLA",
	"AVGO":  "https://finance.yahoo.com/chart/AVGO",
	"NFLX":  "https://finance.yahoo.com/chart/NFLX",
	"AMD":   "https://finance.yahoo.com/chart/AMD",
	// Currencies → Yahoo Finance forex charts
	"EUR": "https://finance.yahoo.com/chart/EURUSD%3DX",
	"GBP": "https://finance.yahoo.com/chart/GBPUSD%3DX",
	"JPY": "https://finance.yahoo.com/chart/JPYUSD%3DX",
	"CAD": "https://finance.yahoo.com/chart/CADUSD%3DX",
	"AUD": "https://finance.yahoo.com/chart/AUDUSD%3DX",
	"CHF": "https://finance.yahoo.com/chart/CHFUSD%3DX",
	"CNY": "https://finance.yahoo.com/chart/CNYUSD%3DX",
	"INR": "https://finance.yahoo.com/chart/INRUSD%3DX",
}

// MarketData represents market data for display
type MarketData struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h"`
	Type      string  `json:"type"`
	UpdatedAt string  `json:"updated_at,omitempty"`
	Source    string  `json:"source,omitempty"`
}

// Handler handles /markets requests
func Handler(w http.ResponseWriter, r *http.Request) {
	// Get current category from query param, default to crypto
	category := r.URL.Query().Get("category")
	if category == "" {
		category = CategoryCrypto
	}

	// Validate category
	if category != CategoryCrypto && category != CategoryFutures && category != CategoryCommodities &&
		category != CategoryCurrencies && category != CategoryStocks {
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
	priceData := AllPriceData()

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
			Source:    pd.Source,
		})
		if !pd.UpdatedAt.IsZero() {
			data[len(data)-1].UpdatedAt = pd.UpdatedAt.UTC().Format(time.RFC3339)
		}
	}
	updatedAt, stale, missing := marketsFreshness(priceData, assets)
	updatedAtText := ""
	if !updatedAt.IsZero() {
		updatedAtText = updatedAt.UTC().Format(time.RFC3339)
	}

	app.RespondJSON(w, map[string]interface{}{
		"category":   category,
		"data":       data,
		"updated_at": updatedAtText,
		"stale":      stale,
		"partial":    missing,
		"freshness":  marketsFreshnessText(updatedAt, stale, missing),
	})
}

// handleHTML returns market data as HTML page
func handleHTML(w http.ResponseWriter, r *http.Request, category string) {
	priceData := AllPriceData()

	// Generate HTML for the selected category
	body := generateMarketsPage(priceData, category, converterHTML(r))

	app.Respond(w, r, app.Response{
		Title:       "Markets",
		Description: "Live cryptocurrency, stock, futures, commodity, and currency market prices",
		HTML:        body,
	})
}

// getAssetsForCategory returns the list of assets for a given category
func getAssetsForCategory(category string) []string {
	switch category {
	case CategoryFutures:
		return futuresAssets
	case CategoryCommodities:
		// Everything a person means by "commodities": oil and the metals as
		// well as the crops. They were split by contract type — futures for the
		// hard ones, commodities for the agricultural — which is a distinction
		// the caller does not have. Asked for the oil price, an agent chose
		// commodities, received coffee and wheat, and reported that oil was
		// unavailable while it sat one category away.
		//
		// futures still returns the hard ones on their own, because that name
		// is accurate about what those contracts are and something may be
		// asking for exactly them.
		return append(append([]string{}, futuresAssets...), commoditiesAssets...)
	case CategoryCurrencies:
		return currencyAssets
	case CategoryStocks:
		return stockAssets
	default:
		return cryptoAssets
	}
}

// generateMarketsPage generates the full markets page HTML
func generateMarketsPage(priceData map[string]PriceData, activeCategory, converter string) string {
	var sb strings.Builder

	// Page header
	sb.WriteString(`<div class="markets-page">`)
	sb.WriteString(`<p class="description">Live market data for stocks, cryptocurrencies, futures, commodities, and currencies</p>`)

	// Category tabs
	sb.WriteString(`<div class="markets-tabs">`)
	sb.WriteString(generateTab("Crypto", CategoryCrypto, activeCategory))
	sb.WriteString(generateTab("Stocks", CategoryStocks, activeCategory))
	sb.WriteString(generateTab("Futures", CategoryFutures, activeCategory))
	sb.WriteString(generateTab("Commodities", CategoryCommodities, activeCategory))
	sb.WriteString(generateTab("Currencies", CategoryCurrencies, activeCategory))
	sb.WriteString(`</div>`)

	sb.WriteString(converter)

	// Market data table
	sb.WriteString(`<table class="markets-table">`)
	sb.WriteString(`<thead><tr><th>Symbol</th><th>Price</th><th>24h Change</th><th>Chart</th></tr></thead>`)
	sb.WriteString(`<tbody>`)

	assets := append([]string{}, getAssetsForCategory(activeCategory)...)

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
	updatedAt, stale, missing := marketsFreshness(priceData, assets)
	fmt.Fprintf(&sb, `<p class="markets-note">%s. Prices refresh every few minutes. For real-time trading, visit official exchanges.</p>`, marketsFreshnessText(updatedAt, stale, missing))
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

	// BTC and GOLD say what they are; AVGO does not. A ticker only reads as a
	// company to someone who already knows it, so the ones with a name carry it.
	label := symbol
	if name, ok := stockNames[symbol]; ok {
		label = symbol + ` <span class="markets-name">` + name + `</span>`
	}

	return fmt.Sprintf(`<tr>
		<td class="markets-symbol">%s</td>
		<td class="markets-price">%s</td>
		<td class="markets-change %s">%s</td>
		<td>%s</td>
	</tr>`, label, priceStr, changeClass, changeStr, chartHTML)
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
