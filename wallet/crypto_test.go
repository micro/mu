package wallet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

