package sms

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"mu/internal/origin"
	"mu/internal/quota"
	"mu/internal/userdb"
)

var welcomeMu sync.Mutex

// welcomeReply is deterministic onboarding, not a model call or an account.
// Only called after provider signature verification. The persisted daily budget
// survives restarts and limits paid replies without limiting shared visitor IPs.
func welcomeReply(channel Channel, number string) string {
	base := origin.Self()
	if base == "" || OptedOut(number) || !ConfiguredFor(channel) || defaultLimitOn(channel) == 0 || quota.DailyLimit(opFor(channel)) == 0 {
		return ""
	}
	if channel == ChannelSMS && !countryAllowed(number) {
		return ""
	}
	welcomeMu.Lock()
	defer welcomeMu.Unlock()
	day := time.Now().UTC().Format("2006-01-02")
	records, err := userdb.List(ns, instance, "welcomes", "mine", map[string]interface{}{"day": day}, "", "", 1)
	if err != nil {
		return ""
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(string(channel)+":"+number)))
	seen := map[string]interface{}{}
	if len(records) > 0 {
		if saved, ok := records[0].Data["senders"].(map[string]interface{}); ok {
			seen = saved
		}
	}
	cap := 200
	for _, limit := range []int{defaultLimitOn(channel), quota.DailyLimit(opFor(channel))} {
		if limit > 0 && limit < cap {
			cap = limit
		}
	}
	if _, sent := seen[key]; sent || len(seen) >= cap {
		return ""
	}
	seen[key] = true
	data := map[string]interface{}{"day": day, "senders": seen}
	if len(records) > 0 {
		_, err = userdb.Update(ns, instance, "welcomes", records[0].ID, data, false)
	} else {
		_, err = userdb.Create(ns, instance, "welcomes", data, false)
	}
	if err != nil {
		return ""
	}
	return "Welcome to Micro. Create an account or sign in at " + base + "/account#phone, then verify this phone number. Once connected, send your message again here to speak to your assistant."
}
