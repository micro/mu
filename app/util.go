package app

import (
	"fmt"
	"time"
)

func TimeAgo(d time.Time) string {
	// Handle zero time
	if d.IsZero() {
		return "just now"
	}

	timeAgo := ""
	startDate := time.Now().Unix()
	deltaMinutes := float64(startDate-d.Unix()) / 60.0
	if deltaMinutes <= 523440 { // less than 363 days
		timeAgo = fmt.Sprintf("%s ago", distanceOfTime(deltaMinutes))
	} else {
		timeAgo = d.Format("2 Jan")
	}

	return timeAgo
}

func distanceOfTime(minutes float64) string {
	switch {
	case minutes < 1:
		secs := int(minutes * 60)
		if secs < 1 {
			secs = 1
		}
		if secs == 1 {
			return "1 sec"
		}
		return fmt.Sprintf("%d secs", secs)
	case minutes < 2:
		return "1 minute"
	case minutes < 59:
		return fmt.Sprintf("%d minutes", int(minutes))
	case minutes < 100:
		hrs := int(minutes / 60)
		if hrs == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hrs)
	case minutes < 1440:
		return fmt.Sprintf("%d hours", int(minutes/60))
	case minutes < 2880:
		return "1 day"
	case minutes < 43800:
		return fmt.Sprintf("%d days", int(minutes/1440))
	case minutes < 87600:
		return "1 month"
	default:
		return fmt.Sprintf("%d months", int(minutes/43800))
	}

	return ""
}
