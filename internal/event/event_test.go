package event

import (
	"testing"
	"time"
)

func TestSubscribeAndPublish(t *testing.T) {
	sub := Subscribe(IndexComplete)
	defer sub.Close()

	Publish(Event{
		Type: IndexComplete,
		Data: map[string]interface{}{"id": "test-1"},
	})

	select {
	case e := <-sub.Chan:
		if e.Type != IndexComplete {
			t.Errorf("expected event type %q, got %q", IndexComplete, e.Type)
		}
		if e.Data["id"] != "test-1" {
			t.Errorf("expected data id 'test-1', got %v", e.Data["id"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribe_OnlyReceivesMatchingType(t *testing.T) {
	sub := Subscribe(GenerateSummary)
	defer sub.Close()

	// Publish a different event type
	Publish(Event{
		Type: IndexComplete,
		Data: map[string]interface{}{"id": "other"},
	})

	// Should not receive anything
	select {
	case e := <-sub.Chan:
		t.Errorf("should not receive non-matching event, got %+v", e)
	case <-time.After(50 * time.Millisecond):
		// Good - no event received
	}
}

func TestMultipleSubscribers(t *testing.T) {
	sub1 := Subscribe(GenerateTag)
	sub2 := Subscribe(GenerateTag)
	defer sub1.Close()
	defer sub2.Close()

	Publish(Event{
		Type: GenerateTag,
		Data: map[string]interface{}{"tag": "tech"},
	})

	for i, sub := range []*Subscription{sub1, sub2} {
		select {
		case e := <-sub.Chan:
			if e.Data["tag"] != "tech" {
				t.Errorf("subscriber %d: expected tag 'tech', got %v", i, e.Data["tag"])
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestClose_RemovesSubscription(t *testing.T) {
	sub := Subscribe(SummaryGenerated)
	sub.Close()

	// Publishing should not panic after close
	Publish(Event{
		Type: SummaryGenerated,
		Data: map[string]interface{}{},
	})
}

func TestPublish_NonBlockingOnFullChannel(t *testing.T) {
	sub := Subscribe(TagGenerated)
	defer sub.Close()

	// Fill the buffer (capacity 10)
	for i := 0; i < 15; i++ {
		Publish(Event{
			Type: TagGenerated,
			Data: map[string]interface{}{"i": i},
		})
	}

	// Should not have blocked - drain what we can
	count := 0
	for {
		select {
		case <-sub.Chan:
			count++
		default:
			goto done
		}
	}
done:
	if count != 10 {
		t.Errorf("expected 10 buffered events, got %d", count)
	}
}

func TestEventConstants(t *testing.T) {
	// Ensure event constants are unique
	constants := []string{
		RefreshHNComments,
		IndexComplete,
		NewArticleMetadata,
		GenerateSummary,
		SummaryGenerated,
		GenerateTag,
		TagGenerated,
	}
	seen := make(map[string]bool)
	for _, c := range constants {
		if seen[c] {
			t.Errorf("duplicate event constant: %q", c)
		}
		seen[c] = true
	}
}
