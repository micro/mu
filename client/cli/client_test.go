package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProtocolBoundary(t *testing.T) {
	for _, s := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/path", "https://example.com?token=secret", "https://example.com#fragment"} {
		if _, e := origin(s); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
	for _, s := range []string{"https://micro.mu/", "http://localhost:8080", "http://[::1]:8080"} {
		if _, e := origin(s); e != nil {
			t.Error(e)
		}
	}
	var leaked atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.Method != "POST" || r.URL.Path != "/" {
			t.Error("invalid request")
		}
		switch r.Header.Get("Micro-Endpoint") {
		case "redirect":
			http.Redirect(w, r, destination.URL, 307)
		case "wrong":
			w.Header().Set("Micro-Protocol", "mucp/1")
			w.Header().Set("Micro-Id", "wrong")
			fmt.Fprint(w, `{}`)
		default:
			w.Header().Set("Micro-Protocol", "mucp/1")
			w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer server.Close()
	c := &client{server: server.URL, token: "secret", http: transport()}
	var out any
	for _, ep := range []string{"redirect", "wrong"} {
		if err := c.call(context.Background(), "assistant", ep, map[string]any{}, &out); err == nil {
			t.Errorf("accepted %s", ep)
		}
	}
	if leaked.Load() != 0 {
		t.Fatal("followed credential redirect")
	}
	if err := c.call(context.Background(), "assistant", "valid", map[string]any{}, &out); err != nil {
		t.Fatal(err)
	}
}

func TestDurableRequestRecovery(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MU_TOKEN", "secret")
	var submits, reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Micro-Protocol", "mucp/1")
		w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
		switch r.Header.Get("Micro-Endpoint") {
		case "Assistant.Submit":
			submits.Add(1)
			var q struct {
				MessageID string `json:"messageId"`
				Request   struct {
					Prompt string `json:"prompt"`
					Mode   string `json:"mode"`
				}
			}
			json.NewDecoder(r.Body).Decode(&q)
			if q.MessageID != "stable-message-123456" || q.Request.Prompt != "question" || q.Request.Mode != "ACCESS_MODE_PERSONAL" {
				t.Error("submission changed")
			}
			fmt.Fprint(w, `{"id":"operation","thread":"thread","state":"OPERATION_STATE_RUNNING"}`)
		case "Assistant.ReadOperation":
			reads.Add(1)
			fmt.Fprint(w, `{"id":"operation","thread":"thread","state":"OPERATION_STATE_SUCCEEDED","result":{"text":"Answer","content":[{"fallback":"Source"}]}}`)
		default:
			t.Error("unexpected call")
		}
	}))
	defer server.Close()
	var out, diag bytes.Buffer
	err := run(context.Background(), []string{"--server", server.URL, "--timeout", "50ms", "--message-id", "stable-message-123456", "ask", "-"}, strings.NewReader("question"), &out, &diag)
	if err == nil || !strings.Contains(err.Error(), "operation operation") || submits.Load() != 1 || reads.Load() != 0 {
		t.Fatalf("did not preserve receipt: %v, %s", err, diag.String())
	}
	out.Reset()
	if err = run(context.Background(), []string{"--server", server.URL, "operation", "operation"}, strings.NewReader(""), &out, &diag); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Answer\nSource\n" || submits.Load() != 1 || reads.Load() != 1 {
		t.Fatalf("incorrect recovery: %q", out.String())
	}
}

func TestBrowserPKCE(t *testing.T) {
	var redirect, challenge string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/register":
			var q struct {
				Redirects []string `json:"redirect_uris"`
			}
			json.NewDecoder(r.Body).Decode(&q)
			redirect = q.Redirects[0]
			fmt.Fprint(w, `{"client_id":"client"}`)
		case "/oauth/token":
			r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("redirect_uri") != redirect || r.Form.Get("code") != "accepted" {
				t.Error("PKCE failed")
			}
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`)
		default:
			t.Error("unexpected OAuth request")
		}
	}))
	defer server.Close()
	open := func(address string) error {
		u, _ := url.Parse(address)
		q := u.Query()
		challenge = q.Get("code_challenge")
		if q.Get("code_challenge_method") != "S256" {
			t.Error("missing S256")
		}
		for _, query := range []string{"state=wrong&code=accepted", "state=" + q.Get("state") + "&state=duplicate&code=accepted"} {
			r, e := http.Get(redirect + "?" + query)
			if e != nil {
				return e
			}
			r.Body.Close()
			if r.StatusCode != 400 {
				t.Error("accepted invalid state")
			}
		}
		callback := redirect + "?state=" + q.Get("state") + "&code=accepted"
		r, e := http.Get(callback)
		if e != nil {
			return e
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Error("rejected callback")
		}
		r, e = http.Get(callback)
		if e != nil {
			return e
		}
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Error("accepted replay")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, err := browserLogin(ctx, transport(), server.URL, &bytes.Buffer{}, open)
	if err != nil {
		t.Fatal(err)
	}
	if s.Token != "access" || s.Refresh != "refresh" || s.Client != "client" {
		t.Fatal(s)
	}
}

func TestRefreshSerializedAndOriginBound(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("refresh_token") != "old" {
			t.Error("wrong refresh token")
		}
		refreshes.Add(1)
		fmt.Fprint(w, `{"access_token":"new","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "session.json")
	if err := saveSession(path, session{Server: server.URL, Token: "expired", Client: "client", Refresh: "old", Expires: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			token, err := credentials(context.Background(), path, server.URL, transport())
			if err != nil || token != "new" {
				t.Errorf("renewal: %s %v", token, err)
			}
		})
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("refreshed %d times", refreshes.Load())
	}
	if _, err := credentials(context.Background(), path, "https://another.example", transport()); err == nil {
		t.Fatal("cross-origin token use")
	}
	s, err := readSession(path)
	if err != nil || s.Refresh != "rotated" {
		t.Fatal("rotation not saved")
	}
}

func TestTokenInputAndOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	t.Setenv("MU_URL", "https://micro.mu")
	t.Setenv("MU_TOKEN", "")
	var out, diag bytes.Buffer
	if err := runWithConfig(context.Background(), []string{"login", "--token-stdin"}, strings.NewReader("secret\n"), &out, &diag, path); err != nil {
		t.Fatal(err)
	}
	s, err := readSession(path)
	if err != nil || s.Token != "secret" {
		t.Fatal("token not saved")
	}
	if strings.Contains(out.String()+diag.String(), "secret") {
		t.Fatal("token exposed")
	}
	if err = runWithConfig(context.Background(), []string{"logout"}, strings.NewReader(""), &out, &diag, path); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("session not removed")
	}
	if strings.ContainsAny(safeText("\x1b]52;clipboard\aHello\r\u009b"), "\x1b\a\r\u009b") {
		t.Fatal("terminal control escaped filtering")
	}
}
