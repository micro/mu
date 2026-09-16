package events

import (
	"mu/internal/auth"
	"testing"
	"time"
)

func TestBriefRequiresOptIn(t *testing.T) {
	reset()
	defer reset()
	auth.Create(&auth.Account{ID: "optin", Zone: "Europe/London"})
	defer auth.DeleteAccount("optin")
	ensureDefaultBriefs()
	if Brief("optin") != nil {
		t.Fatal("automatically enrolled account")
	}
	if err := ConfigureBrief("optin", true, true, "Europe/London"); err != nil {
		t.Fatal(err)
	}
	ensureDefaultBriefs()
	if e := Brief("optin"); e == nil || e.Paused || e.Builtin {
		t.Fatalf("explicit choice lost: %+v", e)
	}
}
func TestExistingDailyBriefIsPreserved(t *testing.T) {
	reset()
	defer reset()
	events["auto"] = &Event{ID: "auto", Owner: "owner", Kind: "brief", Builtin: true, When: time.Now()}
	events["chosen"] = &Event{ID: "chosen", Owner: "other", Kind: "brief", When: time.Now()}
	ensureDefaultBriefs()
	if events["auto"].Paused || events["chosen"].Paused {
		t.Fatal("existing daily briefs must remain enabled")
	}
}
