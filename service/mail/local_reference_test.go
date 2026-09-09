package mail

import (
	"mu/internal/event"
	"testing"
	"time"
)

func TestLocalDeliveryKeepsAReferenceForInbox(t *testing.T) {
	sub := event.Subscribe(event.MailReceived)
	defer sub.Close()
	const owner = "brief_reference"
	if err := SendMessageTo(Delivery{FromID: "agent@example.test", ToID: owner, Body: "**Morning brief**", Tag: "scheduled"}); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(3 * time.Second)
	for {
		select {
		case e := <-sub.Chan:
			m, ok := MessageFrom(e.Data)
			if !ok || m.Owner != owner {
				continue
			}
			if m.MessageID == "" {
				t.Fatal("local mail has no transcript reference")
			}
			stored := FindMessageByMessageID(m.MessageID)
			if stored == nil || stored.ToID != owner || stored.Body != m.Body {
				t.Fatal("reference does not join to owned mail")
			}
			return
		case <-timeout:
			t.Fatal("no mail arrival")
		}
	}
}
