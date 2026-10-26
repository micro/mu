package mobile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type device struct{ calls int }

func (d *device) Apps() (string, error) {
	d.calls++
	return `[{"name":"Camera","package":"test.camera"}]`, nil
}

func TestToolAndConversationSurviveRestart(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing key")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		b, _ := json.Marshal(body["messages"])
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"apps-1","type":"function","function":{"name":"device_apps","arguments":"{}"}}]}}]}`))
			return
		}
		if calls == 2 && !strings.Contains(string(b), "test.camera") {
			t.Error("native tool result not sent to model")
		}
		if calls == 3 && !strings.Contains(string(b), "Which apps can I open?") {
			t.Error("prior question missing after restart")
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Camera is available."}}]}`))
	}))
	defer server.Close()
	old := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = old }()
	dir := t.TempDir()
	d := &device{}
	r, err := Open(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Ask(server.URL+"/v1", "test-key", "test-model", "Which apps can I open?"); err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatalf("tool calls: %d", d.calls)
	}
	r, err = Open(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Ask(server.URL, "test-key", "test-model", "What was that app?"); err != nil {
		t.Fatal(err)
	}
	history, _ := r.History()
	if !strings.Contains(history, "Which apps can I open?") || !strings.Contains(history, "What was that app?") {
		t.Fatal(history)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "conversation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "test-key") {
		t.Fatal("credential saved in transcript")
	}
	// Full on-disk transcript is not truncated by local.Session's bounded context.
	for i := 0; i < 12; i++ {
		if _, err = r.Ask(server.URL, "test-key", "test-model", "Another question"); err != nil {
			t.Fatal(err)
		}
	}
	r, err = Open(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.history) != 14 {
		t.Fatalf("lost transcript: %d", len(r.history))
	}
}
func TestFailedRequestAndCorruptStorage(t *testing.T) {
	dir := t.TempDir()
	d := &device{}
	r, _ := Open(dir, d)
	for _, endpoint := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?key=secret"} {
		if _, err := r.Ask(endpoint, "key", "model", "hello"); err == nil {
			t.Fatal("accepted unsafe endpoint")
		}
	}
	if len(r.history) != 0 {
		t.Fatal("failed request persisted")
	}
	path := filepath.Join(dir, "conversation.json")
	os.WriteFile(path, []byte("broken"), 0600)
	if _, err := Open(dir, d); err == nil {
		t.Fatal("silently discarded corrupt history")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken" {
		t.Fatal("overwrote corrupt history")
	}
}
func TestRedirectDoesNotForwardConversation(t *testing.T) {
	reached := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	old := http.DefaultClient
	ConfigureTransport()
	http.DefaultClient.Transport = source.Client().Transport
	defer func() { http.DefaultClient = old }()
	r, _ := Open(t.TempDir(), &device{})
	if _, err := r.Ask(source.URL, "key", "model", "private message"); err == nil {
		t.Fatal("redirect succeeded")
	}
	if reached {
		t.Fatal("forwarded private conversation")
	}
}

func TestToolFollowupFailureIsNotSaved(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Let me check.","tool_calls":[{"id":"apps-1","type":"function","function":{"name":"device_apps","arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()
	old := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = old }()
	r, _ := Open(t.TempDir(), &device{})
	if _, err := r.Ask(server.URL, "key", "model", "List apps"); err == nil {
		t.Fatal("accepted narration after failed follow-up")
	}
	if len(r.history) != 0 {
		t.Fatal("saved incomplete turn")
	}
}
