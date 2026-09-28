package thread

import "testing"

func TestMergePreservesTransportIdentityAndHistory(t *testing.T) {
	owner := "merge-history"
	defer Forget(owner)
	source := Open(owner, "mail", "root")
	target := Open(owner, WebClient, "checkin:"+source.ID)
	original := Add(Message{Account: owner, Thread: source.ID, Text: "Hello", Ref: "<original@test>"})
	Add(Message{Account: owner, Thread: target.ID, Text: "Hello", Ref: "checkin-import:" + original})
	Add(Message{Account: owner, Thread: target.ID, Text: "Web reply", Ref: "web-reply"})
	if err := Merge(owner, source.ID, target.ID, map[string]bool{"checkin-import:" + original: true}, nil); err != nil {
		t.Fatal(err)
	}
	if len(List(owner, 0)) != 1 || len(Messages(owner, source.ID, 0)) != 2 {
		t.Fatal("split or lost history")
	}
	if Get(owner, source.ID).ID != target.ID || Find(owner, "mail", "root").ID != target.ID || ByRef(owner, "<original@test>").ID != target.ID {
		t.Fatal("legacy identity lost")
	}
	if Add(Message{Account: owner, Thread: source.ID, Text: "Hello", Ref: "<original@test>"}) != original {
		t.Fatal("duplicate native mail")
	}
	Add(Message{Account: owner, Thread: source.ID, Text: "Next mail", Ref: "<next@test>"})
	if len(Messages(owner, target.ID, 0)) != 3 {
		t.Fatal("mail continued a separate conversation")
	}
	MarkSeen(owner, source.ID)
	if UnreadCount(owner) != 0 {
		t.Fatal("alias remains unread")
	}
	foreign := Open("merge-other", WebClient, "private")
	defer Forget("merge-other")
	if err := Merge(owner, target.ID, foreign.ID, nil, nil); err == nil || Get("merge-other", source.ID) != nil {
		t.Fatal("cross-account merge")
	}
	Delete(owner, source.ID)
	if Get(owner, target.ID) != nil || Find(owner, "mail", "root") != nil {
		t.Fatal("delete left a live alias")
	}
	if err := Flush(); err != nil {
		t.Fatal(err)
	}
}
