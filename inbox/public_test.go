package inbox

import (
	"encoding/json"
	"mu/internal/api"
	"mu/internal/thread"
	"testing"
)

func TestPublicInboxKeepsAccountBoundary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	th := thread.Open("public-inbox-owner", thread.WebClient, "owned")
	raw, _ := json.Marshal(map[string]string{"id": th.ID})
	for _, op := range PublicOperations() {
		if op.Name == "inbox_list" {
			continue
		}
		_, err := op.Handle("public-inbox-other", raw)
		if e, ok := err.(*api.Failure); !ok || e.Status != 404 {
			t.Errorf("%s read or changed a foreign conversation: %v", op.Name, err)
		}
	}
	for _, op := range PublicOperations() {
		if op.Name == "inbox_read" {
			if _, err := op.Handle("public-inbox-owner", raw); err != nil {
				t.Fatal(err)
			}
		}
	}
}
