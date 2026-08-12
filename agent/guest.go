package agent

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/service"
	"mu/internal/settings"
)

// The demonstration, and what bounds it.
//
// A guest gets a few agent queries so the landing page can show the tools being
// used rather than described — the ollama run llama3 argument, and the reason
// the front page is not a brochure.
//
// It is not a free tier, and the difference is the second limit. A per-IP
// allowance is unbounded in aggregate: it costs whatever arrives, and the busier
// the day the more it costs, which is exactly the shape the pricing argument
// rejects. An instance-wide ceiling on top makes it a marketing budget instead —
// a fixed number of agent queries a day spent on showing people what this is,
// after which guests are asked to sign up and nothing else changes.
//
// So: a per-IP cap so no one visitor takes the lot, and a per-day total so the
// spend has a ceiling an operator chose.
const guestDailyLimit = 3

// guestDailyTotal is the instance-wide ceiling, overridable because what a
// demonstration is worth is an operator's decision. Zero turns guest queries
// off entirely, which is the right setting for an instance somebody runs for
// themselves.
func guestDailyTotal() int {
	v := strings.TrimSpace(settings.Get("GUEST_DAILY_TOTAL"))
	if v == "" {
		return 300
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 300
	}
	return n
}

// guestExtraTools are the tools a guest may use that have no service behind
// them, so service.GuestAllowedTool cannot answer for them. Everything that is
// service-backed is derived from whether that service is account-scoped.
var guestExtraTools = map[string]bool{
	"quran":         true,
	"quran_search":  true,
	"hadith":        true,
	"blog_read":     true,
	"social_search": true,
	"video_search":  true,
	"apps_run":      true,
}

func isGuestAllowedTool(name string) bool {
	return service.GuestAllowedTool(name) || guestExtraTools[name]
}

var (
	guestMu     sync.Mutex
	guestCounts = map[string]*guestBucket{}

	// The instance-wide count for the current day.
	guestDay   string
	guestToday int
)

type guestBucket struct {
	count   int
	resetAt time.Time
}

func guestQueryAllowed(ip string) bool {
	guestMu.Lock()
	defer guestMu.Unlock()
	rollGuestDay()

	if guestToday >= guestDailyTotal() {
		return false
	}

	b, ok := guestCounts[ip]
	if !ok || time.Now().After(b.resetAt) {
		return true
	}
	return b.count < guestDailyLimit
}

// GuestQueriesLeft is how many the instance will still spend today, and whether
// there is a ceiling at all. For the landing page, which should not offer a
// demonstration it cannot give.
func GuestQueriesLeft() (int, bool) {
	guestMu.Lock()
	defer guestMu.Unlock()
	rollGuestDay()
	total := guestDailyTotal()
	if left := total - guestToday; left > 0 {
		return left, true
	}
	return 0, true
}

// rollGuestDay resets the instance-wide count when the date changes. Called
// with the lock. A date comparison rather than a timer, for the reason quota
// gives: the process may be asleep at midnight.
func rollGuestDay() {
	if d := time.Now().UTC().Format("2006-01-02"); d != guestDay {
		guestDay, guestToday = d, 0
	}
}

func guestQueryRecord(ip string) {
	guestMu.Lock()
	defer guestMu.Unlock()
	rollGuestDay()
	guestToday++

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
