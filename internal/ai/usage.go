package ai

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// UsageRecord tracks token usage for a single API call
type UsageRecord struct {
	Caller               string    `json:"caller"`
	Model                string    `json:"model"`
	InputTokens          int       `json:"input_tokens"`
	OutputTokens         int       `json:"output_tokens"`
	CacheReadTokens      int       `json:"cache_read_tokens"`
	CacheCreationTokens  int       `json:"cache_creation_tokens"`
	EstimatedCostCents   float64   `json:"estimated_cost_cents"`
	Timestamp            time.Time `json:"timestamp"`
}

// CallerUsage summarises usage for a single caller
type CallerUsage struct {
	Caller              string  `json:"caller"`
	Calls               int     `json:"calls"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	TotalCostCents      float64 `json:"total_cost_cents"`
}

// UsageSummary is the full usage report
type UsageSummary struct {
	Since       time.Time     `json:"since"`
	TotalCalls  int           `json:"total_calls"`
	TotalCost   float64       `json:"total_cost_cents"`
	ByCaller    []CallerUsage `json:"by_caller"`
	RecentCalls []UsageRecord `json:"recent_calls"`
}

var (
	usageMu      sync.Mutex
	usageRecords []UsageRecord
	usageStarted = time.Now()
)

// Sonnet pricing (per million tokens) as of 2025
const (
	sonnetInputPricePerM         = 3.0   // $3 per 1M input tokens
	sonnetOutputPricePerM        = 15.0  // $15 per 1M output tokens
	sonnetCacheWritePricePerM    = 3.75  // $3.75 per 1M cache write tokens
	sonnetCacheReadPricePerM     = 0.30  // $0.30 per 1M cache read tokens
)

func estimateCostCents(inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) float64 {
	cost := float64(inputTokens) / 1_000_000 * sonnetInputPricePerM
	cost += float64(outputTokens) / 1_000_000 * sonnetOutputPricePerM
	cost += float64(cacheCreationTokens) / 1_000_000 * sonnetCacheWritePricePerM
	cost += float64(cacheReadTokens) / 1_000_000 * sonnetCacheReadPricePerM
	return cost * 100 // convert to cents
}

func recordUsage(caller, model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) {
	record := UsageRecord{
		Caller:              caller,
		Model:               model,
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
		EstimatedCostCents:  estimateCostCents(inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens),
		Timestamp:           time.Now(),
	}

	usageMu.Lock()
	defer usageMu.Unlock()

	usageRecords = append(usageRecords, record)

	// Keep max 2000 records (rolling window)
	if len(usageRecords) > 2000 {
		usageRecords = usageRecords[len(usageRecords)-2000:]
	}
}

// GetUsageSummary returns a summary of all API usage since startup
func GetUsageSummary() UsageSummary {
	usageMu.Lock()
	defer usageMu.Unlock()

	byCallerMap := make(map[string]*CallerUsage)
	var totalCost float64

	for _, r := range usageRecords {
		cu, ok := byCallerMap[r.Caller]
		if !ok {
			cu = &CallerUsage{Caller: r.Caller}
			byCallerMap[r.Caller] = cu
		}
		cu.Calls++
		cu.InputTokens += r.InputTokens
		cu.OutputTokens += r.OutputTokens
		cu.CacheReadTokens += r.CacheReadTokens
		cu.CacheCreationTokens += r.CacheCreationTokens
		cu.TotalCostCents += r.EstimatedCostCents
		totalCost += r.EstimatedCostCents
	}

	var byCaller []CallerUsage
	for _, cu := range byCallerMap {
		byCaller = append(byCaller, *cu)
	}
	sort.Slice(byCaller, func(i, j int) bool {
		return byCaller[i].TotalCostCents > byCaller[j].TotalCostCents
	})

	// Last 20 calls for the recent calls view
	var recent []UsageRecord
	start := len(usageRecords) - 20
	if start < 0 {
		start = 0
	}
	for i := len(usageRecords) - 1; i >= start; i-- {
		recent = append(recent, usageRecords[i])
	}

	return UsageSummary{
		Since:       usageStarted,
		TotalCalls:  len(usageRecords),
		TotalCost:   totalCost,
		ByCaller:    byCaller,
		RecentCalls: recent,
	}
}

// GetUsageLine returns a one-line summary for status display
func GetUsageLine() string {
	s := GetUsageSummary()
	return fmt.Sprintf("%d calls, est $%.2f since %s",
		s.TotalCalls, s.TotalCost/100, s.Since.Format("15:04"))
}
