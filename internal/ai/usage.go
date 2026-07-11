package ai

import (
	"strings"

	"mu/internal/app"
)

// Model pricing per million tokens.
type modelPricing struct {
	InputPerM  float64
	OutputPerM float64
}

var knownPricing = map[string]modelPricing{
	// Anthropic
	"claude-opus-4":   {15.0, 75.0},
	"claude-sonnet-4": {3.0, 15.0},
	"claude-haiku-4":  {0.80, 4.0},
	// Atlas Cloud / DeepSeek
	"deepseek-v4-pro":   {1.68, 3.38},
	"deepseek-v4-flash": {0.14, 0.28},
	"deepseek-v3":       {0.26, 0.38},
	// Atlas Cloud / Qwen
	"qwen3.6-plus": {0.33, 1.95},
}

func estimateCostCents(model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) float64 {
	// Find pricing by matching model prefix.
	var pricing modelPricing
	for prefix, p := range knownPricing {
		if strings.Contains(model, prefix) {
			pricing = p
			break
		}
	}
	if pricing.InputPerM == 0 {
		// Default to Sonnet pricing as fallback.
		pricing = modelPricing{3.0, 15.0}
	}

	cost := float64(inputTokens) / 1_000_000 * pricing.InputPerM
	cost += float64(outputTokens) / 1_000_000 * pricing.OutputPerM
	// Anthropic cache pricing (only relevant for Claude).
	if cacheCreationTokens > 0 {
		cost += float64(cacheCreationTokens) / 1_000_000 * 3.75
	}
	if cacheReadTokens > 0 {
		cost += float64(cacheReadTokens) / 1_000_000 * 0.30
	}
	return cost * 100 // convert to cents
}

// providerName returns a display name for the provider based on model string.
func providerName(model string) string {
	if strings.Contains(model, "deepseek") || strings.Contains(model, "qwen") ||
		strings.Contains(model, "Qwen") || strings.Contains(model, "glm") ||
		strings.Contains(model, "kimi") {
		return "atlas"
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
