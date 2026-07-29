package wallet

import (
	"strings"
	"testing"
)

// Every billable operation must appear in Pricing(). The cost tables on the
// wallet page, the signed-out wallet page, the pricing API and the public
// pricing page all render from that one list, so an operation missing from it
// is a charge the user is never shown. Each of those tables used to be
// hardcoded separately and they drifted — image generation, the most expensive
// thing a user could trigger short of building an app, was absent from three.
func TestPricingCoversEveryBillableOperation(t *testing.T) {
	billable := []struct {
		op   string
		cost int
	}{
		{OpNewsSearch, CostNewsSearch},
		{OpVideoSearch, CostVideoSearch},
		{OpChatQuery, CostChatQuery},
		{OpBlogCreate, CostBlogCreate},
		{OpBlogComment, CostBlogComment},
		{OpMailSend, CostMailSend},
		{OpExternalEmail, CostExternalEmail},
		{OpPlacesSearch, CostPlacesSearch},
		{OpPlacesNearby, CostPlacesNearby},
		{OpWeatherForecast, CostWeatherForecast},
		{OpWeatherPollen, CostWeatherPollen},
		{OpWebSearch, CostWebSearch},
		{OpWebFetch, CostWebFetch},
		{OpDBWrite, CostDBWrite},
		{OpAgentQuery, CostAgentQuery},
		{OpAgentQueryPremium, CostAgentQueryPremium},
		{OpSocialSearch, CostSocialSearch},
		{OpSocialPost, CostSocialPost},
		{OpSocialReply, CostSocialReply},
		{OpImageGenerate, CostImageGenerate},
		{OpAppBuild, CostAppBuild},
		{OpAppEdit, CostAppEdit},
	}

	listed := map[string]PricingItem{}
	for _, it := range Pricing() {
		listed[it.Operation] = it
	}

	for _, b := range billable {
		it, ok := listed[b.op]
		if !ok {
			t.Errorf("operation %q is charged but not listed in Pricing()", b.op)
			continue
		}
		if it.Cost != b.cost {
			t.Errorf("Pricing() lists %q at %d, actual cost is %d", b.op, it.Cost, b.cost)
		}
		if strings.TrimSpace(it.Description) == "" {
			t.Errorf("operation %q has no description", b.op)
		}
	}

	if len(listed) != len(billable) {
		t.Errorf("Pricing() has %d entries, expected %d — an operation was added or duplicated",
			len(listed), len(billable))
	}
}

func TestPricingSortedByCost(t *testing.T) {
	items := Pricing()
	for i := 1; i < len(items); i++ {
		if items[i].Cost < items[i-1].Cost {
			t.Fatalf("Pricing() not sorted: %s (%d) after %s (%d)",
				items[i].Description, items[i].Cost,
				items[i-1].Description, items[i-1].Cost)
		}
	}
}

func TestPricingTableHTMLRendersEveryItem(t *testing.T) {
	out := PricingTableHTML()
	for _, it := range Pricing() {
		if !strings.Contains(out, it.Description) {
			t.Errorf("cost table missing %q", it.Description)
		}
	}
	if !strings.Contains(out, "included") {
		t.Error("cost table should note what is free")
	}
}

// The API and the rendered tables must agree.
func TestPricingDataMatchesPricing(t *testing.T) {
	api, catalogue := getPricingData(), Pricing()
	if len(api) != len(catalogue) {
		t.Fatalf("pricing API has %d items, catalogue has %d", len(api), len(catalogue))
	}
	for i := range api {
		if api[i] != catalogue[i] {
			t.Errorf("item %d differs: API %+v, catalogue %+v", i, api[i], catalogue[i])
		}
	}
}
