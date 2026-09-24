package event

import (
	"context"
	"errors"
	"mu/internal/persist"
	"sync"
	"testing"
)

func TestCheckpointFollowsSuccessfulHandling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, id := range []string{"one", "two"} {
		if err := Commit(map[string][]byte{"source.json": []byte(id)}, Record{Type: "test.created", Service: "test", Account: "owner", Resource: id}); err != nil {
			t.Fatal(err)
		}
	}
	fail := errors.New("temporary failure")
	var seen []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := Consume(ctx, "reader", []string{"test.created"}, func(r Record) error {
		seen = append(seen, r.Resource)
		if r.Resource == "two" {
			return fail
		}
		return nil
	})
	if !errors.Is(err, fail) {
		t.Fatalf("%v", err)
	}
	if len(seen) != 2 {
		t.Fatal(seen)
	}
	seen = nil
	err = Consume(ctx, "reader", []string{"test.created"}, func(r Record) error { seen = append(seen, r.Resource); cancel(); return nil })
	if !errors.Is(err, context.Canceled) || len(seen) != 1 || seen[0] != "two" {
		t.Fatalf("%v %v", seen, err)
	}
	b, err := persist.Read("source.json")
	if err != nil || string(b) != "two" {
		t.Fatalf("%s %v", b, err)
	}
	// A restart with the checkpoint must not redeliver either successful item.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if err := Consume(ctx2, "reader", []string{"test.created"}, func(Record) error { t.Fatal("redelivered"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestConsumerCannotRunTwiceAndPanicIsRetryable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Commit(nil, Record{Type: "test.created", Service: "test", Account: "owner", Resource: "one"}); err != nil {
		t.Fatal(err)
	}
	ready, release := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := Consume(context.Background(), "reader", []string{"test.created"}, func(Record) error { close(ready); <-release; panic("crash") })
		if err == nil {
			t.Error("panic swallowed as success")
		}
	}()
	<-ready
	if err := Consume(context.Background(), "reader", nil, func(Record) error { return nil }); err == nil {
		t.Error("concurrent consumer allowed")
	}
	close(release)
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	Consume(ctx, "reader", []string{"test.created"}, func(Record) error { calls++; cancel(); return nil })
	if calls != 1 {
		t.Fatalf("lost event after panic: %d", calls)
	}
}
