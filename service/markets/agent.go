package markets

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
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
	fmt.Fprintf(&sb, "Current request date: %s.\n", time.Now().UTC().Format("Monday, 2 January 2006 (2006-01-02, UTC)"))
	updatedAt, stale, missing := marketsFreshness(priceData, assets)
	fmt.Fprintf(&sb, "%s.\n", marketsFreshnessText(updatedAt, stale, missing))
	type marketLine struct {
		symbol string
		price  float64
		change float64
	}
	var movers []marketLine
	for _, symbol := range assets {
		pd, ok := priceData[symbol]
		if !ok || pd.Price == 0 {
			continue
		}
		movers = append(movers, marketLine{symbol: symbol, price: pd.Price, change: pd.Change24h})
	}
	if len(movers) == 0 {
		return fmt.Sprintf("No %s prices available right now.", category)
	}
	sort.SliceStable(movers, func(i, j int) bool {
		return math.Abs(movers[i].change) > math.Abs(movers[j].change)
	})
	limit := 5
	if len(movers) < limit {
		limit = len(movers)
	}
	fmt.Fprintf(&sb, "Top %s movers by 24h change:\n", category)
	for _, item := range movers[:limit] {
		if item.change != 0 {
			fmt.Fprintf(&sb, "%s: $%s (%+.2f%% 24h)\n", item.symbol, marketsPriceStr(item.price), item.change)
		} else {
			fmt.Fprintf(&sb, "%s: $%s (24h change unavailable)\n", item.symbol, marketsPriceStr(item.price))
		}
	}
	if len(movers) > limit {
		var watched []string
		for _, item := range movers[limit:] {
			watched = append(watched, fmt.Sprintf("%s $%s", item.symbol, marketsPriceStr(item.price)))
		}
		fmt.Fprintf(&sb, "Other watched prices: %s.\n", strings.Join(watched, ", "))
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
