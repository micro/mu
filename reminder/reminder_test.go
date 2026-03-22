package reminder

import (
	"testing"
)

func TestReminderData_Structure(t *testing.T) {
	rd := &ReminderData{
		Verse:   "In the name of Allah",
		Name:    "Al-Rahman",
		Hadith:  "Narrated Abu Hurairah",
		Message: "Be mindful of Allah",
		Updated: "2026-03-20",
	}
	if rd.Verse != "In the name of Allah" {
		t.Error("expected verse")
	}
	if rd.Name != "Al-Rahman" {
		t.Error("expected name")
	}
	if rd.Hadith != "Narrated Abu Hurairah" {
		t.Error("expected hadith")
	}
	if rd.Message != "Be mindful of Allah" {
		t.Error("expected message")
	}
}

