package brief

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEditionSurvivesSerializationAndRotation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := entries
	defer func() { entries = old }()
	e := Entry{Text: "Blast closes", Material: "Original sources", Written: time.Now(), Day: today()}
	entries = []Entry{e}
	if err := Pin(e); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	var restored Entry
	json.Unmarshal(raw, &restored)
	if e.ID() != restored.ID() {
		t.Fatal("edition identity changed after serialization")
	}
	entries = []Entry{{Text: "New summary", Written: time.Now(), Day: today()}}
	got, ok := Get(e.ID())
	if !ok || got.Material != e.Material {
		t.Fatal("attached edition lost after rotation")
	}
	if _, ok := Get("../../brief"); ok {
		t.Fatal("invalid id accepted")
	}
}

func TestEditionSourcesComeFromMaterial(t *testing.T) {
	e, err := decodeEdition(`{"text":"Blast closes", "stories":[{"title":"Blast", "detail":"Assets fell", "sources":["https://example.com/blast", "https://invented.example/", "javascript:alert(1)"]}]}`, "Source: https://example.com/blast")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Stories) != 1 || len(e.Stories[0].Sources) != 1 {
		t.Fatalf("unverified source retained: %+v", e)
	}
	if _, err := decodeEdition("not json", ""); err == nil {
		t.Fatal("malformed edition accepted")
	}
}
