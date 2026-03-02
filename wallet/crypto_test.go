package wallet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/app"
)

// TestRPCCallRetries verifies that rpcCall retries on transient errors.
func TestRPCCallRetries(t *testing.T) {
	tests := []struct {
		name       string
		responses  []string // successive responses from the server
		wantErr    bool
		wantResult string
		wantCalls  int
	}{
		{
			name: "succeeds on first attempt",
			responses: []string{
				`{"jsonrpc":"2.0","id":1,"result":"0x1"}`,
			},
			wantErr:    false,
			wantResult: `"0x1"`,
			wantCalls:  1,
		},
		{
			name: "retries on invalid JSON and succeeds",
			responses: []string{
				`error: no backend is currently healthy`,
				`{"jsonrpc":"2.0","id":1,"result":"0x2"}`,
			},
			wantErr:    false,
			wantResult: `"0x2"`,
			wantCalls:  2,
		},
		{
			name: "retries on transient rpc error and succeeds",
			responses: []string{
				`{"jsonrpc":"2.0","id":1,"error":{"message":"no backend is currently healthy to serve"}}`,
				`{"jsonrpc":"2.0","id":1,"result":"0x3"}`,
			},
			wantErr:    false,
			wantResult: `"0x3"`,
			wantCalls:  2,
		},
		{
			name: "does not retry on permanent rpc error",
			responses: []string{
				`{"jsonrpc":"2.0","id":1,"error":{"message":"method not found"}}`,
				`{"jsonrpc":"2.0","id":1,"result":"0x4"}`,
			},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name: "fails after all retries exhausted",
			responses: []string{
				`error: no backend`,
				`error: no backend`,
				`error: no backend`,
			},
			wantErr:   true,
			wantCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := callCount
				callCount++
				if idx < len(tt.responses) {
					fmt.Fprint(w, tt.responses[idx])
				} else {
					fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":null}`)
				}
			}))
			defer srv.Close()

			// Temporarily override the chain RPC URL
			original := chainRPCs["ethereum"]
			chainRPCs["ethereum"] = srv.URL
			defer func() { chainRPCs["ethereum"] = original }()

			result, err := rpcCall("ethereum", "eth_blockNumber", []interface{}{})

			if (err != nil) != tt.wantErr {
				t.Errorf("rpcCall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(result) != tt.wantResult {
				t.Errorf("rpcCall() result = %s, want %s", string(result), tt.wantResult)
			}
			if callCount != tt.wantCalls {
				t.Errorf("rpcCall() made %d calls, want %d", callCount, tt.wantCalls)
			}
		})
	}
}

// TestRPCCallRateLimitFallback verifies that rpcCall rotates to a fallback endpoint on HTTP 429.
func TestRPCCallRateLimitFallback(t *testing.T) {
	// Primary server always returns 429 Too Many Requests.
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer primarySrv.Close()

	// Fallback server returns a successful response.
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`)
	}))
	defer fallbackSrv.Close()

	originalPrimary := chainRPCs["ethereum"]
	originalFallbacks := chainFallbackRPCs["ethereum"]
	chainRPCs["ethereum"] = primarySrv.URL
	chainFallbackRPCs["ethereum"] = []string{fallbackSrv.URL}
	defer func() {
		chainRPCs["ethereum"] = originalPrimary
		chainFallbackRPCs["ethereum"] = originalFallbacks
	}()

	result, err := rpcCall("ethereum", "eth_blockNumber", []interface{}{})
	if err != nil {
		t.Fatalf("expected success after fallback, got error: %v", err)
	}
	if string(result) != `"0x1"` {
		t.Errorf("rpcCall() result = %s, want \"0x1\"", string(result))
	}
}

// newRPCDispatchServer creates a test HTTP server that dispatches JSON-RPC
// responses based on the method name.
func newRPCDispatchServer(methodResponses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":null}`)
			return
		}
		if resp, ok := methodResponses[req.Method]; ok {
			fmt.Fprint(w, resp)
			return
		}
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":null}`)
	}))
}

// TestVerifyAndCreditDeposit covers the early-exit error paths of
// VerifyAndCreditDeposit that do not require a live wallet seed.
func TestVerifyAndCreditDeposit(t *testing.T) {
	const validHash = "0x" + "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	t.Run("unsupported chain", func(t *testing.T) {
		_, err := VerifyAndCreditDeposit("polygon", validHash, "user1")
		if err == nil || !strings.Contains(err.Error(), "unsupported chain") {
			t.Errorf("expected unsupported chain error, got: %v", err)
		}
	})

	t.Run("transaction not found", func(t *testing.T) {
		srv := newRPCDispatchServer(map[string]string{
			"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
		})
		defer srv.Close()

		original := chainRPCs["ethereum"]
		chainRPCs["ethereum"] = srv.URL
		defer func() { chainRPCs["ethereum"] = original }()

		_, err := VerifyAndCreditDeposit("ethereum", validHash, "user1")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected transaction not found error, got: %v", err)
		}
	})

	t.Run("transaction not yet confirmed", func(t *testing.T) {
		srv := newRPCDispatchServer(map[string]string{
			"eth_getTransactionByHash":   `{"jsonrpc":"2.0","id":1,"result":{"to":"0xdeadbeef","value":"0xDE0B6B3A7640000"}}`,
			"eth_getTransactionReceipt": `{"jsonrpc":"2.0","id":1,"result":null}`,
		})
		defer srv.Close()

		original := chainRPCs["ethereum"]
		chainRPCs["ethereum"] = srv.URL
		defer func() { chainRPCs["ethereum"] = original }()

		_, err := VerifyAndCreditDeposit("ethereum", validHash, "user1")
		if err == nil || !strings.Contains(err.Error(), "not yet confirmed") {
			t.Errorf("expected not yet confirmed error, got: %v", err)
		}
	})

	t.Run("transaction failed on chain", func(t *testing.T) {
		srv := newRPCDispatchServer(map[string]string{
			"eth_getTransactionByHash":   `{"jsonrpc":"2.0","id":1,"result":{"to":"0xdeadbeef","value":"0xDE0B6B3A7640000"}}`,
			"eth_getTransactionReceipt": `{"jsonrpc":"2.0","id":1,"result":{"status":"0x0"}}`,
		})
		defer srv.Close()

		original := chainRPCs["ethereum"]
		chainRPCs["ethereum"] = srv.URL
		defer func() { chainRPCs["ethereum"] = original }()

		_, err := VerifyAndCreditDeposit("ethereum", validHash, "user1")
		if err == nil || !strings.Contains(err.Error(), "did not succeed") {
			t.Errorf("expected transaction failed error, got: %v", err)
		}
	})
}

// TestRPCCallAPILog verifies that rpcCall records every attempt in the API log.
func TestRPCCallAPILog(t *testing.T) {
	t.Run("successful call is logged with request and response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":"0xabc"}`)
		}))
		defer srv.Close()

		original := chainRPCs["ethereum"]
		chainRPCs["ethereum"] = srv.URL
		defer func() { chainRPCs["ethereum"] = original }()

		// Drain existing log entries so we start clean.
		beforeCount := len(app.GetAPILog())

		_, err := rpcCall("ethereum", "eth_blockNumber", []interface{}{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		entries := app.GetAPILog()
		if len(entries) <= beforeCount {
			t.Fatal("expected a new API log entry to be recorded")
		}
		e := entries[0] // GetAPILog returns newest-first
		if e.Service != "ethereum_rpc" {
			t.Errorf("Service = %q, want %q", e.Service, "ethereum_rpc")
		}
		if e.Method != "eth_blockNumber" {
			t.Errorf("Method = %q, want %q", e.Method, "eth_blockNumber")
		}
		if e.URL != srv.URL {
			t.Errorf("URL = %q, want %q", e.URL, srv.URL)
		}
		if e.Status != 200 {
			t.Errorf("Status = %d, want 200", e.Status)
		}
		if e.Error != "" {
			t.Errorf("Error = %q, want empty", e.Error)
		}
		if !strings.Contains(e.RequestBody, "eth_blockNumber") {
			t.Errorf("RequestBody %q does not contain method name", e.RequestBody)
		}
		if !strings.Contains(e.ResponseBody, "0xabc") {
			t.Errorf("ResponseBody %q does not contain expected result", e.ResponseBody)
		}
	})

	t.Run("non-JSON response is logged with error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `error: service unavailable`)
		}))
		defer srv.Close()

		original := chainRPCs["ethereum"]
		chainRPCs["ethereum"] = srv.URL
		defer func() { chainRPCs["ethereum"] = original }()

		beforeCount := len(app.GetAPILog())

		rpcCall("ethereum", "eth_blockNumber", []interface{}{}) //nolint:errcheck

		entries := app.GetAPILog()
		if len(entries) <= beforeCount {
			t.Fatal("expected API log entries to be recorded")
		}
		// At least one entry should have a non-empty error and the raw response body.
		found := false
		for _, e := range entries {
			if e.Service == "ethereum_rpc" && e.Error != "" && strings.Contains(e.ResponseBody, "error: service unavailable") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected an API log entry with error and raw response body for non-JSON response")
		}
	})

	t.Run("rpc error response is logged", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"message":"method not found"}}`)
		}))
		defer srv.Close()

		original := chainRPCs["ethereum"]
		chainRPCs["ethereum"] = srv.URL
		defer func() { chainRPCs["ethereum"] = original }()

		beforeCount := len(app.GetAPILog())

		_, err := rpcCall("ethereum", "eth_unknown", []interface{}{})
		if err == nil {
			t.Fatal("expected an error for rpc error response")
		}

		entries := app.GetAPILog()
		if len(entries) <= beforeCount {
			t.Fatal("expected an API log entry to be recorded")
		}
		e := entries[0]
		if !strings.Contains(e.Error, "method not found") {
			t.Errorf("Error = %q, want to contain %q", e.Error, "method not found")
		}
		if !strings.Contains(e.ResponseBody, "method not found") {
			t.Errorf("ResponseBody %q does not contain error message", e.ResponseBody)
		}
	})
}

