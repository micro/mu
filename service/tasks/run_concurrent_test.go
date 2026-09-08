package tasks

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mu/internal/event"
)

func TestConcurrentStartsPublishWorkOnce(t *testing.T) {
	setupTasks(t)
	task, err := CreateOn("alice", "source-thread", "specialist", "Do this once", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if Run("alice", task.ID) == nil {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d starts, want one", accepted.Load())
	}
	select {
	case e := <-sub.Chan:
		if e.Data["thread"] != "source-thread" || e.Data["agent"] != "specialist" || e.Data["account"] != "alice" {
			t.Fatalf("lost delivery identity: %v", e.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted work was not published")
	}
	select {
	case <-sub.Chan:
		t.Fatal("duplicate work published")
	default:
	}
	stored, err := Get("alice", task.ID)
	if err != nil || stored.Status != StatusDoing {
		t.Fatalf("claimed task not persisted: %v, %v", stored, err)
	}
	if err := Run("bob", task.ID); err == nil {
		t.Fatal("another account claimed the task")
	}
}
