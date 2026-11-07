package islam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKnowledgeSearchAndReadPreserveSourceDistinctions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/knowledge/search":
			if r.URL.Query().Get("collection") != "quran" {
				t.Error("collection lost")
			}
			w.Write([]byte(`{"results":[{"kind":"quran","title":"Passage","content":"excerpt","source":"translation","url":"/quran/2/153"}]}`))
		case "/api/knowledge/resource":
			if r.URL.Query().Get("path") != "/quran/2/153" {
				t.Error("resource lost")
			}
			w.Write([]byte(`{"kind":"quran","source":"published source","resource":{"arabic":"original","translation":"translation","commentary":"commentary"}}`))
		default:
			t.Error("unexpected endpoint")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldBase, oldClient := knowledgeBase, knowledgeClient
	knowledgeBase, knowledgeClient = server.URL, server.Client()
	defer func() { knowledgeBase, knowledgeClient = oldBase, oldClient }()
	var results SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "patience", Collection: "quran"}, &results); err != nil {
		t.Fatal(err)
	}
	var record ReadResponse
	if err := (Server{}).Read(context.Background(), &ReadRequest{Path: results.Results[0].URL}, &record); err != nil {
		t.Fatal(err)
	}
	if record.Resource["arabic"] != "original" || record.Resource["translation"] != "translation" || record.Resource["commentary"] != "commentary" || record.URL != "https://aslam.org/quran/2/153" {
		t.Fatalf("source distinctions lost: %+v", record)
	}
}

func TestKnowledgeReadRejectsForeignResources(t *testing.T) {
	for _, path := range []string{"https://example.org/quran/2/153", "//example.org/quran/2/153", "/quran/../private", "/quran/2/153?token=secret"} {
		if err := (Server{}).Read(context.Background(), &ReadRequest{Path: path}, &ReadResponse{}); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}
