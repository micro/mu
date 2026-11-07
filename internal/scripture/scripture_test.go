package scripture

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReferencesNeverRequestOrReturnGeneratedAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/search" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if summary, ok := body["summarise"]; !ok || summary != false {
			t.Error("must explicitly disable summary")
		}
		if body["q"] != "patience" {
			t.Error("query changed")
		}
		w.Write([]byte(`{"q":"patience","answer":"generated answer must not escape","references":[{"text":"source passage","score":0.9,"metadata":{"source":"quran","chapter":"2","verse":"153","translation":"translator"}}]}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTP: server.Client()}
	got, err := client.References(context.Background(), " patience ")
	if err != nil {
		t.Fatal(err)
	}
	if got.References[0].Metadata["translation"] != "translator" {
		t.Fatal("metadata lost")
	}
	var legacy SearchResponse
	if err := client.Search(context.Background(), &SearchRequest{Query: "patience"}, &legacy); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(legacy.Text, "generated answer") || !strings.Contains(legacy.Text, "references") {
		t.Fatal("legacy response must contain only references")
	}
}

func TestReferenceFailuresAndEmptyResults(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		valid  bool
	}{
		{`{"references":[]}`, 200, true},
		{`{"answer":"plausible prose"}`, 200, false},
		{`{"references":[{"text":"no provenance"}]}`, 200, false},
		{`<html>error</html>`, 200, false},
		{``, 200, false},
		{`{}`, 503, false},
		{strings.Repeat("x", (4<<20)+1), 200, false},
	} {
		t.Run(tc.body[:min(len(tc.body), 30)], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer server.Close()
			client := Client{BaseURL: server.URL, HTTP: server.Client()}
			_, err := client.References(context.Background(), "patience")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestScriptureContextAndValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached upstream") }))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTP: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.References(ctx, "patience"); err == nil {
		t.Fatal("ignored cancellation")
	}
	if err := client.Verse(context.Background(), &VerseRequest{Chapter: 2, Verse: -1}, &VerseResponse{}); err == nil {
		t.Fatal("negative verse accepted")
	}
	if err := client.Saying(context.Background(), &SayingRequest{Book: -1}, &SayingResponse{}); err == nil {
		t.Fatal("negative book accepted")
	}
	if _, err := client.References(context.Background(), strings.Repeat("x", 1001)); err == nil {
		t.Fatal("unbounded query")
	}
}

func TestLegacyCollectionRetainsLargerBound(t *testing.T) {
	body := `{"books":[{"text":"` + strings.Repeat("x", 4<<20) + `"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hadith" {
			t.Error("wrong legacy endpoint")
		}
		w.Write([]byte(body))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTP: server.Client()}
	var response SayingResponse
	if err := client.Saying(context.Background(), &SayingRequest{}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Text != body {
		t.Fatal("legacy record changed")
	}
}
