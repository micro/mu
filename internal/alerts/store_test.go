package alerts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"mu/internal/data"
	"mu/internal/dir"
)

func TestConditionLifecyclePersists(t *testing.T) {
	owner := uuid.NewString()
	t.Cleanup(func() { _ = Clear(owner) })
	input := Item{Source: "system", Key: "disk", Title: "Disk nearly full", Detail: "90% used", URL: "/admin/server"}
	i, fresh, err := Raise(owner, input)
	if err != nil || !fresh {
		t.Fatal(i, fresh, err)
	}
	if err = Update(owner, i.ID, "acknowledged"); err != nil {
		t.Fatal(err)
	}
	input.Detail = "91% used"
	again, fresh, err := Raise(owner, input)
	if err != nil || fresh || again.ID != i.ID || again.State != "acknowledged" {
		t.Fatal(again, fresh, err)
	}
	if err = Update(owner, i.ID, "dismissed"); err != nil {
		t.Fatal(err)
	}
	// The next read is from disk; there is no in-memory dedup cache to survive.
	b, err := data.LoadFile(key(owner))
	if err != nil {
		t.Fatal(err)
	}
	var persisted []Item
	if json.Unmarshal(b, &persisted) != nil || len(persisted) != 1 || persisted[0].State != "dismissed" {
		t.Fatal(string(b))
	}
	again, fresh, err = Raise(owner, input)
	if err != nil || fresh || again.State != "dismissed" {
		t.Fatal(again, fresh, err)
	}
	if err = Recover(owner, "system", "disk"); err != nil {
		t.Fatal(err)
	}
	again, fresh, err = Raise(owner, input)
	if err != nil || !fresh || again.State != "open" || again.ID != i.ID {
		t.Fatal(again, fresh, err)
	}
	if _, err = Get("other-account", i.ID); err == nil {
		t.Fatal("cross-account read")
	}
	if err = Update("other-account", i.ID, "resolved"); err == nil {
		t.Fatal("cross-account update")
	}
}
func TestCustomRecoveryAndNamespaces(t *testing.T) {
	owner := uuid.NewString()
	t.Cleanup(func() { _ = Clear(owner) })
	system, _, _ := Raise(owner, Item{Source: "system", Key: "same", Title: "System"})
	custom, _, _ := Raise(owner, Item{Source: "custom", Key: "same", Title: "Custom"})
	if system.ID == custom.ID {
		t.Fatal("custom can overwrite system")
	}
	if err := Update(owner, custom.ID, "resolved"); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := Raise(owner, Item{Source: "custom", Key: "same", Title: "Again"})
	if err != nil || !fresh {
		t.Fatal(fresh, err)
	}
	if err = Update(owner, system.ID, "resolved"); err != nil {
		t.Fatal(err)
	}
	_, fresh, err = Raise(owner, Item{Source: "system", Key: "same", Title: "Still true"})
	if err != nil || fresh {
		t.Fatal("manual resolve manufactured recovery", err)
	}
}
func TestBadStorageFailsClosed(t *testing.T) {
	owner := uuid.NewString()
	t.Cleanup(func() { _ = Clear(owner) })
	if err := data.SaveFile(key(owner), "broken"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Raise(owner, Item{Source: "custom", Key: "k", Title: "test"}); err == nil {
		t.Fatal("overwrote corrupt storage")
	}
	b, _ := data.LoadFile(key(owner))
	if string(b) != "broken" {
		t.Fatal("lost corrupt original")
	}
	_ = Clear(owner)
	path := filepath.Join(dir.Data(), key(owner))
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, fresh, err := Raise(owner, Item{Source: "custom", Key: "k", Title: "test"}); err == nil || fresh {
		t.Fatal("reported success without persistence")
	}
	_ = os.Remove(path)
}
func TestActionURLValidation(t *testing.T) {
	for _, link := range []string{"javascript:alert(1)", "//evil.test", "/\\evil.test", "https://user:secret@example.com", "data:text/html,x"} {
		if err := validate(Item{Source: "custom", Key: "k", Title: "t", URL: link}); err == nil {
			t.Errorf("accepted %q", link)
		}
	}
}
