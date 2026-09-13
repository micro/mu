package hazards

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheDoesNotWaitAndRetainsDataOnFailure(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	c := feedCache[int]{items: []int{7}, at: time.Now().Add(-time.Hour), fetch: func() ([]int, error) { calls.Add(1); <-release; return nil, errors.New("offline") }}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			items, _, _ := c.read()
			if len(items) != 1 || items[0] != 7 {
				t.Error("cached results lost")
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("read waited for upstream")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		items, at, failed := c.read()
		if failed {
			if len(items) != 1 || at.IsZero() || calls.Load() != 1 {
				t.Fatal("failed refresh lost data or duplicated work")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("refresh did not finish")
}
