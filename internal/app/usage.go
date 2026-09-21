package app

import (
	"sort"
	"sync"
	"time"

	"mu/internal/data"
)

// UsageRecord tracks a single external API call with its cost.
type UsageRecord struct {
	Service   string         `json:"service"` // e.g. "claude", "google_places", "brave", "fetch"
	Caller    string         `json:"caller"`  // which feature triggered this
	CostCents float64        `json:"cost_cents"`
	Details   map[string]any `json:"details,omitempty"` // service-specific (tokens, model, etc.)
	Timestamp time.Time      `json:"timestamp"`
}

// ServiceUsage summarises usage for a single service.
type ServiceUsage struct {
	Service   string  `json:"service"`
	Calls     int     `json:"calls"`
	CostCents float64 `json:"cost_cents"`
}

// UsageSummary is the full usage report across all services.
type UsageSummary struct {
	Since       time.Time      `json:"since"`
	TotalCalls  int            `json:"total_calls"`
	TotalCost   float64        `json:"total_cost_cents"`
	ByService   []ServiceUsage `json:"by_service"`
	RecentCalls []UsageRecord  `json:"recent_calls"`
}

// DailyCost keeps provider estimates beyond the bounded recent-call log.
type DailyCost struct {
	Day       string  `json:"day"`
	Calls     int     `json:"calls"`
	CostCents float64 `json:"cost_cents"`
}

type persistedUsage struct {
	Accounts   map[string]map[string]DailyCost `json:"accounts,omitempty"`
	DailySince time.Time                       `json:"daily_since"`
	Daily      map[string]DailyCost            `json:"daily"`
	Since      time.Time                       `json:"since"`
	Records    []UsageRecord                   `json:"records"`
}

const usageFile = "usage.json"
const maxUsageRecords = 2000

// usageFlush is how long a cost record may sit in memory before it is on disk.
const usageFlush = 2 * time.Second

var (
	usageMu      sync.Mutex
	usageRecords []UsageRecord
	usageStarted time.Time
	usageDirty   bool
	dailySince   time.Time
	dailyCosts   = map[string]DailyCost{}
	accountCosts = map[string]map[string]DailyCost{}
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
		// Migrate: write it out under the new name now rather than waiting for
		// the first tick, so a restart before then does not read the legacy
		// file a second time.
		usageDirty = true
		flushUsage()
	} else {
		usageStarted = time.Now()
	}

	if stored.Accounts != nil {
		accountCosts = stored.Accounts
	}

	// Do not reconstruct whole days from a truncated recent-call log.
	dailySince = stored.DailySince
	if stored.Daily != nil {
		dailyCosts = stored.Daily
	}
	if dailySince.IsZero() {
		dailySince = time.Now().UTC()
		usageDirty = true
	}

	go func() {
		for range time.Tick(usageFlush) {
			flushUsage()
		}
	}()
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

	day := record.Timestamp.UTC().Format("2006-01-02")
	d := dailyCosts[day]
	d.Day = day
	d.Calls++
	d.CostCents += costCents
	dailyCosts[day] = d
	cutoff := record.Timestamp.UTC().AddDate(0, 0, -89).Format("2006-01-02")
	for key := range dailyCosts {
		if key < cutoff {
			delete(dailyCosts, key)
		}
	}
	if id, ok := details["account"].(string); ok && id != "" {
		if accountCosts[id] == nil {
			accountCosts[id] = map[string]DailyCost{}
		}
		d := accountCosts[id][day]
		d.Day = day
		d.Calls++
		d.CostCents += costCents
		accountCosts[id][day] = d
	}
	for id, days := range accountCosts {
		for key := range days {
			if key < cutoff {
				delete(days, key)
			}
		}
		if len(days) == 0 {
			delete(accountCosts, id)
		}
	}
	usageRecords = append(usageRecords, record)
	if len(usageRecords) > maxUsageRecords {
		usageRecords = usageRecords[len(usageRecords)-maxUsageRecords:]
	}

	usageDirty = true
}

// flushUsage writes the spend log if anything has been added to it.
//
// Every cost-bearing call used to write the whole file, in the call's own
// goroutine, holding the lock while it marshalled and wrote. Two thousand
// records is a hundred and forty kilobytes, so a busy minute is a hundred and
// forty kilobytes serialised and fsynced per external request, and every other
// recorder waiting behind it. The records are the same records a second later;
// what they are not is on the critical path of the thing being measured.
func flushUsage() {
	usageMu.Lock()
	if !usageDirty {
		usageMu.Unlock()
		return
	}
	days := make(map[string]DailyCost, len(dailyCosts))
	for day, cost := range dailyCosts {
		days[day] = cost
	}
	accounts := make(map[string]map[string]DailyCost, len(accountCosts))
	for id, days := range accountCosts {
		accounts[id] = make(map[string]DailyCost, len(days))
		for day, d := range days {
			accounts[id][day] = d
		}
	}
	snapshot := persistedUsage{
		Accounts:   accounts,
		DailySince: dailySince, Daily: days,
		Since:   usageStarted,
		Records: append([]UsageRecord(nil), usageRecords...),
	}
	usageDirty = false
	usageMu.Unlock()

	if err := data.SaveJSON(usageFile, snapshot); err != nil {
		usageMu.Lock()
		usageDirty = true
		usageMu.Unlock()
	}
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

// DailyCosts returns up to 90 UTC days of recorded provider estimates, newest
// first. Since marks when this collection began; its first day can be partial.
func DailyCosts() (since time.Time, days []DailyCost) {
	usageMu.Lock()
	defer usageMu.Unlock()
	for _, d := range dailyCosts {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Day > days[j].Day })
	return dailySince, days
}

// AccountCosts reports attributed model cost, not the complete cost to serve an
// account. Tool invoices, infrastructure, fees and tax are separate.
func AccountCosts() map[string]DailyCost {
	usageMu.Lock()
	defer usageMu.Unlock()
	out := map[string]DailyCost{}
	cutoff := time.Now().UTC().AddDate(0, 0, -29).Format("2006-01-02")
	for id, days := range accountCosts {
		d := DailyCost{}
		for day, cost := range days {
			if day >= cutoff {
				d.Calls += cost.Calls
				d.CostCents += cost.CostCents
			}
		}
		if d.Calls > 0 {
			out[id] = d
		}
	}
	return out
}

func ForgetAccountCosts(id string) {
	usageMu.Lock()
	defer usageMu.Unlock()
	delete(accountCosts, id)
	for i := range usageRecords {
		if usageRecords[i].Details["account"] == id {
			details := make(map[string]any, len(usageRecords[i].Details))
			for k, v := range usageRecords[i].Details {
				if k != "account" {
					details[k] = v
				}
			}
			usageRecords[i].Details = details
		}
	}
	usageDirty = true
}
