package agent

import (
	"errors"
	"mu/internal/event"
	"mu/internal/persist"
	"testing"
)

func TestEventReactionRetriesLookupButNeverRepeatsExecution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	e := event.Record{ID: "00000000000000000001", Account: "owner"}
	calls := 0
	unavailable := errors.New("source unavailable")
	if err := reactOnce("test", e, func(event.Record) (func(), error) { return nil, unavailable }); !errors.Is(err, unavailable) {
		t.Fatal(err)
	}
	prepare := func(event.Record) (func(), error) { return func() { calls++ }, nil }
	if err := reactOnce("test", e, prepare); err != nil {
		t.Fatal(err)
	}
	if err := reactOnce("test", e, func(event.Record) (func(), error) { t.Fatal("completed event reread"); return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("executions: %d", calls)
	}
}

func TestInterruptedEventCannotRepeatSideEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	e := event.Record{ID: "00000000000000000002", Account: "owner"}
	calls := 0
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected interruption")
			}
		}()
		_ = reactOnce("test", e, func(event.Record) (func(), error) {
			return func() { calls++; panic("process interrupted after action") }, nil
		})
	}()
	if err := reactOnce("test", e, func(event.Record) (func(), error) { t.Fatal("interrupted action prepared again"); return nil, nil }); err != nil {
		t.Fatal(err)
	}
	b, err := persist.Read("agent/events/test/" + e.ID + ".json")
	if err != nil || string(b) != `"interrupted"` || calls != 1 {
		t.Fatalf("%s %v executions=%d", b, err, calls)
	}
}
