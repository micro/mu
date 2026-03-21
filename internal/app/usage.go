package app

import (
	"sort"
	"sync"
	"time"

	"mu/internal/data"
)

// UsageRecord tracks a single external API call with its cost.
type UsageRecord struct {
	Service  string            `json:"service"`  // e.g. "claude", "google_places", "brave", "fetch"
	Caller   string            `json:"caller"`   // which feature triggered this
	CostCents float64          `json:"cost_cents"`
	Details  map[string]any    `json:"details,omitempty"` // service-specific (tokens, model, etc.)
	Timestamp time.Time        `json:"timestamp"`
}

// ServiceUsage summarises usage for a single service.
type ServiceUsage struct {
	Service    string  `json:"service"`
	Calls      int     `json:"calls"`
	CostCents  float64 `json:"cost_cents"`
}

// UsageSummary is the full usage report across all services.
type UsageSummary struct {
	Since       time.Time      `json:"since"`
	TotalCalls  int            `json:"total_calls"`
	TotalCost   float64        `json:"total_cost_cents"`
	ByService   []ServiceUsage `json:"by_service"`
	RecentCalls []UsageRecord  `json:"recent_calls"`
}

type persistedUsage struct {
	Since   time.Time     `json:"since"`
	Records []UsageRecord `json:"records"`
}

const usageFile = "usage.json"
const maxUsageRecords = 2000

var (
	usageMu      sync.Mutex
	usageRecords []UsageRecord
	usageStarted time.Time
)

func init() {
	var stored persistedUsage
	// Try new file first, fall back to legacy ai_usage.json
	if err := data.LoadJSON(usageFile, &stored); err == nil && len(stored.Records) > 0 {
		usageRecords = stored.Records
		usageStarted = stored.Since
	} else if err := data.LoadJSON("ai_usage.json", &stored); err == nil && len(stored.Records) > 0 {
		usageRecords = stored.Records
		usageStarted = stored.Since
		// Migrate: save as new file
		saveUsage()
	} else {
		usageStarted = time.Now()
	}
}

// RecordUsage records a cost-bearing external API call.
func RecordUsage(service, caller string, costCents float64, details map[string]any) {
	record := UsageRecord{
		Service:   service,
		Caller:    caller,
		CostCents: costCents,
		Details:   details,
		Timestamp: time.Now(),
	}

	usageMu.Lock()
	defer usageMu.Unlock()

	usageRecords = append(usageRecords, record)
	if len(usageRecords) > maxUsageRecords {
		usageRecords = usageRecords[len(usageRecords)-maxUsageRecords:]
	}

	saveUsage()
}

func saveUsage() {
	data.SaveJSON(usageFile, persistedUsage{
		Since:   usageStarted,
		Records: usageRecords,
	})
}

// GetUsageSummary returns a summary of all usage across services.
func GetUsageSummary() UsageSummary {
	usageMu.Lock()
	defer usageMu.Unlock()

	byServiceMap := make(map[string]*ServiceUsage)
	var totalCost float64

	for _, r := range usageRecords {
		su, ok := byServiceMap[r.Service]
		if !ok {
			su = &ServiceUsage{Service: r.Service}
			byServiceMap[r.Service] = su
		}
		su.Calls++
		su.CostCents += r.CostCents
		totalCost += r.CostCents
	}

	var byService []ServiceUsage
	for _, su := range byServiceMap {
		byService = append(byService, *su)
	}
	sort.Slice(byService, func(i, j int) bool {
		return byService[i].CostCents > byService[j].CostCents
	})

	// Last 30 calls
	var recent []UsageRecord
	start := len(usageRecords) - 30
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
		ByService:   byService,
		RecentCalls: recent,
	}
}
