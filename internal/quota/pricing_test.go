package quota

// quota.json and the operation constants have to describe the same world.
//
// Moving prices out of Go bought an operator the ability to change one without
// a rebuild, and cost the compiler its ability to catch a typo. `"op":
// "web_serch"` compiles, loads, and quietly prices web_search at the
// unpriced-operation default of 1 credit — no error, no log line, and a cost
// table that still looks complete because the misspelled row is in it.
//
// So the two are checked against each other here, in both directions.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// declaredOps reads the Op constants out of the source, which is the same
// trick the charge-site test uses: a hand-kept list here would be a third copy
// of the thing being checked.
func declaredOps(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("quota.go")
	if err != nil {
		t.Fatalf("read quota.go: %v", err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`Op[A-Za-z]+\s*=\s*"([a-z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	if len(out) < 20 {
		t.Fatalf("found %d operation constants — the parser is broken, not the code", len(out))
	}
	return out
}

func TestEveryPricedOperationIsARealOne(t *testing.T) {
	declared := declaredOps(t)
	for _, p := range Prices() {
		if !declared[p.Op] {
			t.Errorf("quota.json prices %q, which is not a declared operation — "+
				"a misspelling here is silent, and the operation it meant to name "+
				"falls back to charging 1 credit", p.Op)
		}
	}
}

// Movements of credit are not prices. They appear on a transaction and have
// nothing to publish, which is why they are absent from the file on purpose.
var notAPrice = map[string]bool{
	"topup": true, "refund": true, "transfer": true, "app_revenue": true,
	"escrow_hold": true, "escrow_release": true, "escrow_refund": true,
	// Charged per request at a price the app's author sets, so there is no
	// fixed figure to publish.
	"app_use": true,
}

func TestEveryOperationHasAPrice(t *testing.T) {
	for op := range declaredOps(t) {
		if notAPrice[op] {
			continue
		}
		if !Published(op) {
			t.Errorf("operation %q has no entry in pricing.json, so it charges the "+
				"1-credit default and appears in no cost table", op)
		}
	}
}

// Every entry says what it is, or the cost table has a blank row.
func TestEveryPriceIsLabelled(t *testing.T) {
	for _, p := range Prices() {
		if strings.TrimSpace(p.Label) == "" {
			t.Errorf("%s has no label", p.Op)
		}
	}
}

// Cheapest first, because that is the order the tables render in and sorting at
// the point of display was how one of them ended up sorted differently.
func TestPricesComeOutSorted(t *testing.T) {
	list := Prices()
	for i := 1; i < len(list); i++ {
		if list[i].Cost < list[i-1].Cost {
			t.Fatalf("out of order: %s (%d) after %s (%d)",
				list[i].Op, list[i].Cost, list[i-1].Op, list[i-1].Cost)
		}
	}
}

// An operator's override of one price must not silently drop the other
// twenty-five. Replacing the whole list is the obvious implementation and the
// wrong one: a file with a single line in it would leave everything else at the
// unpriced default.
func TestAnOverrideReplacesOneEntryNotTheList(t *testing.T) {
	before := len(Prices())
	if before == 0 {
		t.Fatal("no prices loaded")
	}
	webBefore := OperationCost(OpWebSearch)

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := `{"operations":[{"op":"web_search","cost":9}]}`
	if err := os.WriteFile(filepath.Join(dir, ".mu", "data", "quota.json"), []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ReloadPrices)
	ReloadPrices()

	if got := OperationCost(OpWebSearch); got != 9 {
		t.Fatalf("an operator's quota.json was ignored: web_search is %d, want 9", got)
	}
	if after := len(Prices()); after != before {
		t.Errorf("a one-line override left %d prices, was %d — the rest were dropped",
			after, before)
	}
	if OperationCost(OpAgentQuery) == 1 && webBefore != 1 {
		t.Error("agent_query fell back to the unpriced default, so the override " +
			"replaced the list rather than one entry")
	}
}

// The env var named on an entry overrides it, which is how a container changes
// one price without shipping a file.
func TestTheNamedEnvVarOverridesAPrice(t *testing.T) {
	var withEnv Price
	for _, p := range Prices() {
		if p.Env != "" {
			withEnv = p
			break
		}
	}
	if withEnv.Op == "" {
		t.Fatal("no priced operation names an env var, so nothing can be overridden")
	}
	t.Setenv(withEnv.Env, "42")
	t.Cleanup(ReloadPrices)
	ReloadPrices()
	if got := OperationCost(withEnv.Op); got != 42 {
		t.Errorf("%s=42 left %s at %d", withEnv.Env, withEnv.Op, got)
	}
}

// quota.json has to parse, because main embeds it and refuses to start on a
// file it cannot read — a broken one is a deploy that does not come up.
func TestTheShippedFileParses(t *testing.T) {
	priceMu.RLock()
	b := defaults
	priceMu.RUnlock()

	var f priceFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("quota.json: %v", err)
	}
	if len(f.Operations) == 0 {
		t.Fatal("the price list is empty")
	}
	// No daily_quota key any more. It granted every account a hundred credits a
	// day to spend on anything priced, which existed only to pay back the charge
	// for talking to the agent — and the agent is free now.
	if strings.Contains(string(b), "daily_quota") {
		t.Error("quota.json is granting a daily allowance again")
	}
}
