package server

import (
	"io"
	"mu/internal/app"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestsRespondWhileDataRestorationIsBlocked(t *testing.T) {
	ready := make(chan struct{})
	var calls atomic.Int32
	assets := app.Serve()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if startingResponse(w, r, ready, assets) {
			return
		}
		calls.Add(1)
		w.Write([]byte("restored"))
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = time.Second
	for _, tc := range []struct{ method, path, accept string }{
		{"GET", "/inbox?id=existing", "text/html"},
		{"GET", "/inbox", "application/json"},
		{"POST", "/agent/micro", "application/json"},
		{"POST", "/sms/webhook", ""},
		{"POST", "/mcp", ""},
		{"HEAD", "/", "text/html"},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader("do not execute"))
		req.Header.Set("Accept", tc.accept)
		rsp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request waited for restoration: %v", err)
		}
		body, _ := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if rsp.StatusCode != 503 || rsp.Header.Get("Retry-After") != "1" {
			t.Fatalf("%s %s: status=%d", tc.method, tc.path, rsp.StatusCode)
		}
		if tc.method == "POST" && rsp.Header.Get("Refresh") != "" {
			t.Fatal("mutation would be automatically resubmitted")
		}
		if tc.method == "HEAD" && len(body) > 0 {
			t.Fatal("HEAD returned a body")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("uninitialized handler was reached")
	}
	// Restore finishes: the same listener and URL immediately use the real handler.
	close(ready)
	rsp, err := client.Get(server.URL + "/inbox?id=existing")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rsp.Body)
	rsp.Body.Close()
	if rsp.StatusCode != 200 || string(body) != "restored" || calls.Load() != 1 {
		t.Fatalf("handoff failed: %s", body)
	}
}

func TestStartupServesStylesWithoutUsingUnregisteredRoutes(t *testing.T) {
	ready := make(chan struct{})
	req := httptest.NewRequest("GET", "/mu.css", nil)
	w := httptest.NewRecorder()
	if !startingResponse(w, req, ready, app.Serve()) {
		t.Fatal("not handled")
	}
	if w.Code != 200 || !strings.Contains(w.Body.String(), "font-weight") {
		t.Fatalf("missing shared stylesheet: %d", w.Code)
	}
	// An API that happens to end in .json must still receive a retryable response.
	req = httptest.NewRequest("GET", "/api/v1/private.json", nil)
	w = httptest.NewRecorder()
	startingResponse(w, req, ready, app.Serve())
	if w.Code != 503 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("API bypassed readiness gate")
	}
}
