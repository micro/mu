package home

// Which column a card is in is cards.json's to say.
//
// This has been got wrong three times, each time by code deciding the layout
// for itself and leaving the file as decoration:
//
//   - Every card was flattened into one list and dealt out alternately —
//     left, right, left, right — so the file's halves meant nothing.
//   - Then the column was computed from Streamed(), whether the card carries a
//     time. Blog, news, social, video and images all do; markets and prayer do
//     not. Five against two, and no way to fix it, because the only code that
//     read the layout was the code ignoring it.
//
// Both were reasonable-looking rules that happened to produce a page nobody
// asked for. There is no property of a card that computes where it should go —
// it is a judgement about how the page looks — so the file decides and this
// says so.

import (
	"encoding/json"
	"testing"
	"time"
)

// wanted reads cards.json: id → column, and the order within each column.
//
// Read rather than written out, because the file is the thing under test. A
// copy of the layout here would be a second place to keep it in step, which is
// the failure this whole test is about.
func wanted(t *testing.T) (column map[string]string, order map[string][]string) {
	t.Helper()
	b, err := f.ReadFile("cards.json")
	if err != nil {
		t.Fatalf("reading cards.json: %v", err)
	}
	var config CardConfig
	if err := json.Unmarshal(b, &config); err != nil {
		t.Fatalf("parsing cards.json: %v", err)
	}
	column, order = map[string]string{}, map[string][]string{}
	for _, c := range config.Left {
		column[c.ID] = "left"
		order["left"] = append(order["left"], c.ID)
	}
	for _, c := range config.Right {
		column[c.ID] = "right"
		order["right"] = append(order["right"], c.ID)
	}
	if len(column) == 0 {
		t.Fatal("cards.json declares no cards, so this test is checking nothing")
	}
	return column, order
}

// Every card that loaded is in the column the file puts it in.
//
// Only cards whose service registered a renderer are loaded, and in a bare test
// most have not — so this checks the rule against whatever did load rather than
// naming cards. Naming them would make this a test of which services happen to
// be registered, which is not the thing that keeps breaking.
func TestTheFileDecidesTheColumns(t *testing.T) {
	Load()
	if len(Cards) == 0 {
		t.Fatal("no cards loaded, so this test is checking nothing")
	}
	column, _ := wanted(t)

	for _, c := range Cards {
		want, known := column[c.ID]
		if !known {
			t.Errorf("card %q is loaded but is in no column in cards.json", c.ID)
			continue
		}
		if c.column() != want {
			t.Errorf("card %q renders in the %s column; cards.json puts it in %s — "+
				"something is deciding the layout for itself again",
				c.ID, c.column(), want)
		}
	}
}

// And in the file's order within that column, with the left column first so the
// two are never interleaved.
func TestCardsKeepTheFilesOrder(t *testing.T) {
	Load()
	_, order := wanted(t)

	loaded := map[string]bool{}
	for _, c := range Cards {
		loaded[c.ID] = true
	}

	var want []string
	for _, col := range []string{"left", "right"} {
		for _, id := range order[col] {
			if loaded[id] {
				want = append(want, id)
			}
		}
	}
	for i, c := range Cards {
		if i >= len(want) {
			t.Fatalf("more cards loaded (%d) than cards.json declares (%d)", len(Cards), len(want))
		}
		if c.ID != want[i] {
			t.Errorf("card %d is %q, want %q — Cards is not in the file's order, so "+
				"the columns render out of order", i, c.ID, want[i])
			break
		}
	}
}

// A card's contents cannot move it.
//
// Computing the column from Streamed() meant an empty card reshuffled the page:
// whether the daily image had landed yet changed where everything after it
// went. Where a card goes is settled at load; only what is in it changes.
func TestWhatIsInACardDoesNotMoveIt(t *testing.T) {
	Load()

	before := map[string]string{}
	for _, c := range Cards {
		before[c.ID] = c.column()
	}

	// The thing that used to decide it: whether the card carries a time, and
	// whether it has anything in it at all.
	for i := range Cards {
		if Cards[i].At.IsZero() {
			Cards[i].At = Cards[i].UpdatedAt
		} else {
			Cards[i].At = time.Time{}
		}
		Cards[i].CachedHTML = ""
	}

	for _, c := range Cards {
		if c.column() != before[c.ID] {
			t.Errorf("%s moved from %s to %s because its contents changed",
				c.ID, before[c.ID], c.column())
		}
	}
}
