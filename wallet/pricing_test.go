package wallet

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mu/internal/quota"
)

// Every billable operation must appear in the published list. The cost tables
// on the wallet page, the signed-out wallet page, the pricing API and the
// public pricing page all render from it, so an operation missing from it is a
// charge the user is never shown. Each of those tables used to be hardcoded
// separately and they drifted — image generation, the most expensive thing a
// user could trigger short of building an app, was absent from three.
//
// The list is quota.json now and this only checks that the
// wallet renders all of it, which is the half that lives here. Whether the file
// covers every operation the code charges is asserted where the operations are
// declared, in internal/quota.
func TestPricingRendersTheWholeList(t *testing.T) {
	published := quota.Prices()
	if len(published) < 20 {
		t.Fatalf("only %d priced operations — the price list is not loading", len(published))
	}

	listed := map[string]PricingItem{}
	for _, it := range Pricing() {
		listed[it.Operation] = it
	}
	for _, p := range published {
		it, ok := listed[p.Op]
		if !ok {
			t.Errorf("operation %q is priced but the wallet does not show it", p.Op)
			continue
		}
		if it.Cost != p.Cost {
			t.Errorf("the wallet shows %q at %d, the price list says %d", p.Op, it.Cost, p.Cost)
		}
		if strings.TrimSpace(it.Description) == "" {
			t.Errorf("operation %q has no label to show", p.Op)
		}
	}
	if len(listed) != len(published) {
		t.Errorf("the wallet shows %d operations, the price list has %d",
			len(listed), len(published))
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

// Charge sites live outside this package and are wired up as
// WalletOp: wallet.OpX or QuotaCheck(r, wallet.OpX). Nothing else links a
// constant to its use, so an operation can be charged without ever being
// published — that is how paid-app usage and a misspelled "search" op both
// escaped the cost table, and how "App edit (AI)" stayed published at 50
// credits while nothing charged it.
//
// This reads the constant block and the charge sites out of the source, so it
// cannot drift the way a hand-maintained list does.
func TestEveryChargedOperationIsPublished(t *testing.T) {
	// Movements of credit rather than prices for work. These appear on
	// transactions but do not belong in a cost table.
	notAPrice := map[string]bool{
		"topup": true, "refund": true, "transfer": true, "app_revenue": true,
		"escrow_hold": true, "escrow_release": true, "escrow_refund": true,
		// Charged per request at a price the app author sets, so it has no
		// fixed figure to publish; the table notes it separately.
		"app_use": true,
	}

	// Constant name -> operation string, read from the source of truth.
	src, err := os.ReadFile("../internal/quota/quota.go")
	if err != nil {
		t.Fatalf("read quota.go: %v", err)
	}
	opValue := map[string]string{}
	for _, m := range regexp.MustCompile(`(Op[A-Za-z]+)\s*=\s*"([a-z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		opValue[m[1]] = m[2]
	}
	if len(opValue) == 0 {
		t.Fatal("found no Op constants — the parser is broken, not the code")
	}

	published := map[string]bool{}
	for _, it := range Pricing() {
		published[it.Operation] = true
	}

	// Three forms. WalletOp on a hand-written tool and QuotaCheck at a call
	// site were the original two; Cost on a service Endpoint is where most
	// charges live now that tools derive from Specs, and a scan that did not
	// know about it went from thirty-odd charge sites to twelve without
	// noticing that the ones it lost were the ones that moved.
	site := regexp.MustCompile(`(?:WalletOp:\s*|Cost:\s*|QuotaCheck\([^,]+,\s*)(?:quota\.(Op[A-Za-z]+)|"([a-z_]+)")`)

	found := 0
	err = filepath.Walk(repoRoot(t), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/wallet/") || strings.Contains(path, "/quota/") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range site.FindAllStringSubmatch(string(b), -1) {
			op := m[2] // bare string form
			if m[1] != "" {
				var ok bool
				op, ok = opValue[m[1]]
				if !ok {
					t.Errorf("%s charges quota.%s, which is not a declared operation", path, m[1])
					continue
				}
			}
			found++
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
	// Guards against the scan silently matching nothing.
	if found < 15 {
		t.Errorf("only found %d charge sites; the scan is probably not matching", found)
	}
}

// repoRoot walks up to the directory holding go.mod, so this test keeps
// scanning the whole tree if packages are moved. It was previously ".." — which
// silently became service/ when the services were nested, and found almost
// nothing. The minimum-hits assertion above is what caught that.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test directory")
		}
		dir = parent
	}
}
