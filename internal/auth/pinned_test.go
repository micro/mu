package auth

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Pinning is a toggle, because the control is one button on a tile. Two
// buttons, or a button whose meaning depends on state you cannot see, would
// both be worse.
func TestPinningTogglesAndReportsTheResult(t *testing.T) {
	acc := &Account{ID: "reader", Pinned: []string{}}

	if on := acc.TogglePin("video"); !on {
		t.Error("pinning a service reported it as not pinned")
	}
	if got := acc.PinnedServices(); len(got) != 1 || got[0] != "video" {
		t.Fatalf("after pinning: %v", got)
	}

	if on := acc.TogglePin("video"); on {
		t.Error("unpinning reported it as still pinned")
	}
	if got := acc.PinnedServices(); len(got) != 0 {
		t.Errorf("after unpinning: %v", got)
	}
}

// Order is the reader's, and pinning appends rather than sorting.
func TestPinningAppendsInTheOrderChosen(t *testing.T) {
	acc := &Account{ID: "reader", Pinned: []string{}}
	for _, name := range []string{"video", "mail", "news"} {
		acc.TogglePin(name)
	}

	got := acc.PinnedServices()
	want := []string{"video", "mail", "news"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Unpinning from the middle must not disturb what is around it. This is worth a
// test because the obvious implementation shares the backing array between the
// slice it reads and the slice it writes, which silently corrupts the tail.
func TestUnpinningFromTheMiddleKeepsTheRest(t *testing.T) {
	acc := &Account{ID: "reader", Pinned: []string{}}
	for _, name := range []string{"video", "mail", "news", "markets"} {
		acc.TogglePin(name)
	}

	acc.TogglePin("news")

	got := acc.PinnedServices()
	want := []string{"video", "mail", "markets"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Duplicates and blanks are the shapes a form can produce, and neither should
// reach the sidebar.
func TestPinnedIsCleanedOnTheWayIn(t *testing.T) {
	acc := &Account{ID: "reader", Pinned: []string{}}
	acc.SetPinned([]string{"Video", " video ", "", "  ", "mail"})

	got := acc.PinnedServices()
	want := []string{"video", "mail"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDefaultPinsAndExplicitEmptySurviveJSON(t *testing.T) {
	a := &Account{}
	want := []string{"web", "news", "video", "bookmarks"}
	if !reflect.DeepEqual(a.PinnedServices(), want) {
		t.Fatal(a.PinnedServices())
	}
	a.SetPinned(nil)
	raw, _ := json.Marshal(a)
	var restored Account
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.PinnedServices()) != 0 {
		t.Fatal("empty selection reset to defaults")
	}
	a.SetPinned([]string{"mail", "news"})
	raw, _ = json.Marshal(a)
	json.Unmarshal(raw, &restored)
	if !reflect.DeepEqual(restored.PinnedServices(), []string{"mail", "news"}) {
		t.Fatal("custom pins changed")
	}
}
