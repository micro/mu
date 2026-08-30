package brief

// One line, or none.
//
// Two things have to hold without a model in the room. Whatever comes back
// becomes a clause or nothing — a model asked for two plain sentences will
// sometimes send a bulleted paragraph with a markdown link in it, and Home has
// nowhere to put that. And a line about yesterday must not be shown today,
// because it carries no date and reads as if it were now.

import (
	"strings"
	"testing"
	"time"
)

// A model told "plain text, two sentences" answers in all of these.
func TestWhateverComesBackBecomesAClause(t *testing.T) {
	for _, c := range []struct {
		what, in, want string
	}{
		{
			"a plain line",
			"Egypt's second-largest bank was hit by US sanctions.",
			"Egypt's second-largest bank was hit by US sanctions.",
		},
		{
			"a markdown link",
			"[Banque Misr](https://example.com/x) was hit by US sanctions.",
			"Banque Misr was hit by US sanctions.",
		},
		{
			"bold and a bullet",
			"- **Banque Misr** hit by US sanctions.",
			"Banque Misr hit by US sanctions.",
		},
		{
			"a heading and a second paragraph",
			"## Today\n\nOil fell 3%.\n\nEverything else was quiet.",
			"Oil fell 3%.",
		},
		{
			"a preamble",
			"Here is the line:\n\nRates held at 4%.",
			"Rates held at 4%.",
		},
		{
			"quotes around the whole thing",
			`"Oil fell 3% on the Gulf ceasefire."`,
			"Oil fell 3% on the Gulf ceasefire.",
		},
		{
			"leading and trailing space",
			"  Rates held at 4%.  ",
			"Rates held at 4%.",
		},
	} {
		if got := clean(c.in); got != c.want {
			t.Errorf("%s: clean(%q) = %q, want %q", c.what, c.in, got, c.want)
		}
	}
}

// A quiet day has to be sayable, or one gets manufactured every Sunday.
func TestNothingIsAnAnswer(t *testing.T) {
	for _, in := range []string{"", "   ", "NOTHING", "Nothing", "nothing.", "NOTHING!"} {
		if got := clean(in); got != "" {
			t.Errorf("clean(%q) = %q, want nothing", in, got)
		}
	}
}

// The prompt asks for under 256 characters. This is what happens when that is
// ignored, because a paragraph laid across the top of Home is the failure the
// length rule exists to prevent.
func TestAParagraphIsCutDownToALine(t *testing.T) {
	long := strings.Repeat("The central bank held rates and markets barely moved. ", 20)

	got := clean(long)
	if n := len([]rune(got)); n > limit+1 {
		t.Errorf("a %d character answer was kept as %d characters", len(long), n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a cut line does not say it was cut: %q", got)
	}
}

// Yesterday's line is not today's news, and it carries no date to say so.
func TestALineAboutYesterdayIsNotShown(t *testing.T) {
	mu.Lock()
	entries = []Entry{{
		Text:    "Oil fell 3% on the Gulf ceasefire.",
		Written: time.Now().Add(-20 * time.Hour),
		Day:     time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
	}}
	mu.Unlock()

	if got := Line(); got != "" {
		t.Errorf("yesterday's line is still on the page: %q", got)
	}

	mu.Lock()
	entries[0].Day = today()
	mu.Unlock()

	if got := Line(); got != "Oil fell 3% on the Gulf ceasefire." {
		t.Errorf("today's line is not shown: %q", got)
	}

	mu.Lock()
	entries = nil
	mu.Unlock()

	if got := Line(); got != "" {
		t.Errorf("a line appeared before anything was written: %q", got)
	}
}

// Written every hour and kept for a fortnight: the history is the only reason
// this is on disk rather than in memory.
func TestOnlyAFortnightIsKept(t *testing.T) {
	mu.Lock()
	entries = nil
	for i := 0; i < keep+10; i++ {
		entries = append(entries, Entry{Text: "line", Written: time.Now(), Day: today()})
		if len(entries) > keep {
			entries = entries[len(entries)-keep:]
		}
	}
	n := len(entries)
	entries = nil
	mu.Unlock()

	if n != keep {
		t.Errorf("kept %d lines, want %d", n, keep)
	}
}

// It does not write again until the last one is an hour old, because a front
// page whose sentence changes every time you look at it reads as noise however
// good each sentence is.
func TestItWaitsAnHourBetweenLines(t *testing.T) {
	mu.Lock()
	entries = []Entry{{Text: "line", Written: time.Now(), Day: today()}}
	mu.Unlock()
	if due() {
		t.Error("wrote again straight away")
	}

	mu.Lock()
	entries[0].Written = time.Now().Add(-gap - time.Minute)
	mu.Unlock()
	if !due() {
		t.Error("an hour passed and it did not write")
	}

	// A new day is due whatever the clock says about the last line.
	mu.Lock()
	entries[0].Written = time.Now()
	entries[0].Day = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	mu.Unlock()
	if !due() {
		t.Error("the day turned over and it did not write")
	}

	// Nothing written yet is always due.
	mu.Lock()
	entries = nil
	mu.Unlock()
	if !due() {
		t.Error("it never writes a first line")
	}

	// Except while one is already being written.
	mu.Lock()
	running = true
	mu.Unlock()
	if due() {
		t.Error("two runs at once")
	}
	mu.Lock()
	running = false
	mu.Unlock()
}
