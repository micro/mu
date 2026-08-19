package account

// Where you are, and how precisely.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func located(t *testing.T, id string) {
	t.Helper()
	if _, err := auth.GetAccount(id); err == nil {
		return
	}
	if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
		t.Fatalf("could not create %s: %v", id, err)
	}
}

// Coordinates are rounded on the way in, always.
//
// About a kilometre, which is the same forecast, the same prayer time, the same
// trains and the same cafes — and is not somebody's address. This is stored on
// a server and read by a model that may quote it back, so the rounding is a
// property of the store rather than of whoever remembered to round before
// calling.
func TestAPlaceIsNotAnAddress(t *testing.T) {
	const who = "place-precision"
	located(t, who)

	if err := SetPlace(who, "London", 51.507351, -0.127758, "Europe/London"); err != nil {
		t.Fatal(err)
	}
	acc, err := auth.GetAccount(who)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Lat != 51.51 || acc.Lon != -0.13 {
		t.Errorf("stored %v,%v — six decimal places is a doorway, not a city",
			acc.Lat, acc.Lon)
	}
	if acc.Zone != "Europe/London" {
		t.Errorf("the timezone was lost: %q", acc.Zone)
	}
}

// A point that is not on the earth is refused rather than stored, because a
// latitude of 400 reaches a tool as a latitude of 400.
func TestSomewhereThatIsNotAPlace(t *testing.T) {
	const who = "place-impossible"
	located(t, who)
	if err := SetPlace(who, "Nowhere", 400, 0, ""); err == nil {
		t.Error("a latitude of 400 was accepted")
	}
}

// And it can be taken back. A place you cannot clear is one you have given
// away permanently, which is the wrong deal for this particular fact.
func TestItCanBeTakenBack(t *testing.T) {
	const who = "place-cleared"
	located(t, who)
	if err := SetPlace(who, "Lisbon", 38.72, -9.14, "Europe/Lisbon"); err != nil {
		t.Fatal(err)
	}
	if PlaceLine(who) == "" {
		t.Fatal("nothing was stored")
	}
	if err := SetPlace(who, "", 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	if got := PlaceLine(who); got != "" {
		t.Errorf("clearing it left %q", got)
	}
}

// The line an agent is given carries both the name and the coordinates.
//
// The name is what a person reads back — "in Lisbon" — and the coordinate is
// what a tool takes. An agent handed only "Lisbon" has to geocode it before it
// can ask for a forecast, which is a tool call, a delay, and a chance to pick
// the wrong Lisbon.
func TestTheLineCarriesBothHalves(t *testing.T) {
	const who = "place-line"
	located(t, who)
	if err := SetPlace(who, "Lisbon", 38.72, -9.14, "Europe/Lisbon"); err != nil {
		t.Fatal(err)
	}
	got := PlaceLine(who)
	for _, want := range []string{"Lisbon", "38.72", "-9.14", "Europe/Lisbon"} {
		if !strings.Contains(got, want) {
			t.Errorf("the line is missing %q: %q", want, got)
		}
	}
}

// Nobody has said, so nothing is claimed. An agent told "They are in " would
// answer as though it knew.
func TestSilenceWhenNobodyHasSaid(t *testing.T) {
	const who = "place-unset"
	located(t, who)
	if got := PlaceLine(who); got != "" {
		t.Errorf("an account that has said nothing reports %q", got)
	}
}
