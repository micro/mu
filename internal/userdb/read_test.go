package userdb

import (
	"errors"
	"mu/internal/data"
	"testing"
)

func TestReadErrorIsNotMissingRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rec, err := Create("test", "owner", "messages", map[string]interface{}{"text": "private"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SaveFile("test/db/messages.json", "{broken"); err != nil {
		t.Fatal(err)
	}
	_, err = Get("test", "owner", "messages", rec.ID)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("storage failure hidden: %v", err)
	}
}
