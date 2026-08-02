package markets

import (
	"strings"
	"testing"
	"time"
)

func TestMarketsTextPrioritizesTopMovers(t *testing.T) {
	marketsMutex.Lock()
	old := cachedPriceData
	cachedPriceData = map[string]PriceData{
		"BTC":  {Price: 100000, Change24h: 1.2, UpdatedAt: time.Now().UTC()},
		"ETH":  {Price: 3000, Change24h: -4.5, UpdatedAt: time.Now().UTC()},
		"SOL":  {Price: 150, Change24h: 7.1, UpdatedAt: time.Now().UTC()},
		"ADA":  {Price: 0.5, Change24h: -0.8, UpdatedAt: time.Now().UTC()},
		"DOT":  {Price: 5, Change24h: 2.0, UpdatedAt: time.Now().UTC()},
		"LINK": {Price: 18, Change24h: -1.5, UpdatedAt: time.Now().UTC()},
	}
	marketsMutex.Unlock()
	defer func() {
		marketsMutex.Lock()
		cachedPriceData = old
		marketsMutex.Unlock()
	}()

	got := MarketsText(CategoryCrypto)
	if !strings.Contains(got, "Top crypto movers by 24h change:") {
		t.Fatalf("expected top movers heading, got %q", got)
	}
	firstSOL := strings.Index(got, "SOL: $")
	firstETH := strings.Index(got, "ETH: $")
	if firstSOL == -1 || firstETH == -1 || firstSOL > firstETH {
		t.Fatalf("expected largest absolute mover first, got %q", got)
	}
	if strings.Count(got, "24h") > 6 {
		t.Fatalf("expected concise mover list, got %q", got)
	}
	if !strings.Contains(got, "Other watched prices:") {
		t.Fatalf("expected extra assets compressed into watched prices, got %q", got)
	}
}
