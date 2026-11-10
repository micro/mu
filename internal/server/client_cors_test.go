package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCORSIsNarrowAndNeverUsesCookies(t *testing.T) {
	for _, path := range []string{"/", "/oauth/register", "/oauth/token"} {
		r := httptest.NewRequest("OPTIONS", "https://runtime.example"+path, nil)
		r.Header.Set("Origin", "https://client.example")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "content-type,authorization,micro-protocol,micro-service,micro-endpoint,micro-id")
		w := httptest.NewRecorder()
		if !clientCORS(w, r) || w.Code != 204 || w.Header().Get("Access-Control-Allow-Credentials") != "" || w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("preflight %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/account", "/home", "/admin", "/oauth/authorize"} {
		r := httptest.NewRequest("OPTIONS", "https://runtime.example"+path, nil)
		r.Header.Set("Origin", "https://client.example")
		w := httptest.NewRecorder()
		if clientCORS(w, r) || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("exposed runtime browser route")
		}
	}
	for _, bearer := range []bool{false, true} {
		r := httptest.NewRequest("POST", "https://runtime.example/", nil)
		r.Header.Set("Origin", "https://client.example")
		r.Header.Set("Micro-Protocol", "mucp/1")
		r.AddCookie(&http.Cookie{Name: "session", Value: "ambient"})
		if bearer {
			r.Header.Set("Authorization", "Bearer scoped-token")
		}
		w := httptest.NewRecorder()
		done := clientCORS(w, r)
		if !bearer && (!done || w.Code != 401) {
			t.Fatal("cross-origin cookie authentication accepted")
		}
		if bearer && (done || r.Header.Get("Cookie") != "") {
			t.Fatal("explicit client credentials did not exclude cookies")
		}
	}
	for _, value := range []string{"null", "http://insecure.example", "https://user:pass@client.example", "https://client.example/path"} {
		r := httptest.NewRequest("OPTIONS", "https://runtime.example/oauth/token", nil)
		r.Header.Set("Origin", value)
		w := httptest.NewRecorder()
		if !clientCORS(w, r) || w.Code != 403 {
			t.Fatalf("unsafe origin %s accepted", value)
		}
	}
}
