package mail

import (
	"context"
	"fmt"
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

func TestSearchPageUsesIndexedQuerySemantics(t *testing.T) {
	indexed(t)
	mutex.Lock()
	old := append([]*Message(nil), messages...)
	m := &Message{ID: "indexed_page", ToID: "indexed_owner", Subject: "Alpha report", Body: "invoice body", Tag: "work"}
	addMessage(m)
	mutex.Unlock()
	indexMessage(m)
	defer func() { unindexMessage(m); mutex.Lock(); setMessages(old); mutex.Unlock() }()
	hits := Search("indexed_owner", `"alpha report"`, 10)
	if len(hits) != 1 || hits[0].ID != m.ID {
		t.Fatalf("quoted indexed search lost: %+v", hits)
	}
	var rsp SearchResponse
	if err := messagePage("indexed_owner", `"alpha report"`, "work", 0, 10, true, &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Total != 1 || len(rsp.Items) != 1 || rsp.Items[0].ID != m.ID {
		t.Fatalf("indexed multi-field query lost: %+v", rsp)
	}
}

func TestSearchPageBeyondIndexFetchCap(t *testing.T) {
	indexed(t)
	mutex.Lock()
	old := append([]*Message(nil), messages...)
	var added []*Message
	for i := 0; i < 225; i++ {
		m := &Message{ID: fmt.Sprintf("pagecap_%03d", i), ToID: "pagecap_owner", Subject: "capneedle report"}
		if i >= 210 {
			m.Tag = "late"
		}
		addMessage(m)
		added = append(added, m)
	}
	other := &Message{ID: "pagecap_other", ToID: "pagecap_stranger", Subject: "capneedle report", Tag: "late"}
	addMessage(other)
	added = append(added, other)
	mutex.Unlock()
	for _, m := range added {
		indexMessage(m)
	}
	defer func() {
		for _, m := range added {
			unindexMessage(m)
		}
		mutex.Lock()
		setMessages(old)
		mutex.Unlock()
	}()
	for _, tc := range []struct {
		query, tag           string
		offset, total, count int
	}{
		{"capneedle", "", 200, 225, 10},
		{"capneedle", "late", 0, 15, 10},
		{"capneedle", "", 220, 225, 5},
		{"capneedle", "", 500, 225, 0},
		{"apneedle", "late", 0, 15, 10},
	} {
		var rsp SearchResponse
		if err := messagePage("pagecap_owner", tc.query, tc.tag, tc.offset, 10, true, &rsp); err != nil {
			t.Fatal(err)
		}
		if rsp.Total != tc.total || len(rsp.Items) != tc.count {
			t.Fatalf("%+v: %+v", tc, rsp)
		}
		if rsp.Offset+tc.count < tc.total {
			if rsp.NextOffset == nil || *rsp.NextOffset != rsp.Offset+tc.count {
				t.Fatalf("missing next page: %+v", rsp)
			}
		} else if rsp.NextOffset != nil {
			t.Fatalf("unexpected next page: %+v", rsp)
		}
		for _, item := range rsp.Items {
			if item.ID == other.ID {
				t.Fatal("cross-owner search result")
			}
		}
	}
}
