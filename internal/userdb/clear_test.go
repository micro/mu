package userdb

import "testing"

func TestClearOnlyRemovesCallerCollection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a, e := Create("forms", "one", "responses", map[string]any{"text": "one"}, false)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Create("forms", "two", "responses", map[string]any{"text": "two"}, false)
	if e != nil {
		t.Fatal(e)
	}
	c, e := Create("forms", "one", "other", map[string]any{"text": "other"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = Clear("forms", "", "responses"); e != ErrAuth {
		t.Fatal(e)
	}
	if e = Clear("forms", "one", "responses"); e != nil {
		t.Fatal(e)
	}
	if _, e = Get("forms", "one", "responses", a.ID); e == nil {
		t.Fatal("not removed")
	}
	if _, e = Get("forms", "two", "responses", b.ID); e != nil {
		t.Fatal("other owner removed", e)
	}
	if _, e = Get("forms", "one", "other", c.ID); e != nil {
		t.Fatal("other collection removed", e)
	}
}
