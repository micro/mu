package markets

import "testing"

// Oil is what somebody asking about commodities means. It used to be filed
// under futures alone, so the obvious category answered with crops and an
// agent reported the price unavailable while it sat one call away.
func TestCommoditiesIncludeOilAndMetals(t *testing.T) {
	got := map[string]bool{}
	for _, a := range getAssetsForCategory(CategoryCommodities) {
		got[a] = true
	}
	for _, want := range []string{"OIL", "GOLD", "SILVER", "COPPER", "COFFEE", "WHEAT"} {
		if !got[want] {
			t.Errorf("commodities is missing %s", want)
		}
	}
}

// futures keeps its narrow meaning: that name is accurate about those
// contracts, and something may want exactly them.
func TestFuturesStaysTheHardOnes(t *testing.T) {
	got := getAssetsForCategory(CategoryFutures)
	if len(got) != 4 {
		t.Errorf("futures = %v, want the four hard commodities", got)
	}
	for _, a := range got {
		if a == "COFFEE" || a == "WHEAT" {
			t.Errorf("futures gained a crop: %v", got)
		}
	}
}
