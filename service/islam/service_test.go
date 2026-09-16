package islam

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func upstream(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	s := httptest.NewServer(h)
	old := knowledgeBase
	knowledgeBase = s.URL
	t.Cleanup(func() { knowledgeBase = old; s.Close() })
}

func TestSearchAndReadPreserveSources(t *testing.T) {
	upstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials forwarded")
		}
		switch r.URL.Path {
		case "/api/knowledge/search":
			if r.URL.Query().Get("q") != "patience & prayer" || r.URL.Query().Get("collection") != "quran" {
				t.Error(r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"results":[{"Kind":"quran","Title":"2:153","Content":"excerpt","URL":"/quran/2/153","Source":"Quran"},{"Kind":"chat","Content":"must not surface"}]}`)
		case "/api/knowledge/resource":
			if r.URL.Query().Get("path") != "/quran/2/153" {
				t.Error(r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"kind":"quran","source":"Quran","resource":{"Text":"complete passage","Arabic":"عربي","Commentary":"separate commentary"}}`)
		default:
			t.Error(r.URL.Path)
		}
	})
	var found SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "patience & prayer", Collection: "quran"}, &found); err != nil {
		t.Fatal(err)
	}
	if len(found.Results) != 1 || found.Results[0].URL != "https://aslam.org/quran/2/153" || found.Results[0].Source != "Quran" {
		t.Fatalf("%+v", found)
	}
	var read ReadResponse
	if err := (Server{}).Read(context.Background(), &ReadRequest{Path: found.Results[0].URL}, &read); err != nil {
		t.Fatal(err)
	}
	if read.Resource["Arabic"] != "عربي" || read.Resource["Commentary"] != "separate commentary" || read.URL != found.Results[0].URL {
		t.Fatalf("%+v", read)
	}
}

func TestRejectsPrivateAndForeignResources(t *testing.T) {
	for _, p := range []string{"/chat/1", "/notes/1", "https://evil.test/quran/2/1", "//aslam.org/quran/2/1", "/quran/../chat/1", "https://user@aslam.org/quran/2/1", "/quran/2/1?q=private"} {
		if _, err := resourcePath(p); err == nil {
			t.Errorf("accepted %s", p)
		}
	}
}

func TestUpstreamFailuresAndCancellation(t *testing.T) {
	for _, mode := range []string{"status", "json", "oversize", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			upstream(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "status":
					w.WriteHeader(503)
				case "json":
					fmt.Fprint(w, "<html>error</html>")
				case "oversize":
					fmt.Fprint(w, strings.Repeat("x", (4<<20)+1))
				case "cancel":
					t.Error("cancelled request reached upstream")
				}
			})
			ctx := context.Background()
			if mode == "cancel" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			if err := (Server{}).Search(ctx, &SearchRequest{Query: "patience"}, &SearchResponse{}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
