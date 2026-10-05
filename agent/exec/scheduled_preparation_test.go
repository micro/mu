package exec

import (
	"encoding/json"
	"mu/internal/persist"
	"testing"
	"time"
)

func TestPreparedResultHeldAndDeliveredOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := request{Account: "owner", ID: "brief", Revision: "1", Due: time.Now().Add(-time.Second), Preparing: true}
	generated, sent := 0, 0
	prepare := func(string, string, string, time.Time) (bool, string, error) {
		generated++
		return true, "prepared brief", nil
	}
	current := func(string, string, string) bool { return true }
	send := func(_ request, body string, err error) error {
		sent++
		if body != "prepared brief" || err != nil {
			t.Fatalf("wrong result %q %v", body, err)
		}
		return nil
	}
	if err := consumePreparedWith(r, prepare, current, send); err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatal("delivered during preparation")
	}
	// A second call reads the persisted result rather than generating again.
	if err := consumePreparedWith(r, prepare, current, send); err != nil {
		t.Fatal(err)
	}
	r.Preparing = false
	for i := 0; i < 2; i++ {
		if err := consumePreparedWith(r, prepare, current, send); err != nil {
			t.Fatal(err)
		}
	}
	if generated != 1 || sent != 1 {
		t.Fatalf("generated=%d sent=%d", generated, sent)
	}
}

func TestPreparedCancellationAndInterruptedRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := request{Account: "owner", ID: "research", Revision: "1", Due: time.Now().Add(-time.Second), Preparing: true}
	valid := true
	current := func(string, string, string) bool { return valid }
	prepare := func(string, string, string, time.Time) (bool, string, error) { return true, "report", nil }
	sent := 0
	send := func(request, string, error) error { sent++; return nil }
	if err := consumePreparedWith(r, prepare, current, send); err != nil {
		t.Fatal(err)
	}
	valid = false
	r.Preparing = false
	if err := consumePreparedWith(r, prepare, current, send); err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatal("canceled result delivered")
	}
	valid = true
	r.Revision = "2"
	key, _ := preparationKey(r)
	if err := persist.Write(key, []byte(`{"state":"started"}`)); err != nil {
		t.Fatal(err)
	}
	prepare = func(string, string, string, time.Time) (bool, string, error) {
		t.Fatal("repeated interrupted generation")
		return false, "", nil
	}
	send = func(_ request, _ string, err error) error {
		if err == nil {
			t.Fatal("interruption not reported")
		}
		sent++
		return nil
	}
	if err := consumePreparedWith(r, prepare, current, send); err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatal("missing failure delivery")
	}
}

func TestPreparedEmptyResultDoesNotClaimDelivery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := request{Account: "empty-reading-owner", ID: "reading", Revision: "1", EventID: "empty-reading-event", Due: time.Now().Add(-time.Second)}
	prepare := func(string, string, string, time.Time) (bool, string, error) { return true, "", nil }
	current := func(string, string, string) bool { return true }
	send := func(request, string, error) error { t.Fatal("sent empty result"); return nil }
	for i := 0; i < 2; i++ {
		if err := consumePreparedWith(r, prepare, current, send); err != nil {
			t.Fatal(err)
		}
	}
	key, _ := preparationKey(r)
	b, err := persist.Read(key)
	if err != nil {
		t.Fatal(err)
	}
	var result preparedResult
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "done" || result.MessageID != "" {
		t.Fatalf("%+v", result)
	}
}
