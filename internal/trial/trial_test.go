package trial

import (
	"os"
	"testing"
)

func reset() {
	dayMu.Lock()
	dayStamp, dayCount = "", 0
	dayMu.Unlock()
}

// What a demonstration is worth is an operator's decision, so the ceiling is a
// setting — and zero is a real answer, not a missing one. An instance somebody
// runs for themselves has nobody to demonstrate to.
func TestTheDailyCeilingIsAnOperatorsSetting(t *testing.T) {
	for _, tt := range []struct {
		set  string
		want int
	}{
		{"", 500},
		{"12", 12},
		{"0", 0},
		{"  40  ", 40},
		{"lots", 500}, // unreadable falls back rather than turning it off
		{"-3", 500},   // and so does nonsense, for the same reason
	} {
		os.Setenv("TRIAL_DAILY_TOTAL", tt.set)
		if got := dailyTotal(); got != tt.want {
			t.Errorf("TRIAL_DAILY_TOTAL=%q gave %d, want %d", tt.set, got, tt.want)
		}
	}
	os.Unsetenv("TRIAL_DAILY_TOTAL")
}

// The ceiling is the instance's, not one person's: it counts every free
// exchange given away today and stops when they are gone.
func TestTheCeilingCountsEveryExchange(t *testing.T) {
	os.Setenv("TRIAL_DAILY_TOTAL", "3")
	defer os.Unsetenv("TRIAL_DAILY_TOTAL")
	reset()

	for i := 1; i <= 3; i++ {
		if !dayTaken() {
			t.Fatalf("exchange %d was refused with room left", i)
		}
	}
	if dayTaken() {
		t.Error("a fourth exchange got through a ceiling of three")
	}
}

// Zero gives nothing away, without any other setting being needed.
func TestAZeroCeilingGivesNothingAway(t *testing.T) {
	os.Setenv("TRIAL_DAILY_TOTAL", "0")
	defer os.Unsetenv("TRIAL_DAILY_TOTAL")
	reset()

	if dayTaken() {
		t.Error("a ceiling of zero still gave an exchange away")
	}
}

// Tomorrow it works again, which is what the refusal says. A counter that never
// reset would make "try again tomorrow" a lie after the first busy day.
func TestTheCeilingResetsWithTheDay(t *testing.T) {
	os.Setenv("TRIAL_DAILY_TOTAL", "1")
	defer os.Unsetenv("TRIAL_DAILY_TOTAL")
	reset()

	if !dayTaken() || dayTaken() {
		t.Fatal("the ceiling of one is not being applied")
	}

	// Roll the day over the way midnight does.
	dayMu.Lock()
	dayStamp = "1999-12-31"
	dayMu.Unlock()

	if !dayTaken() {
		t.Error("the ceiling did not reset with the day, so tomorrow never comes")
	}
}
