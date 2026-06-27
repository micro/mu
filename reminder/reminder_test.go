package reminder

import "testing"

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

func TestStringField(t *testing.T) {
	fields := map[string]interface{}{
		"message": "Be mindful of Allah",
		"updated": 1700000000,
	}

	if got := stringField(fields, "message"); got != "Be mindful of Allah" {
		t.Fatalf("stringField(message) = %q, want %q", got, "Be mindful of Allah")
	}
	if got := stringField(fields, "updated"); got != "" {
		t.Fatalf("stringField(updated) = %q, want empty string for non-string value", got)
	}
	if got := stringField(fields, "missing"); got != "" {
		t.Fatalf("stringField(missing) = %q, want empty string", got)
	}
}

func TestDeduplicateVerseName(t *testing.T) {
	input := "Muhammad - Muhammad - 47:1\nThose who disbelieve..."
	want := "Muhammad - 47:1\nThose who disbelieve..."

	if got := deduplicateVerseName(input); got != want {
		t.Fatalf("deduplicateVerseName() = %q, want %q", got, want)
	}
}

func TestDeduplicateVerseNameLeavesDifferentNames(t *testing.T) {
	input := "Al-Fatihah - The Opener - 1:1\nIn the name..."

	if got := deduplicateVerseName(input); got != input {
		t.Fatalf("deduplicateVerseName() = %q, want unchanged input", got)
	}
}
