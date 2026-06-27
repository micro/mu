package agent

import (
	"sync"
	"time"
)

const guestDailyLimit = 3

var guestAllowedTools = map[string]bool{
	"news":            true,
	"news_headlines":  true,
	"news_read":       true,
	"news_search":     true,
	"markets":         true,
	"weather_forecast": true,
	"video":           true,
	"video_search":    true,
	"web_search":      true,
	"web_fetch":       true,
	"social":          true,
	"social_search":   true,
	"blog_list":       true,
	"blog_read":       true,
	"apps_search":     true,
	"apps_read":       true,
	"search":          true,
	"reminder":        true,
	"quran":           true,
	"hadith":          true,
	"quran_search":    true,
	"stream":          true,
	"places_search":   true,
	"places_nearby":   true,
}

func isGuestAllowedTool(name string) bool {
	return guestAllowedTools[name]
}

var (
	guestMu     sync.Mutex
	guestCounts = map[string]*guestBucket{}
)

type guestBucket struct {
	count   int
	resetAt time.Time
}

func guestQueryAllowed(ip string) bool {
	guestMu.Lock()
	defer guestMu.Unlock()

	b, ok := guestCounts[ip]
	if !ok || time.Now().After(b.resetAt) {
		return true
	}
	return b.count < guestDailyLimit
}

func guestQueryRecord(ip string) {
	guestMu.Lock()
	defer guestMu.Unlock()

	b, ok := guestCounts[ip]
	if !ok || time.Now().After(b.resetAt) {
		guestCounts[ip] = &guestBucket{
			count:   1,
			resetAt: time.Now().Add(24 * time.Hour),
		}
		return
	}
	b.count++
}
