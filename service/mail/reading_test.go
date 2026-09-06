package mail

import (
	"context"
	"mu/internal/service"
	"strings"
	"testing"
)

func TestReadOwnershipAndBodyPaging(t *testing.T) {
	mutex.Lock()
	old := messages
	setMessages([]*Message{{ID: "paging", ToID: "reader", Body: strings.Repeat("λ", 4500), Subject: "Long", Tag: "work"}})
	mutex.Unlock()
	defer func() { mutex.Lock(); setMessages(old); mutex.Unlock() }()
	var rsp ReadResponse
	if err := (Server{}).Read(service.WithAccount(context.Background(), "other"), &ReadRequest{ID: "paging"}, &rsp); err == nil {
		t.Fatal("other account read mail")
	}
	ctx := service.WithAccount(context.Background(), "reader")
	if err := (Server{}).Read(ctx, &ReadRequest{ID: "paging"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if len([]rune(rsp.Item.Body)) != 2000 || rsp.NextOffset != 2000 || rsp.Total != 4500 {
		t.Fatalf("bad page: %d %d", rsp.Total, rsp.NextOffset)
	}
	var page SearchResponse
	if err := messagePage("reader", "", "missing", 0, 10, false, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatal("tag filter ignored")
	}
}
