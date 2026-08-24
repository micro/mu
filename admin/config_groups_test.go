package admin

import (
	"strings"
	"testing"
)

// Every group says what it is for.
//
// The page was 112 settings in 17 groups named for vendors — AI, Twilio, S3 —
// and a row was a name, a badge and a box. That answers "what are my Twilio
// settings", which nobody asks. It could not answer "why doesn't SMS work",
// which is the only reason anybody opens it.
func TestEveryGroupSaysWhatItIsFor(t *testing.T) {
	for _, g := range settingGroups {
		if strings.TrimSpace(g.Does) == "" {
			t.Errorf("the %q group does not say what it is for, so its settings are "+
				"names an operator has to already know", g.Name)
		}
		// A sentence, not a label. "AI settings" is the group name again.
		if len(g.Does) < 25 {
			t.Errorf("the %q group's description is %q — too short to be an "+
				"explanation", g.Name, g.Does)
		}
	}
}

// Anything a group needs is a setting the page can actually set.
//
// A required value that is not in Vars is one the status line names and the
// page cannot offer — a dead end worse than saying nothing, which is the exact
// mistake /sms and /whatsapp made by pointing here for Twilio values that were
// not on this page at all.
func TestARequiredSettingIsOnThePage(t *testing.T) {
	for _, g := range settingGroups {
		have := map[string]bool{}
		for _, v := range g.Vars {
			have[v] = true
		}
		for _, n := range g.Needs {
			if !have[n] {
				t.Errorf("the %q group needs %s and does not offer it, so the page "+
					"names a value it cannot set", g.Name, n)
			}
			if !Settable(n) {
				t.Errorf("%s is required by %q and is not settable anywhere", n, g.Name)
			}
		}
	}
}

// A capability with required values reports whether it is on.
func TestAGroupKnowsWhetherItIsWorking(t *testing.T) {
	t.Setenv("TWILIO_ACCOUNT_SID", "")
	t.Setenv("TWILIO_AUTH_TOKEN", "")
	t.Setenv("TWILIO_FROM", "")

	var twilio settingGroup
	for _, g := range settingGroups {
		if strings.HasPrefix(g.Name, "Twilio") {
			twilio = g
		}
	}
	if twilio.Name == "" {
		t.Fatal("no Twilio group")
	}

	ok, missing := twilio.on()
	if ok {
		t.Error("Twilio reports itself working with none of its values set")
	}
	if len(missing) != 3 {
		t.Errorf("named %d missing values, want 3 — the status line has to say "+
			"which, or it is the same dead end as before: %v", len(missing), missing)
	}

	t.Setenv("TWILIO_ACCOUNT_SID", "AC-test")
	if _, missing := twilio.on(); len(missing) != 2 {
		t.Errorf("after setting one value, %d are still named; want 2", len(missing))
	}
}

// A group with nothing required does not claim to be broken.
//
// Transit answers from published timetables with no key at all, and Chain has
// defaults. Marking them "not working" would send an operator hunting for a
// key that is not needed — the inverse of the bug being fixed, and just as
// wrong.
func TestAnOptionalGroupIsNotReportedAsBroken(t *testing.T) {
	for _, g := range settingGroups {
		if len(g.Needs) != 0 {
			continue
		}
		if ok, missing := g.on(); !ok {
			t.Errorf("the %q group has no required values but reports itself off: %v",
				g.Name, missing)
		}
	}
}
