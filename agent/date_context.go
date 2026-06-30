package agent

import (
	"fmt"
	"time"
)

// currentDateContext gives synthesis prompts one canonical request date to use
// for today/latest/current answers. It includes an ISO date so model output is
// anchored even when weekday or locale formatting would otherwise drift.
func currentDateContext(now time.Time) string {
	now = now.UTC()
	return fmt.Sprintf("%s (%s, UTC)", now.Format("Monday, 2 January 2006"), now.Format("2006-01-02"))
}

func withCurrentDateContext(text string) string {
	if text == "" {
		return "Current request date: " + currentDateContext(time.Now().UTC()) + "."
	}
	return "Current request date: " + currentDateContext(time.Now().UTC()) + ".\n" + text
}
