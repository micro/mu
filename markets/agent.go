package markets

import (
	"fmt"
	"strings"
)

// MarketsText returns a compact, model-ready snapshot of live prices for the
// given category (crypto, futures, commodities or currencies; default crypto).
// It is the AI-first accessor behind the markets agent tool — no HTML, no
// HTTP round-trip.
func MarketsText(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	if category != CategoryFutures && category != CategoryCommodities && category != CategoryCurrencies {
		category = CategoryCrypto
	}

	priceData := GetAllPriceData()
	assets := getAssetsForCategory(category)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Live %s prices:\n", category)
	found := 0
	for _, symbol := range assets {
		pd, ok := priceData[symbol]
		if !ok || pd.Price == 0 {
			continue
		}
		if pd.Change24h != 0 {
			fmt.Fprintf(&sb, "%s: $%s (%+.2f%% 24h)\n", symbol, marketsPriceStr(pd.Price), pd.Change24h)
		} else {
			fmt.Fprintf(&sb, "%s: $%s\n", symbol, marketsPriceStr(pd.Price))
		}
		found++
	}
	if found == 0 {
		return fmt.Sprintf("No %s prices available right now.", category)
	}
	return sb.String()
}

// marketsPriceStr formats a price with precision appropriate to its magnitude.
func marketsPriceStr(p float64) string {
	switch {
	case p >= 100:
		return fmt.Sprintf("%.2f", p)
	case p >= 1:
		return fmt.Sprintf("%.3f", p)
	default:
		return fmt.Sprintf("%.6f", p)
	}
}
