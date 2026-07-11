package discord

import (
	"sync"
	"time"

	"mu/internal/data"
)

type UserUsage struct {
	Queries    int       `json:"queries"`
	LastQuery  time.Time `json:"last_query"`
	DailyCount int       `json:"daily_count"`
	DayReset   string    `json:"day_reset"`
}

var (
	usageMu sync.Mutex
	usage   = map[string]*UserUsage{} // mu account ID → usage
)

func loadUsage() {
	data.LoadJSON("discord_usage.json", &usage)
}

func trackQuery(accountID string) {
	usageMu.Lock()
	defer usageMu.Unlock()

	today := time.Now().Format("2006-01-02")

	u, ok := usage[accountID]
	if !ok {
		u = &UserUsage{}
		usage[accountID] = u
	}

	u.Queries++
	u.LastQuery = time.Now()

	if u.DayReset != today {
		u.DailyCount = 0
		u.DayReset = today
	}
	u.DailyCount++

	data.SaveJSON("discord_usage.json", usage)
}

// GetUsageStats returns usage stats for the admin dashboard.
func GetUsageStats() map[string]*UserUsage {
	usageMu.Lock()
	defer usageMu.Unlock()

	result := make(map[string]*UserUsage, len(usage))
	for k, v := range usage {
		copy := *v
		result[k] = &copy
	}
	return result
}

// GetUserUsage returns usage for a specific user.
func GetUserUsage(accountID string) *UserUsage {
	usageMu.Lock()
	defer usageMu.Unlock()
	if u, ok := usage[accountID]; ok {
		copy := *u
		return &copy
	}
	return &UserUsage{}
}
