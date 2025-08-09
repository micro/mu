package video

import (
	"fmt"
	"time"
)

func timeAgo(d time.Time) string {
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
		return fmt.Sprintf("%d secs", int(minutes*60))
	case minutes < 59:
		return fmt.Sprintf("%d minutes", int(minutes))
	case minutes < 1440:
		return fmt.Sprintf("%d hours", int(minutes/60))
	case minutes < 2880:
		return fmt.Sprintf("%d day", int(minutes/1440))
	case minutes < 43800:
		return fmt.Sprintf("%d days", int(minutes/1440))
	case minutes < 87600:
		return fmt.Sprintf("%d month", int(minutes/43800))
	default:
		return fmt.Sprintf("%d months", int(minutes/43800))
	}

	return ""
}
