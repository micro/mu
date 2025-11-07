package markets

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
	"mu/news"
)

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
  #tickers, #futures {
    display: flex;
    flex-wrap: wrap;
    gap: 15px;
  }
  #tickers .ticker, #futures .ticker {
    background: whitesmoke;
    padding: 15px 20px;
    border-radius: 5px;
    font-size: 1.1em;
  }
</style>
<h1>Markets</h1>
<div class="market-section">
  <h2>Cryptocurrency</h2>
  %s
</div>
<div class="market-section">
  <h2>Commodities &amp; Futures</h2>
  %s
</div>
`

// Handler serves the markets page
func Handler(w http.ResponseWriter, r *http.Request) {
	// Get the markets data from news package
	marketsData := news.Markets()

	var tickersHTML, futuresHTML string

	// Simple parsing: split by the two divs
	if len(marketsData) > 0 {
		// Extract tickers section
		if tickersStart := strings.Index(marketsData, `<div id="tickers">`); tickersStart != -1 {
			tickersEnd := strings.Index(marketsData[tickersStart:], `</div>`)
			if tickersEnd != -1 {
				tickersHTML = marketsData[tickersStart : tickersStart+tickersEnd+len(`</div>`)]
			}
		}

		// Extract futures section
		if futuresStart := strings.Index(marketsData, `<div id="futures">`); futuresStart != -1 {
			futuresEnd := strings.Index(marketsData[futuresStart:], `</div>`)
			if futuresEnd != -1 {
				futuresHTML = marketsData[futuresStart : futuresStart+futuresEnd+len(`</div>`)]
			}
		}
	}

	body := fmt.Sprintf(Template, tickersHTML, futuresHTML)
	html := app.RenderHTML("Markets", "Market prices for crypto, commodities and futures", body)
	w.Write([]byte(html))
}
