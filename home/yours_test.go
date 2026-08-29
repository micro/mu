package home

// The blocks that are the reader's own, laid across the page.

import (
	"strings"
	"testing"
)

// What is yours goes across the page, and an empty block is not a column.
//
// It matters more here than it did in a stack. Three full-width bands cost
// nothing when one of them was empty — it simply was not drawn. A column that
// renders nothing would take a third of the row and say nothing in it, and a
// new account has two of the three empty.
func TestWhatIsYoursSkipsTheBlocksWithNothingInThem(t *testing.T) {
	got := yours(
		block{"Notes", `<div class="notes-peek">a note</div>`},
		block{"Inbox", ""},
		block{"Agents", "   \n "},
	)
	if n := strings.Count(got, "home-yours-col"); n != 1 {
		t.Errorf("%d columns drawn, want 1", n)
	}
	for _, gone := range []string{"Inbox", "Agents"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s has nothing in it and still got a heading", gone)
		}
	}
	if !strings.Contains(got, "Notes") || !strings.Contains(got, "a note") {
		t.Errorf("the block with something in it did not render:\n%s", got)
	}
}

// Nothing yours at all is no row, not an empty one.
func TestAnEmptyRowIsNotDrawn(t *testing.T) {
	if got := yours(block{"Notes", ""}, block{"Inbox", ""}); got != "" {
		t.Errorf("an empty row rendered %q", got)
	}
}

// Each block keeps its heading, and they stay in the order given: what you
// wrote down, what arrived, who is working — inside before outside.
func TestTheBlocksKeepTheirOrderAndTheirHeadings(t *testing.T) {
	got := yours(
		block{"Notes", "<p>n</p>"},
		block{"Inbox", "<p>i</p>"},
		block{"Agents", "<p>a</p>"},
	)
	if n := strings.Count(got, "home-yours-col"); n != 3 {
		t.Errorf("%d columns drawn, want 3", n)
	}
	notes, inbox, agents := strings.Index(got, "Notes"), strings.Index(got, "Inbox"), strings.Index(got, "Agents")
	if !(notes < inbox && inbox < agents) {
		t.Errorf("the columns are not in the order given:\n%s", got)
	}
	if n := strings.Count(got, "home-section"); n != 3 {
		t.Errorf("%d headings for 3 columns", n)
	}
}
