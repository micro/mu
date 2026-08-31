package ai

// What a question cost.
//
// # Exact ids, not substrings
//
// This matched with strings.Contains over a map and iterated it to do so, and
// both halves of that were wrong in ways that only showed up as money.
//
// Contains meant a key became a trap the moment a newer id extended it.
// "claude-opus-4" is a prefix of claude-opus-4-5, -4-6, -4-7 and -4-8, so four
// current models were billed at the retired Opus 4 rate — three times over. And
// it silently missed in the other direction: "claude-sonnet-5" does not contain
// "claude-sonnet-4", so the interactive default matched nothing at all.
//
// Iterating a map and breaking on the first hit meant that if any id ever
// matched two keys, the price was whichever key Go's randomised map order
// reached first — a different answer between runs of the same binary.
//
// So: exact ids, one entry per model this instance can actually ask for, and a
// lookup that says whether it found one. Adding a model means adding its price,
// and TestEveryModelWeAskForHasAPrice fails until you do — which is the part
// that was missing, because every miss until now was silent.
//
// # Found is not the same as free
//
// The old lookup used InputPerM == 0 to mean "not found", so a genuinely free
// model — some hosted Qwen and GLM tiers are zero — would have been charged
// Sonnet's price. The bool separates the two.
//
// # Cache rates belong to the model
//
// They were two constants, 3.75 and 0.30, added to every request that reported
// cache tokens whoever served it. Those are Sonnet 4.5's numbers. Applied to
// Haiku they are three times too high; applied to a DeepSeek request they are
// Anthropic's prices on somebody else's bill.
//
// Anthropic's structure is regular — cache write is 1.25x input, cache read is
// 0.1x — and every row below satisfies it. They are written out rather than
// derived anyway, because the whole of this file's history is a hidden constant
// that nobody could see was wrong. A provider that does not price a cache
// leaves them zero and is charged nothing for one.
//
// The 1h cache tier is a second write rate (2x input) that this cannot tell
// apart, because nothing records which TTL was used. The 5m tier is the one
// stored, which is the cheaper of the two and the one a short conversation
// actually uses.
//
// Prices are per million tokens, in US dollars, from each provider's own page.
// They go stale; TestEveryModelWeAskForHasAPrice keeps the table complete and
// nothing can keep it current, so it is worth re-reading when a model is added.

import (
	"sort"
	"strings"
	"sync"

	"mu/internal/app"
)

// modelPricing is what one model costs, per million tokens.
type modelPricing struct {
	InputPerM  float64
	OutputPerM float64
	// CacheWritePerM is the 5-minute tier. Zero for a provider that does not
	// charge for a cache, which is charged nothing rather than Anthropic's rate.
	CacheWritePerM float64
	CacheReadPerM  float64
}

// knownPricing is keyed on the exact model id, as sent to the provider.
var knownPricing = map[string]modelPricing{
	// Anthropic. Current first, then the retired ones, which are kept because a
	// conversation recorded before an upgrade still has to price correctly.
	"claude-opus-5":             {5.0, 25.0, 6.25, 0.50},
	"claude-opus-4-8":           {5.0, 25.0, 6.25, 0.50},
	"claude-opus-4-7":           {5.0, 25.0, 6.25, 0.50},
	"claude-opus-4-6":           {5.0, 25.0, 6.25, 0.50},
	"claude-opus-4-5":           {5.0, 25.0, 6.25, 0.50},
	"claude-opus-4-1":           {15.0, 75.0, 18.75, 1.50},
	"claude-opus-4":             {15.0, 75.0, 18.75, 1.50},
	"claude-sonnet-5":           {2.0, 10.0, 2.50, 0.20},
	"claude-sonnet-4-6":         {3.0, 15.0, 3.75, 0.30},
	"claude-sonnet-4-5":         {3.0, 15.0, 3.75, 0.30},
	"claude-sonnet-4":           {3.0, 15.0, 3.75, 0.30},
	"claude-haiku-4-5-20251001": {1.0, 5.0, 1.25, 0.10},
	"claude-haiku-4-5":          {1.0, 5.0, 1.25, 0.10},
	"claude-3-5-haiku":          {0.80, 4.0, 1.0, 0.08},

	// Atlas Cloud. No cache rate recorded: what they charge for one is small
	// enough to be noise and is not Anthropic's, which is what mattered.
	"deepseek-ai/deepseek-v4-pro":   {1.68, 3.38, 0, 0},
	"deepseek-ai/deepseek-v4-flash": {0.14, 0.28, 0, 0},
	"deepseek-ai/deepseek-v3":       {0.26, 0.38, 0, 0},
	"qwen/qwen3.6-plus":             {0.33, 1.95, 0, 0},

	// Google.
	"gemini-pro-latest":   {1.25, 10.0, 0, 0.31},
	"gemini-flash-latest": {0.30, 2.50, 0, 0.075},

	// OpenRouter passes through to whoever serves the id, and its own default
	// here is OpenAI's cheapest.
	"openai/gpt-4o-mini": {0.15, 0.60, 0, 0.075},
}

// priceOf is what this model costs, and whether we know.
//
// Exact first, because an id we ask for on purpose should never be approximated.
// Then longest prefix, which is how a dated id — claude-haiku-4-5-20251001 —
// finds its undated family entry, and how anything a provider appends a suffix
// to keeps working. Sorted longest first so the answer does not depend on map
// order: the bug the old lookup had was that it did.
func priceOf(model string) (modelPricing, bool) {
	m := strings.TrimSpace(model)
	if p, ok := knownPricing[m]; ok {
		return p, true
	}

	keys := make([]string, 0, len(knownPricing))
	for k := range knownPricing {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if strings.HasPrefix(m, k) {
			return knownPricing[k], true
		}
	}
	return modelPricing{}, false
}

// fallbackPricing is what an unknown model is charged.
//
// Sonnet's, which is the middle of the range and was the old default. It is a
// guess and is named as one: the point of priceOf returning a bool is that this
// is reached only by a model nobody added, and the table test exists so that is
// never a model this instance asks for on purpose.
var fallbackPricing = modelPricing{3.0, 15.0, 3.75, 0.30}

func estimateCostCents(model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) float64 {
	pricing, ok := priceOf(model)
	if !ok {
		pricing = fallbackPricing
		unpricedModel.Do(func() {
			app.Log("ai", "no price for model %q, so usage is estimated at Sonnet's "+
				"rate — add it to knownPricing", model)
		})
	}

	cost := float64(inputTokens) / 1_000_000 * pricing.InputPerM
	cost += float64(outputTokens) / 1_000_000 * pricing.OutputPerM
	// The model's own cache rates, and nothing when it has none. These were two
	// constants at Sonnet's price added to every provider's bill.
	cost += float64(cacheCreationTokens) / 1_000_000 * pricing.CacheWritePerM
	cost += float64(cacheReadTokens) / 1_000_000 * pricing.CacheReadPerM
	return cost * 100 // convert to cents
}

// unpricedModel keeps that warning to once per process rather than once per
// question.
var unpricedModel sync.Once

// providerName returns a display name for the provider based on model string.
//
// Gemini was missing, so every Gemini question was filed under "claude" — the
// last branch is a default and an id with no slash and no Atlas word fell into
// it. Usage by provider is what an operator reads to see where the money went.
func providerName(model string) string {
	if strings.Contains(model, "deepseek") || strings.Contains(model, "qwen") ||
		strings.Contains(model, "Qwen") || strings.Contains(model, "glm") ||
		strings.Contains(model, "kimi") {
		return "atlas"
	}
	if strings.HasPrefix(model, "gemini") {
		return "gemini"
	}
	if strings.Contains(model, "/") {
		return "openrouter"
	}
	return "claude"
}

func recordUsage(caller, model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) {
	costCents := estimateCostCents(model, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)

	app.RecordUsage(providerName(model), caller, costCents, map[string]any{
		"model":                 model,
		"input_tokens":          inputTokens,
		"output_tokens":         outputTokens,
		"cache_read_tokens":     cacheReadTokens,
		"cache_creation_tokens": cacheCreationTokens,
	})
}
