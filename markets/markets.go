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
var additionalFutures = []string{"SILVER", "COPPER", "NATGAS", "CORN", "SOYBEANS"}

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
  .price-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 15px;
    margin-bottom: 20px;
  }
  .price-item {
    background: whitesmoke;
    padding: 15px;
    border-radius: 5px;
  }
  .price-item .symbol {
    font-weight: bold;
    font-size: 1.1em;
    color: #333;
    margin-bottom: 5px;
  }
  .price-item .price {
    font-size: 1em;
    color: #666;
  }
</style>
<h1>Markets</h1>
<div class="market-section">
  <h2>Cryptocurrency</h2>
  <div class="price-grid">%s</div>
</div>
<div class="market-section">
  <h2>Commodities &amp; Futures</h2>
  <div class="price-grid">%s</div>
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
		if price, ok := prices[ticker]; ok {
			cryptoHTML += fmt.Sprintf(`
				<div class="price-item">
					<div class="symbol">%s</div>
					<div class="price">$%.2f</div>
				</div>`, ticker, price)
		}
	}

	// Build futures section with homepage futures + additional ones
	allFuturesKeys := append(news.GetHomepageFutures(), additionalFutures...)
	
	for _, key := range allFuturesKeys {
		if price, ok := prices[key]; ok {
			futuresHTML += fmt.Sprintf(`
				<div class="price-item">
					<div class="symbol">%s</div>
					<div class="price">$%.2f</div>
				</div>`, key, price)
		}
	}

	body := fmt.Sprintf(Template, cryptoHTML, futuresHTML)
	html := app.RenderHTML("Markets", "Market prices for crypto, commodities and futures", body)
	w.Write([]byte(html))
}
