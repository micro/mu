package userdb

import "testing"

func TestWriteErrorsAreReturnedWithoutChangingTheRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bad := map[string]interface{}{"invalid": make(chan int)}
	if _, err := Create("test", "alice", "records", bad, false); err == nil {
		t.Fatal("unpersistable creation reported success")
	}
	rec, err := Create("test", "alice", "records", map[string]interface{}{"value": "before"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Update("test", "alice", "records", rec.ID, bad, false); err == nil {
		t.Fatal("unpersistable update reported success")
	}
	got, err := Get("test", "alice", "records", rec.ID)
	if err != nil || got.Data["value"] != "before" {
		t.Fatalf("failed update changed the stored record: %+v, %v", got, err)
	}
}
