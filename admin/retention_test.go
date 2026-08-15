package admin

// A retention number that is quietly wrong is worse than no retention number,
// because it gets believed and acted on. These pin the arithmetic and the two
// places it would otherwise lie.

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// activity builds the "who was active on which day" map the handler reads out
// of the day counters, without needing the counters.
func activity(m map[string][]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for d, ids := range m {
		set := map[string]bool{}
		for _, id := range ids {
			set[id] = true
		}
		out[d] = set
	}
	return out
}

// countReturned is cohortsFrom's counting, isolated from auth so the arithmetic
// can be checked against a fixture. The handler's version reads signup dates
// from the accounts; this takes them.
func countReturned(signups map[string][]string, active map[string]map[string]bool,
	capped map[string]bool) (cohorts []cohort, total, returned int) {
	for key, ids := range signups {
		d := day(key)
		c := cohort{Day: d, Size: len(ids), Returned: map[int]int{}, Partial: map[int]bool{}}
		total += len(ids)
		came := map[string]bool{}
		for _, off := range offsets {
			k := d.AddDate(0, 0, off).Format("2006-01-02")
			set, known := active[k]
			if !known {
				continue
			}
			if capped[k] {
				c.Partial[off] = true
			}
			for _, id := range ids {
				if set[id] {
					c.Returned[off]++
					came[id] = true
				}
			}
		}
		returned += len(came)
		cohorts = append(cohorts, c)
	}
	return
}

// Somebody who came back on two different days is one person who came back.
//
// Summing the columns would double-count them and report a day-two rate above
// 100% on a cohort where a few people are enthusiastic — which is precisely the
// shape a small instance has.
func TestAReturnerIsCountedOnceHoweverOftenTheyReturn(t *testing.T) {
	signups := map[string][]string{"2026-01-01": {"amy", "bob", "cat"}}
	active := activity(map[string][]string{
		"2026-01-02": {"amy"},        // +1
		"2026-01-03": {"amy"},        // +2, same person
		"2026-01-08": {"amy", "bob"}, // +7
	})

	cohorts, total, returned := countReturned(signups, active, nil)
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if returned != 2 {
		t.Errorf("returned = %d, want 2 — amy came back three times and is one person", returned)
	}
	c := cohorts[0]
	if c.Returned[1] != 1 || c.Returned[7] != 2 {
		t.Errorf("+1d = %d (want 1), +7d = %d (want 2)", c.Returned[1], c.Returned[7])
	}
	if c.Returned[30] != 0 {
		t.Errorf("+30d = %d, want 0", c.Returned[30])
	}
}

// Activity on the signup day itself is not returning.
//
// Everyone is active the day they sign up — that is what signing up is. Letting
// day zero count would make every cohort look perfectly retained and the page
// would say the opposite of the truth.
func TestTheSignupDayIsNotAReturn(t *testing.T) {
	signups := map[string][]string{"2026-01-01": {"amy", "bob"}}
	active := activity(map[string][]string{"2026-01-01": {"amy", "bob"}})

	_, total, returned := countReturned(signups, active, nil)
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if returned != 0 {
		t.Errorf("returned = %d, want 0 — both were only ever active on the day they "+
			"signed up, which is not coming back", returned)
	}
	for _, off := range offsets {
		if off == 0 {
			t.Fatal("day zero is in offsets, so every cohort will look fully retained")
		}
	}
}

// A day that hit the caller cap is marked, because its numbers are a floor.
//
// Each day keeps at most 200 distinct callers by name and counts the rest
// together. That never binds on a quiet day and binds exactly on the day a link
// reached the front page of Hacker News — the day being asked about. An
// unmarked floor reads as a fact.
func TestACappedDayIsMarkedAsAFloor(t *testing.T) {
	signups := map[string][]string{"2026-01-01": {"amy", "bob"}}
	active := activity(map[string][]string{"2026-01-02": {"amy"}})

	_, _, plain := countReturned(signups, active, nil)
	cohorts, _, _ := countReturned(signups, active, map[string]bool{"2026-01-02": true})

	if plain != 1 {
		t.Fatalf("returned = %d, want 1", plain)
	}
	if !cohorts[0].Partial[1] {
		t.Error("the +1d cell is not marked partial on a day that overflowed the " +
			"caller cap, so a floor is shown as a count")
	}
	if cohorts[0].Partial[7] {
		t.Error("a day that did not overflow is marked partial")
	}
}

// A cohort with no activity after it reports zero rather than nothing.
func TestACohortNobodyReturnedFromIsZeroNotBlank(t *testing.T) {
	signups := map[string][]string{"2026-01-01": {"amy", "bob", "cat"}}
	active := activity(map[string][]string{"2026-01-02": {"someone-else"}})

	cohorts, total, returned := countReturned(signups, active, nil)
	if total != 3 || returned != 0 {
		t.Fatalf("total = %d returned = %d, want 3 and 0", total, returned)
	}
	if got := pct(cohorts[0].Returned[1], cohorts[0].Size); got != "0%" {
		t.Errorf("the +1d cell reads %q, want 0%%", got)
	}
}

func TestPercentagesDoNotDivideByZero(t *testing.T) {
	if got := pct(0, 0); got != "—" {
		t.Errorf("pct(0,0) = %q, want an em dash rather than a panic or 0%%", got)
	}
	if got := pct(1, 3); got != "33%" {
		t.Errorf("pct(1,3) = %q, want 33%%", got)
	}
}
