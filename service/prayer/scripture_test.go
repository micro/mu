package prayer

// The client that fetches a chapter, a saying or an answer.
//
// These moved here with the three endpoints they serve. They used to test the
// same two helpers inside internal/api, which had its own copy of an HTTP
// client for reminder.dev — a package that may not import a service, holding
// the network call for the one service that already talks to that host.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScriptureLookupRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/daily" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	origBase := scriptureBase
	origClient := scriptureClient
	scriptureBase = server.URL + "/"
	scriptureClient = server.Client()
	defer func() {
		scriptureBase = origBase
		scriptureClient = origClient
	}()

	_, err := scriptureGet("/daily")
	if err == nil {
		t.Fatal("expected non-success status to return an error")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestScriptureLookupUsesConfiguredBaseAndClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: %s", got)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBase := scriptureBase
	origClient := scriptureClient
	scriptureBase = server.URL + "/api/"
	scriptureClient = server.Client()
	defer func() {
		scriptureBase = origBase
		scriptureClient = origClient
	}()

	got, err := scripturePost("/search", "application/json", `{"q":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", got)
	}
}
