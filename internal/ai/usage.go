package ai

import (
	"mu/internal/app"
)

// Sonnet pricing (per million tokens) as of 2025
const (
	sonnetInputPricePerM      = 3.0  // $3 per 1M input tokens
	sonnetOutputPricePerM     = 15.0 // $15 per 1M output tokens
	sonnetCacheWritePricePerM = 3.75 // $3.75 per 1M cache write tokens
	sonnetCacheReadPricePerM  = 0.30 // $0.30 per 1M cache read tokens
)

func estimateCostCents(inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) float64 {
	cost := float64(inputTokens) / 1_000_000 * sonnetInputPricePerM
	cost += float64(outputTokens) / 1_000_000 * sonnetOutputPricePerM
	cost += float64(cacheCreationTokens) / 1_000_000 * sonnetCacheWritePricePerM
	cost += float64(cacheReadTokens) / 1_000_000 * sonnetCacheReadPricePerM
	return cost * 100 // convert to cents
}

func recordUsage(caller, model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) {
	costCents := estimateCostCents(inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)

	app.RecordUsage("claude", caller, costCents, map[string]any{
		"model":                 model,
		"input_tokens":          inputTokens,
		"output_tokens":         outputTokens,
		"cache_read_tokens":     cacheReadTokens,
		"cache_creation_tokens": cacheCreationTokens,
	})
}
