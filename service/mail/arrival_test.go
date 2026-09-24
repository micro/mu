package mail

import (
	"context"
	"encoding/json"
	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
	"strings"
	"testing"
)

func TestArrivalCommitsReferenceAndTrustWithMail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := Delivery{ToID: "arrival-owner", FromID: "person@example.com", Body: "private arrival text", MessageID: "<arrival-test>", arrival: &arrival{Shared: true, To: "agent@example.net", Authenticated: true, Owned: true, InReplyTo: "<prior>", References: "<root> <prior>"}}
	if err := SendMessageTo(d); err != nil {
		t.Fatal(err)
	}
	// SMTP retransmission must not queue a second agent run.
	if err := SendMessageTo(d); err != nil {
		t.Fatal(err)
	}
	names, err := persist.List("outbox/log")
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, name := range names {
		b, err := persist.Read("outbox/log/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), d.Body) {
			t.Fatal("private body copied into event")
		}
		var e event.Record
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatal(err)
		}
		if e.Type != event.MailAccepted {
			continue
		}
		accepted++
		for _, owner := range []string{d.ToID, "other"} {
			var rsp service.SourceResponse
			if err := (Server{}).Source(service.WithAccount(context.Background(), owner), &service.SourceRequest{ID: e.Resource}, &rsp); err != nil {
				t.Fatal(err)
			}
			if owner != "arrival-owner" {
				if rsp.Item != nil {
					t.Fatal("cross-account mail")
				}
				continue
			}
			if rsp.Item == nil || rsp.Item.Facts["authenticated"] != true || rsp.Item.Text != d.Body || rsp.Item.InReplyTo != "<prior>" {
				t.Fatalf("%+v", rsp.Item)
			}
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted events: %d", accepted)
	}
}
