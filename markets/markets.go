package markets

import (
	"fmt"
	"net/http"
	"sort"

	"mu/app"
	"mu/news"
)

// Additional tickers to display on the markets page (beyond the homepage summary)
var additionalCrypto = []string{
	"ADA", "SOL", "DOT", "AVAX", "MATIC", "LINK", "UNI", "ATOM", "XRP", "DOGE",
	"LTC", "BCH", "ETC", "XMR", "ALGO", "VET", "FIL", "AAVE", "SNX", "CRV",
}

// Additional futures to display (beyond homepage, these are already fetched)
var additionalFutures = []string{"SILVER", "COPPER", "CORN", "SOYBEANS"}

var Template = `
<style>
  .market-section {
    margin-bottom: 40px;
  }
  .market-section h2 {
    margin-bottom: 20px;
    border-bottom: 2px solid #333;
    padding-bottom: 10px;
  }
  .market-list {
    display: flex;
    flex-direction: column;
    gap: 0;
  }
  .market-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 15px;
    border-bottom: 1px solid #e0e0e0;
    background: white;
  }
  .market-item:hover {
    background: #f9f9f9;
  }
  .market-item .symbol {
    font-weight: bold;
    font-size: 1em;
    color: #333;
    min-width: 100px;
  }
  .market-item .price {
    font-size: 1em;
    color: #666;
    text-align: right;
  }
</style>
<h1>Markets</h1>
<div class="market-section">
  <h2>Cryptocurrency</h2>
  <div class="market-list">%s</div>
</div>
<div class="market-section">
  <h2>Commodities &amp; Futures</h2>
  <div class="market-list">%s</div>
</div>
`

// Handler serves the markets page with extended market data
func Handler(w http.ResponseWriter, r *http.Request) {
	// Get all prices from the news package
	prices := news.GetAllPrices()

	var cryptoHTML, futuresHTML string

	// Build crypto section with homepage tickers + additional ones
	allCryptoTickers := append(news.GetHomepageTickers(), additionalCrypto...)
	
	// Sort for consistent display
	sort.Strings(allCryptoTickers)
	
	for _, ticker := range allCryptoTickers {
		if price, ok := prices[ticker]; ok && price > 0 {
			cryptoHTML += fmt.Sprintf(`
				<div class="market-item">
					<div class="symbol">%s</div>
					<div class="price">$%.2f</div>
				</div>`, ticker, price)
		}
	}

	// Build futures section with homepage futures + additional ones
	allFuturesKeys := append(news.GetHomepageFutures(), additionalFutures...)
	
	for _, key := range allFuturesKeys {
		if price, ok := prices[key]; ok && price > 0 {
			futuresHTML += fmt.Sprintf(`
				<div class="market-item">
					<div class="symbol">%s</div>
					<div class="price">$%.2f</div>
				</div>`, key, price)
		}
	}

	body := fmt.Sprintf(Template, cryptoHTML, futuresHTML)
	html := app.RenderHTML("Markets", "Market prices for crypto, commodities and futures", body)
	w.Write([]byte(html))
}
