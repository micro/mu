package work

import (
	"encoding/json"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/tasks"
	"testing"
	"time"
)

func TestWorkAPIRejectsForeignAndPersonalItems(t *testing.T) {
	for _, who := range []string{"work_public_owner", "work_public_other"} {
		if err := auth.Create(&auth.Account{ID: who, Secret: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	delegated, err := tasks.Create("work_public_owner", "goal", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	personal, err := tasks.Create("work_public_owner", "todo", "", tasks.Me, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range PublicOperations() {
		if op.Name != "work_get" && op.Name != "work_retry" {
			continue
		}
		for _, tc := range []struct{ owner, id string }{{"work_public_other", delegated.ID}, {"work_public_owner", personal.ID}} {
			raw, _ := json.Marshal(map[string]string{"id": tc.id})
			_, err := op.Handle(tc.owner, raw)
			if e, ok := err.(*api.Failure); !ok || e.Status != 404 {
				t.Fatalf("%s: %v", op.Name, err)
			}
		}
	}
	th := thread.Open("work_public_other", thread.WebClient, "work-public-foreign")
	for _, op := range PublicOperations() {
		if op.Name == "work_submit" {
			raw, _ := json.Marshal(map[string]string{"prompt": "goal", "thread": th.ID})
			_, err := op.Handle("work_public_owner", raw)
			if e, ok := err.(*api.Failure); !ok || e.Status != 404 {
				t.Fatalf("foreign thread: %v", err)
			}
		}
	}
}
