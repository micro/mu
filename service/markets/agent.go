package markets

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Text returns a compact, model-ready snapshot of live prices for the
// given category (crypto, stocks, futures, commodities or currencies; default
// crypto).
// It is the AI-first accessor behind the markets agent tool — no HTML, no
// HTTP round-trip.
func Text(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	if category != CategoryFutures && category != CategoryCommodities &&
		category != CategoryCurrencies && category != CategoryStocks {
		category = CategoryCrypto
	}

	priceData := AllPriceData()
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
		label := item.symbol
		if name, ok := stockNames[item.symbol]; ok {
			label = item.symbol + " (" + name + ")"
		}
		if item.change != 0 {
			fmt.Fprintf(&sb, "%s: $%s (%+.2f%% 24h)\n", label, marketsPriceStr(item.price), item.change)
		} else {
			fmt.Fprintf(&sb, "%s: $%s (24h change unavailable)\n", label, marketsPriceStr(item.price))
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

// Now is the prices as a prompt reads them: the crypto majors, one line each.
//
// Purpose-built rather than Text(CategoryCrypto), for two reasons this cost a
// test to notice. Text opens with "Current request date", which the system
// prompt has already said one paragraph earlier — a second, differently worded
// copy of the date is how a model comes to disagree with itself about what day
// it is. And with no prices Text says "No crypto prices available right now",
// which is a true sentence and the wrong thing to put in front of every
// question on the instance: silence is what "I have nothing to add" looks like
// in a prompt.
//
// Nothing is fetched. AllPriceData reads the poller's last answer out of
// memory, which is what makes this free to carry. See service.Spec.Now.
func Now() string {
	priceData := AllPriceData()
	var lines []string
	for _, symbol := range getAssetsForCategory(CategoryCrypto) {
		pd, ok := priceData[symbol]
		if !ok || pd.Price == 0 {
			continue
		}
		line := fmt.Sprintf("- %s $%s", symbol, marketsPriceStr(pd.Price))
		if pd.Change24h != 0 {
			line += fmt.Sprintf(" (%+.2f%% 24h)", pd.Change24h)
		}
		lines = append(lines, line)
		if len(lines) >= nowAssets {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Prices, as of now:\n" + strings.Join(lines, "\n") +
		"\nAsk markets_list for other categories, markets_convert to convert."
}

// nowAssets is how many prices ride along on every question. The majors answer
// "what is bitcoin doing"; the rest are a tool call away and always were.
const nowAssets = 6
