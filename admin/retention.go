package admin

// Whether anybody comes back.
//
// /admin/traffic answers how busy this instance is, and it answers it with
// call counts — which go up when one enthusiastic account hammers an endpoint
// and look identical to a hundred people arriving. After a front-page link on
// Hacker News that distinction is the only one that matters: thousands of
// developers saw the thing, and "traffic is low" afterwards means they came,
// looked and did not return. That is a different problem from nobody finding
// it, and it is not visible on a chart of totals.
//
// So this is the other question. Group accounts by the day they signed up, and
// ask how many of them did anything on the days after. A cohort table is the
// only shape that separates "we never got anyone" from "we get people and lose
// them", and those two have nothing in common as problems.
//
// Nothing new is stored. Signup dates come from the accounts themselves and
// activity from the day counters already kept for ninety days — this is a
// reading of what was there, not a new collection.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
)

// offsets are the days after signup a cohort is asked about. Day 1 is the one
// that decides everything — an account that never came back the next day did
// not find a reason to.
var offsets = []int{1, 2, 3, 7, 14, 30}

// cohortDays is how far back to group. The day ring keeps ninety, and a cohort
// needs room after it to have somewhere to return to, so the newest cohorts are
// young and their later columns are blank rather than zero.
const cohortDays = 90

type cohort struct {
	Day      time.Time
	Size     int
	Returned map[int]int // offset → how many of Size were active that day
	Partial  map[int]bool
}

// RetentionHandler shows /admin/retention.
func RetentionHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireAdmin(r); err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	active, capped := activeByDay()
	cohorts, total, returned := cohortsFrom(active, capped)

	var sb strings.Builder
	sb.WriteString(usage.CSS)
	sb.WriteString(retentionCSS)

	// The answer first. Everything below it is the working.
	sb.WriteString(`<div class="card"><div class="traffic-stats">`)
	usage.Stat(&sb, "Signed up (90 days)", total)
	usage.Stat(&sb, "Came back another day", returned)
	// Written out rather than via usage.Stat, which takes a count: this tile is
	// a percentage, and the markup has to match the same CSS.
	sb.WriteString(`<div class="traffic-stat">` +
		`<span class="traffic-stat-n">` + pct(returned, total) + `</span>` +
		`<span class="traffic-stat-l">day-two rate</span></div>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`<p class="card-desc">Of everyone who made an account in the last ` +
		`90 days, this is how many did anything at all on a later day. It is the ` +
		`number that separates "nobody arrives" from "everybody leaves".</p>`)
	sb.WriteString(`</div>`)

	sb.WriteString(signupChart(cohorts))
	sb.WriteString(retentionTable(cohorts))
	sb.WriteString(retentionCaveats(capped))

	app.Respond(w, r, app.Response{
		Title:       "Retention",
		Description: "Whether accounts come back after the day they signed up",
		HTML:        sb.String(),
	})
}

// activeByDay reads the day counters into "which accounts did anything on this
// day", and reports which days lost names to the per-bucket cap.
//
// A bucket keeps at most 200 distinct callers and counts the rest under
// "other", so a day busier than that under-reports who was there. On an
// ordinary day it never binds; on the day a link went to the front page of
// Hacker News it is exactly the day it binds, which is the day being asked
// about. Saying so is the difference between a floor and a wrong number.
func activeByDay() (map[string]map[string]bool, map[string]bool) {
	active := map[string]map[string]bool{}
	capped := map[string]bool{}

	for _, b := range usage.Series(usage.Day, cohortDays) {
		key := b.At.UTC().Format("2006-01-02")
		set := map[string]bool{}
		for who := range b.Users {
			if who == "guest" {
				// Rows written while signed-out runs existed, all under one
				// name: they cannot be told apart, so they were never a cohort.
				// There are no new ones — every run belongs to an account now —
				// and the old rows stay on disk until they age out.
				continue
			}
			if who == usage.Other {
				capped[key] = true
				continue
			}
			set[who] = true
		}
		active[key] = set
	}
	return active, capped
}

// cohortsFrom groups accounts by signup day and counts who was active after.
func cohortsFrom(active map[string]map[string]bool, capped map[string]bool) ([]cohort, int, int) {
	cutoff := time.Now().UTC().AddDate(0, 0, -cohortDays)

	byDay := map[string][]string{}
	for _, acc := range auth.AllAccounts() {
		if acc == nil || acc.Created.Before(cutoff) {
			continue
		}
		key := acc.Created.UTC().Format("2006-01-02")
		byDay[key] = append(byDay[key], acc.ID)
	}

	var out []cohort
	var total, returned int
	for key, ids := range byDay {
		day, err := time.Parse("2006-01-02", key)
		if err != nil {
			continue
		}
		c := cohort{Day: day, Size: len(ids), Returned: map[int]int{}, Partial: map[int]bool{}}
		total += len(ids)

		came := map[string]bool{}
		for _, off := range offsets {
			d := day.AddDate(0, 0, off).Format("2006-01-02")
			set, known := active[d]
			if !known {
				continue
			}
			if capped[d] {
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
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Day.After(out[j].Day) })
	return out, total, returned
}

// signupChart draws signups per day, so the spike is visible next to what
// happened after it.
func signupChart(cohorts []cohort) string {
	if len(cohorts) == 0 {
		return ""
	}
	buckets := make([]usage.Bucket, 0, len(cohorts))
	for i := len(cohorts) - 1; i >= 0; i-- {
		buckets = append(buckets, usage.Bucket{At: cohorts[i].Day, Total: cohorts[i].Size})
	}
	win := usage.Window{Slug: "quarter", Label: "Signups", Res: usage.Day,
		Points: len(buckets), Format: "2 Jan"}

	var sb strings.Builder
	sb.WriteString(`<div class="card"><span class="card-title">Signups per day</span>`)
	sb.WriteString(`<p class="card-desc">Only days with at least one signup. A spike ` +
		`with nothing after it is the shape this page exists to show.</p>`)
	sb.WriteString(usage.ChartSVG(buckets, win))
	sb.WriteString(`</div>`)
	return sb.String()
}

func retentionTable(cohorts []cohort) string {
	var sb strings.Builder
	sb.WriteString(`<div class="card"><span class="card-title">By signup day</span>`)

	if len(cohorts) == 0 {
		sb.WriteString(`<p class="card-desc">No accounts were created in the last 90 days.</p></div>`)
		return sb.String()
	}

	sb.WriteString(`<div class="cohort-scroll"><table class="cohort">`)
	sb.WriteString(`<tr><th>Signed up</th><th>Accounts</th>`)
	for _, off := range offsets {
		fmt.Fprintf(&sb, `<th>+%dd</th>`, off)
	}
	sb.WriteString(`</tr>`)

	now := time.Now().UTC()
	for _, c := range cohorts {
		fmt.Fprintf(&sb, `<tr><td>%s</td><td class="n">%d</td>`,
			html.EscapeString(c.Day.Format("2 Jan")), c.Size)
		for _, off := range offsets {
			// A cohort younger than the offset has not had the chance yet, and
			// showing it as 0% would read as a failure that has not happened.
			if c.Day.AddDate(0, 0, off).After(now) {
				sb.WriteString(`<td class="n pending">—</td>`)
				continue
			}
			n := c.Returned[off]
			mark := ""
			if c.Partial[off] {
				mark = `<span class="floor" title="that day hit the 200-caller cap, so this is a floor">*</span>`
			}
			fmt.Fprintf(&sb, `<td class="n %s">%s%s</td>`, heat(n, c.Size), pct(n, c.Size), mark)
		}
		sb.WriteString(`</tr>`)
	}
	sb.WriteString(`</table></div></div>`)
	return sb.String()
}

func retentionCaveats(capped map[string]bool) string {
	var sb strings.Builder
	sb.WriteString(`<div class="card"><span class="card-title">What this cannot tell you</span><ul class="caveats">`)
	sb.WriteString(`<li><b>Visitors who never signed up are invisible.</b> Every ` +
		`signed-out caller is counted under one name, so a bounce at the landing ` +
		`page looks like nothing at all. This measures accounts, not arrivals.</li>`)
	sb.WriteString(`<li><b>A busy day under-reports.</b> Each day keeps at most 200 ` +
		`distinct callers by name and counts the rest together, so on a day past ` +
		`that the numbers are a floor, marked *.`)
	if len(capped) > 0 {
		days := make([]string, 0, len(capped))
		for d := range capped {
			days = append(days, d)
		}
		sort.Strings(days)
		sb.WriteString(` Hit on: ` + html.EscapeString(strings.Join(days, ", ")) + `.`)
	} else {
		sb.WriteString(` Not hit in this window.`)
	}
	sb.WriteString(`</li>`)
	sb.WriteString(`<li><b>Ninety days, and no further.</b> The day counters keep a ` +
		`quarter; anything older has been trimmed.</li>`)
	sb.WriteString(`</ul></div>`)
	return sb.String()
}

func pct(n, of int) string {
	if of == 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", n*100/of)
}

// heat grades a cell so the shape of the table is readable before the numbers
// are.
func heat(n, of int) string {
	if of == 0 || n == 0 {
		return "cold"
	}
	switch p := n * 100 / of; {
	case p >= 40:
		return "hot"
	case p >= 15:
		return "warm"
	default:
		return "cool"
	}
}

const retentionCSS = `<style>
.cohort-scroll { overflow-x: auto; }
table.cohort { border-collapse: collapse; font-size: 13px; width: 100%; }
table.cohort th, table.cohort td { padding: 5px 10px; text-align: left; white-space: nowrap; }
table.cohort th { font-size: 11px; text-transform: uppercase; letter-spacing: .06em;
  color: #888; border-bottom: 1px solid #e6e6e6; }
table.cohort td.n { text-align: right; font-variant-numeric: tabular-nums; }
table.cohort td.cold { color: #bbb; }
table.cohort td.cool { background: rgba(31,111,92,.07); }
table.cohort td.warm { background: rgba(31,111,92,.16); }
table.cohort td.hot  { background: rgba(31,111,92,.28); font-weight: 600; }
table.cohort td.pending { color: #ccc; }
.floor { color: #a8641b; font-weight: 600; }
ul.caveats { margin: 6px 0 0; padding-left: 18px; font-size: 13px; color: #555; }
ul.caveats li { margin-bottom: 6px; }
</style>`
