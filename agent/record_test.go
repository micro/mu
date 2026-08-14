package agent

// A run nobody watched still has to be findable.

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAMailRunIsWrittenDownWhereTheOwnerLooks(t *testing.T) {
	before := len(ListFlows("recowner"))

	started := time.Now().Add(-30 * time.Second)
	id := Record(Recorded{
		Account: "recowner",
		Source:  FromMail,
		Trigger: "email from stranger@example.com",
		Prompt:  "What is the weather in Reading?",
		Answer:  "Cloudy, 18°C.",
		Started: started,
	})
	if id == "" {
		t.Fatal("the run was not recorded")
	}

	runs := ListFlows("recowner")
	if len(runs) != before+1 {
		t.Fatalf("recorded %d runs, want one more than %d", len(runs), before)
	}
	var f *Flow
	for _, r := range runs {
		if r.ID == id {
			f = r
		}
	}
	if f == nil {
		t.Fatal("the recorded run is not in the account's list")
	}
	if f.Status != "done" {
		t.Errorf("status %q, want done", f.Status)
	}
	// Who set it off is the whole point: somebody else's mail spent this
	// account's credits, and the owner has to be able to see that.
	if f.Source != FromMail || !strings.Contains(f.Trigger, "stranger@example.com") {
		t.Errorf("lost where the run came from: source=%q trigger=%q", f.Source, f.Trigger)
	}
	if f.Answer != "Cloudy, 18°C." {
		t.Errorf("lost what was said on the owner's behalf: %q", f.Answer)
	}
	if f.HTML == "" {
		t.Error("no rendered form, so the run reads as raw text next to the watched ones")
	}
	if !f.CreatedAt.Equal(started) {
		t.Errorf("stamped %v, want when the run began (%v)", f.CreatedAt, started)
	}
}

func TestAFailedRunSaysSoRatherThanLookingDone(t *testing.T) {
	id := Record(Recorded{
		Account: "recowner",
		Source:  FromMail,
		Prompt:  "anything",
		Err:     fmt.Errorf("the model was unreachable"),
	})
	f := getFlow(id)
	if f == nil {
		t.Fatal("not recorded")
	}
	if f.Status != "error" || !strings.Contains(f.Error, "unreachable") {
		t.Errorf("status=%q error=%q, want the failure recorded", f.Status, f.Error)
	}
}

func TestAnUndeliveredReplyStopsLookingDelivered(t *testing.T) {
	// The case this exists for: the agent answered, the answer was recorded,
	// and then the mail server could not deliver it.
	id := Record(Recorded{
		Account: "recowner",
		Source:  FromMail,
		Prompt:  "anything",
		Answer:  "here you are",
	})
	if f := getFlow(id); f == nil || f.Status != "done" {
		t.Fatal("setup: the run should start out done")
	}

	Failed(id, fmt.Errorf("no MX for example.invalid"))

	f := getFlow(id)
	if f.Status != "error" {
		t.Errorf("status %q — a reply that never arrived still reads as delivered", f.Status)
	}
	if !strings.Contains(f.Error, "no MX") {
		t.Errorf("lost why it failed: %q", f.Error)
	}
	// The answer stays: what the agent wrote is the record, whether or not it
	// got there.
	if f.Answer != "here you are" {
		t.Errorf("dropped the answer on a failed delivery: %q", f.Answer)
	}
}

func TestRecordingNeedsAnAccount(t *testing.T) {
	if id := Record(Recorded{Source: FromMail, Prompt: "x"}); id != "" {
		t.Error("recorded a run against nobody")
	}
}
