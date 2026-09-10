package mail

import "testing"

func TestMarkReadOwnInboxOnly(t *testing.T) {
	mutex.Lock()
	old := messages
	setMessages([]*Message{
		{ID: "mark-a", ThreadID: "mark-a", ToID: "mark-owner", FromID: "sender", Body: "hello"},
		{ID: "mark-b", ThreadID: "mark-b", ToID: "mark-other", FromID: "sender", Body: "private"},
		{ID: "mark-spam", ThreadID: "mark-spam", ToID: "mark-owner", Spam: true},
	})
	rebuildInboxes()
	mutex.Unlock()
	defer func() { mutex.Lock(); setMessages(old); rebuildInboxes(); mutex.Unlock() }()
	if _, err := markRead("mark-owner", []string{"mark-a", "mark-b"}, false); err == nil {
		t.Fatal("accepted another owner's mail")
	}
	if messages[0].Read {
		t.Fatal("partially applied invalid selection")
	}
	if _, err := markRead("mark-owner", nil, false); err == nil {
		t.Fatal("implicit bulk action")
	}
	n, err := markRead("mark-owner", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || !messages[0].Read || messages[1].Read || messages[2].Read {
		t.Fatal("incorrect bulk scope")
	}
	if GetUnreadCount("mark-owner") != 0 {
		t.Fatal("unread count not updated")
	}
	if n, err := markRead("mark-owner", nil, true); err != nil || n != 0 {
		t.Fatal("not idempotent")
	}
}
