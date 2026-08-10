package quota

// What things cost is data, not code.
//
// It used to be thirty exported variables and a fifty-line switch mapping each
// one to an operation name, in the package every service imports — so changing
// a price meant editing Go, and the same list existed a second time in the
// wallet's Pricing() to render the cost table. Two hand-maintained copies of an
// operator's decision, in two packages, in different orders. They drifted:
// image generation, the most expensive thing a user could trigger short of
// building an app, was absent from three of the tables that rendered from it.
//
// One file now. The gate reads it and the published cost table reads it, so
// there is nothing left for them to disagree about.
//
// Three layers, in this order: the defaults main hands over at startup, then a
// quota.json in the data directory if an operator has dropped one there, then
// the env var named on each entry. Env last because that is how a container
// overrides one price without shipping a file.
//
// This package does not embed the file and does not go looking for it. quota.json
// sits at the top level beside main.go, which embeds it and calls Load — the
// same arrangement as everything else here, where the question lives in this
// package and the answer is supplied from above. A package underneath the
// product deciding for itself where on disk its configuration lives is how you
// end up unable to run two instances from one tree.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mu/internal/data"
)

// Price is one operation as an operator set it.
type Price struct {
	Op    string `json:"op"`
	Cost  int    `json:"cost"`
	Label string `json:"label"`
	Env   string `json:"env,omitempty"`
	Note  string `json:"note,omitempty"`
}

type priceFile struct {
	DailyQuota struct {
		Value int    `json:"value"`
		Env   string `json:"env"`
	} `json:"daily_quota"`
	Operations []Price `json:"operations"`
}

var (
	priceMu sync.RWMutex
	prices  = map[string]Price{}
	ordered []Price

	// DailyQuota is how many free calls a day an account gets where this
	// instance grants any.
	DailyQuota = 100
)

// defaults are the bytes of quota.json as main handed them over. Kept so
// ReloadPrices can re-apply the data directory and the environment on top of
// them without main having to be involved again.
var defaults []byte

// Load takes the contents of quota.json and makes it this instance's price
// list. Called once by main with the file embedded in the binary.
//
// Until it is called there are no prices, and an operation with no price costs
// the 1-credit default — which is the safe direction to be wrong in, but it is
// wrong, so a build that forgets this charges a flat penny for everything.
func Load(quotaJSON []byte) error {
	var f priceFile
	if err := json.Unmarshal(quotaJSON, &f); err != nil {
		return err
	}
	priceMu.Lock()
	defaults = quotaJSON
	priceMu.Unlock()
	apply(f)
	return nil
}

// ReloadPrices re-applies the data directory and the environment over the
// defaults, for an operator changing quota.json without a restart.
func ReloadPrices() {
	priceMu.RLock()
	b := defaults
	priceMu.RUnlock()
	if len(b) == 0 {
		return
	}
	_ = Load(b)
}

func apply(f priceFile) {

	// An operator's own file replaces any entry it names and leaves the rest.
	// Replacing entry by entry rather than wholesale means a file listing one
	// price does not silently zero the other twenty-five.
	if b, err := data.LoadFile("quota.json"); err == nil && len(b) > 0 {
		var override priceFile
		if err := json.Unmarshal(b, &override); err == nil {
			if override.DailyQuota.Value > 0 {
				f.DailyQuota.Value = override.DailyQuota.Value
			}
			at := map[string]int{}
			for i, p := range f.Operations {
				at[p.Op] = i
			}
			for _, p := range override.Operations {
				if i, ok := at[p.Op]; ok {
					f.Operations[i].Cost = p.Cost
					if p.Label != "" {
						f.Operations[i].Label = p.Label
					}
					continue
				}
				f.Operations = append(f.Operations, p)
			}
		}
	}

	byOp := make(map[string]Price, len(f.Operations))
	list := make([]Price, 0, len(f.Operations))
	for _, p := range f.Operations {
		p.Cost = envOverride(p.Env, p.Cost)
		byOp[p.Op] = p
		list = append(list, p)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Cost < list[j].Cost })

	quotaValue := envOverride(f.DailyQuota.Env, f.DailyQuota.Value)
	if f.DailyQuota.Env == "DAILY_QUOTA" {
		// The older name, still honoured for instances configured before the
		// rename.
		quotaValue = envOverride("FREE_DAILY_QUOTA", quotaValue)
		quotaValue = envOverride("DAILY_QUOTA", quotaValue)
	}

	priceMu.Lock()
	prices, ordered, DailyQuota = byOp, list, quotaValue
	priceMu.Unlock()
}

// envOverride reads key as a positive integer, or returns the fallback.
//
// Zero is not accepted as an override, which is deliberate and worth knowing:
// an unset variable and one set to "0" are the same string to a container, and
// a price silently dropping to free is the wrong way to fail. Set it in the
// file to make something free.
func envOverride(key string, fallback int) int {
	if key == "" {
		return fallback
	}
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// GetOperationCost is what this operation costs the caller, in credits.
//
// An operation with no entry costs 1. That is the old switch's default and it
// is kept on purpose: an unpriced operation charging nothing would be the
// quieter failure of the two, and TestEveryChargedOperationIsPublished catches
// a charge site whose operation is not in the file.
func GetOperationCost(operation string) int {
	priceMu.RLock()
	defer priceMu.RUnlock()
	if p, ok := prices[operation]; ok {
		return p.Cost
	}
	return 1
}

// Prices is every published operation, cheapest first. The cost tables on the
// wallet page, the signed-out wallet page, the pricing API and the public
// pricing page all render from this one list.
func Prices() []Price {
	priceMu.RLock()
	defer priceMu.RUnlock()
	out := make([]Price, len(ordered))
	copy(out, ordered)
	return out
}

// Published reports whether an operation has a price anybody can look up.
func Published(operation string) bool {
	priceMu.RLock()
	defer priceMu.RUnlock()
	_, ok := prices[operation]
	return ok
}

// LoadFromTree finds quota.json by walking up from the working directory and
// loads it.
//
// The command embeds the file, so nothing in production comes through here.
// Test binaries are the case: they run in a package directory inside the source
// tree, nobody has called Load, and every operation would fall back to the
// 1-credit default — which does not fail, it just quietly makes any test about
// prices agree with itself about nothing.
func LoadFromTree() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		path := filepath.Join(dir, "quota.json")
		if b, err := os.ReadFile(path); err == nil {
			return Load(b)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return errors.New("no quota.json above " + dir)
		}
		dir = parent
	}
}
