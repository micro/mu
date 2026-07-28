package auth

import "testing"

func TestShowHomeCard(t *testing.T) {
	cases := []struct {
		name string
		acc  Account
		id   string
		want bool
	}{
		{"fresh account shows every default", Account{}, "images", true},
		{"fresh account shows islam", Account{}, "reminder", true},
		{"legacy set (no seen) shows new card images",
			Account{HomeCards: []string{"blog", "news", "markets", "reminder", "social", "video"}}, "images", true},
		{"legacy set keeps a chosen card",
			Account{HomeCards: []string{"blog", "news"}}, "blog", true},
		{"legacy set hides a pre-existing deselected card",
			Account{HomeCards: []string{"blog", "news"}}, "social", false},
		{"post-save deselect of images sticks",
			Account{HomeCards: []string{"blog", "news"}, HomeCardsSeen: []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}}, "images", false},
		{"post-save select of images shows",
			Account{HomeCards: []string{"blog", "images"}, HomeCardsSeen: []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}}, "images", true},
		{"future card after a save defaults on",
			Account{HomeCards: []string{"blog"}, HomeCardsSeen: []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}}, "audio", true},
	}
	for _, c := range cases {
		if got := c.acc.ShowHomeCard(c.id); got != c.want {
			t.Errorf("%s: ShowHomeCard(%q)=%v want %v", c.name, c.id, got, c.want)
		}
	}
}

func TestHomeCardActiveOptIn(t *testing.T) {
	if (&Account{}).HomeCardActive("mail") {
		t.Error("fresh account should not have mail opt-in active")
	}
	if !(&Account{HomeCards: []string{"mail"}}).HomeCardActive("mail") {
		t.Error("mail in HomeCards should be active")
	}
}

// A card id that was renamed must keep resolving from stored preferences: an
// account that saved "reminder" before the rename still sees the Islam card,
// and a deliberate hide still sticks.
func TestLegacyCardIDStillResolves(t *testing.T) {
	seenOld := []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}

	selected := Account{HomeCards: []string{"blog", "reminder"}, HomeCardsSeen: seenOld}
	if !selected.ShowHomeCard("islam") {
		t.Error("stored \"reminder\" selection should show the islam card")
	}

	hidden := Account{HomeCards: []string{"blog"}, HomeCardsSeen: seenOld}
	if hidden.ShowHomeCard("islam") {
		t.Error("islam was offered as \"reminder\" and not selected — should stay hidden")
	}

	// Legacy account with no seen-set: islam is not a new card, so a
	// customised account that dropped it must not have it reappear.
	legacy := Account{HomeCards: []string{"blog", "news"}}
	if legacy.ShowHomeCard("islam") {
		t.Error("islam predates seen-tracking; should not resurface for a customised legacy account")
	}
}
