package wallet

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
