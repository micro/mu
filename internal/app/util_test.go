package app

import (
	"testing"
	"time"
)

func TestDistanceOfTime(t *testing.T) {
	tests := []struct {
		name     string
		minutes  float64
		expected string
	}{
		{"Less than 1 second", 0.001, "1 sec"},
		{"1 second", 1.0 / 60.0, "1 sec"},
		{"30 seconds", 0.5, "30 secs"},
		{"1 minute", 1.5, "1 minute"},
		{"5 minutes", 5, "5 minutes"},
		{"59 minutes", 59, "59 minutes"},
		{"1 hour", 60, "1 hour"},
		{"2 hours", 120, "2 hours"},
		{"23 hours", 1380, "23 hours"},
		{"1 day", 1440, "1 day"},
		{"2 days", 2880, "2 days"},
		{"30 days", 43200, "30 days"},
		{"1 month", 43800, "1 month"},
		{"2 months", 87600, "2 months"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := distanceOfTime(tt.minutes)
			if result != tt.expected {
				t.Errorf("distanceOfTime(%v) = %q, want %q", tt.minutes, result, tt.expected)
			}
		})
	}
}

func TestTimeAgo_ZeroTime(t *testing.T) {
	result := TimeAgo(time.Time{})
	if result != "just now" {
		t.Errorf("TimeAgo(zero) = %q, want %q", result, "just now")
	}
}

func TestTimeAgo_RecentTime(t *testing.T) {
	result := TimeAgo(time.Now().Add(-5 * time.Minute))
	if result != "5 minutes ago" {
		t.Errorf("TimeAgo(5 min ago) = %q, want %q", result, "5 minutes ago")
	}
}

func TestTimeAgo_OldTime(t *testing.T) {
	// More than 363 days ago should show date format
	old := time.Now().Add(-400 * 24 * time.Hour)
	result := TimeAgo(old)
	expected := old.Format("2 Jan")
	if result != expected {
		t.Errorf("TimeAgo(old) = %q, want %q", result, expected)
	}
}

func TestTimeAgo_OneHourAgo(t *testing.T) {
	result := TimeAgo(time.Now().Add(-61 * time.Minute))
	if result != "1 hour ago" {
		t.Errorf("TimeAgo(1h ago) = %q, want %q", result, "1 hour ago")
	}
}

func TestTimeAgo_OneDayAgo(t *testing.T) {
	result := TimeAgo(time.Now().Add(-25 * time.Hour))
	if result != "1 day ago" {
		t.Errorf("TimeAgo(1d ago) = %q, want %q", result, "1 day ago")
	}
}
