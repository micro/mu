package ai

// Every model this instance asks for has a price, and it is that model's.
//
// The bug was never one wrong number. It was that a miss was silent: the lookup
// used strings.Contains, so a key became wrong the moment a newer id extended
// it and missed entirely when an id moved on, and either way the request was
// billed at a fallback with nothing said. Both hard-coded defaults were
// mispriced at once — the interactive model matched nothing, the background one
// matched a retired rate — and the table had drifted for however long.
//
// So the test that matters is not "is Sonnet $2". It is "does every id this
// code can produce find its own entry", which is checkable here and fails the
// day somebody changes a default without moving the price with it.

import (
	"math"
	"testing"
)

// The models providers.go names. Anything this instance can ask for on purpose.
var askedFor = []string{
	ModelClaudeOpus, ModelClaudeSonnet, ModelClaudeHaiku,
	ModelDeepSeekPro, ModelDeepSeekFlash, ModelQwenPlus,
	ModelGeminiPro, ModelGeminiFlash,
	ModelOpenRouter,
}

func TestEveryModelWeAskForHasAPrice(t *testing.T) {
	for _, id := range askedFor {
		p, ok := priceOf(id)
		if !ok {
			t.Errorf("%s is a model this instance asks for and has no price, so "+
				"every question on it is billed at the fallback", id)
			continue
		}
		if p.InputPerM <= 0 || p.OutputPerM <= 0 {
			t.Errorf("%s prices at in=%v out=%v", id, p.InputPerM, p.OutputPerM)
		}
	}
}

// The two that were wrong, named, because they are the ones that were live.
//
// claude-sonnet-5 does not contain "claude-sonnet-4", so it matched nothing and
// took the fallback. claude-haiku-4-5-20251001 contains "claude-haiku-4", so it
// took Haiku 3.5's retired rate.
func TestTheDefaultsAreNotPricedAsSomethingElse(t *testing.T) {
	sonnet, ok := priceOf(ModelClaudeSonnet)
	if !ok {
		t.Fatalf("%s has no entry", ModelClaudeSonnet)
	}
	if sonnet == fallbackPricing {
		t.Errorf("%s is priced at the fallback, which is what the old Contains "+
			"match did to it", ModelClaudeSonnet)
	}

	haiku, ok := priceOf(ModelClaudeHaiku)
	if !ok {
		t.Fatalf("%s has no entry", ModelClaudeHaiku)
	}
	if haiku.InputPerM >= sonnet.InputPerM {
		t.Errorf("the background model costs %v and the interactive one %v — "+
			"background work is meant to be the cheap end", haiku.InputPerM, sonnet.InputPerM)
	}
}

// A newer id in a family does not inherit the retired one's price.
//
// "claude-opus-4" is a prefix of claude-opus-4-5 and everything after it, and
// Contains handed all of them the retired Opus 4 rate. Longest-prefix matching
// is what makes the specific entry win.
func TestANewerIdDoesNotTakeARetiredPrice(t *testing.T) {
	retired, ok := priceOf("claude-opus-4")
	if !ok {
		t.Fatal("claude-opus-4 has no entry")
	}
	for _, id := range []string{"claude-opus-4-5", "claude-opus-4-6", "claude-opus-4-8"} {
		p, ok := priceOf(id)
		if !ok {
			t.Errorf("%s has no entry", id)
			continue
		}
		if p == retired {
			t.Errorf("%s is billed at claude-opus-4's rate, which is the retired one", id)
		}
	}
}

// And the answer does not change between runs.
//
// The old lookup iterated a map and broke on the first hit, so two matching keys
// meant a price that depended on Go's randomised map order. Nothing overlapped
// at the time, which is why it was survivable and why it would have been a
// nightmare when it stopped being true.
func TestAPriceIsTheSameEveryTime(t *testing.T) {
	for _, id := range append(append([]string{}, askedFor...),
		"claude-haiku-4-5-20251001", "claude-opus-4-5-20260101") {
		first, ok := priceOf(id)
		if !ok {
			continue
		}
		for i := 0; i < 50; i++ {
			again, _ := priceOf(id)
			if again != first {
				t.Fatalf("%s priced %v then %v — the lookup depends on map order", id, first, again)
			}
		}
	}
}

// A cache is charged at the model's own rate, or not at all.
//
// It was two constants at Sonnet 4.5's price added to every request that
// reported cache tokens, whoever served it: three times over for Haiku, and
// Anthropic's prices on an Atlas bill.
func TestCacheIsChargedAtTheModelsOwnRate(t *testing.T) {
	const tokens = 1_000_000

	// Anthropic's cheapest model is not charged Sonnet's cache rate.
	haiku := estimateCostCents(ModelClaudeHaiku, 0, 0, tokens, 0)
	sonnet := estimateCostCents(ModelClaudeSonnet, 0, 0, tokens, 0)
	if haiku >= sonnet {
		t.Errorf("a cache read costs %v on Haiku and %v on Sonnet — Haiku is the "+
			"cheaper model", haiku, sonnet)
	}

	// And a provider with no cache price is charged nothing for one, rather
	// than Anthropic's.
	plain := estimateCostCents(ModelDeepSeekPro, 1000, 0, 0, 0)
	cached := estimateCostCents(ModelDeepSeekPro, 1000, 0, tokens, tokens)
	if math.Abs(plain-cached) > 1e-9 {
		t.Errorf("a DeepSeek request with cache tokens cost %v against %v without — "+
			"that difference is Anthropic's cache rate on somebody else's bill", cached, plain)
	}
}

// Found is not the same as free.
//
// The old lookup used InputPerM == 0 to mean "no entry", so a model priced at
// zero — some hosted tiers are — would have been silently charged Sonnet's rate.
func TestAFreeModelIsNotChargedTheFallback(t *testing.T) {
	const free = "test/free-model"
	knownPricing[free] = modelPricing{}
	t.Cleanup(func() { delete(knownPricing, free) })

	if p, ok := priceOf(free); !ok {
		t.Fatal("a model in the table was not found")
	} else if p != (modelPricing{}) {
		t.Errorf("a free model priced %v", p)
	}
	if got := estimateCostCents(free, 1_000_000, 1_000_000, 0, 0); got != 0 {
		t.Errorf("a free model cost %v cents", got)
	}
}

// Usage is filed under the provider that served it.
//
// Gemini fell through to the "claude" default — an id with no slash and no
// Atlas word — so every Gemini question was reported as Anthropic spend on the
// page an operator reads to see where the money went.
func TestUsageIsFiledUnderTheRightProvider(t *testing.T) {
	for model, want := range map[string]string{
		ModelClaudeSonnet:  "claude",
		ModelGeminiPro:     "gemini",
		ModelGeminiFlash:   "gemini",
		ModelDeepSeekPro:   "atlas",
		ModelQwenPlus:      "atlas",
		ModelOpenRouter:    "openrouter",
		"claude-opus-4-5":  "claude",
		"anthropic/claude": "openrouter",
	} {
		if got := providerName(model); got != want {
			t.Errorf("%s is filed under %q, want %q", model, got, want)
		}
	}
}
