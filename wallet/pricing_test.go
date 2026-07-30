package wallet

import (
	"os"
	"path/filepath"
	"regexp"
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
		{OpQuranSearch, CostQuranSearch},
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
		// OpAppEdit is intentionally not published — nothing charges it.
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

// The list above is hand-maintained, which is how "App edit (AI)" came to be
// published at 50 credits while nothing charged it, and how paid-app usage
// came to be charged while nothing published it. This test instead reads the
// charge sites out of the source: operations are wired up with string literals
// (WalletOp: "agent_query", QuotaCheck(r, "chat_query")), not the Op
// constants, so nothing links a constant to its use and drift is invisible.
//
// Every operation string the code actually charges must appear in Pricing().
func TestEveryChargedOperationIsPublished(t *testing.T) {
	// Movements of credit rather than charges for work — these are recorded on
	// transactions but are not prices, so they do not belong in a cost table.
	notAPrice := map[string]bool{
		"topup": true, "refund": true, "transfer": true,
		"app_revenue": true, "app_use": true, // app_use is variable, noted separately
		"escrow_hold": true, "escrow_release": true, "escrow_refund": true,
	}

	published := map[string]bool{}
	for _, it := range Pricing() {
		published[it.Operation] = true
	}

	// WalletOp: "..." and QuotaCheck(r, "...") are the two ways an operation
	// gets charged outside this package.
	pat := regexp.MustCompile(`(?:WalletOp:\s*|QuotaCheck\([^,]+,\s*)"([a-z_]+)"`)

	root := ".."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/wallet/") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range pat.FindAllStringSubmatch(string(b), -1) {
			op := m[1]
			if notAPrice[op] || published[op] {
				continue
			}
			t.Errorf("%s charges %q but Pricing() does not publish it", path, op)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
