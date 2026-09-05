package server

import (
	"mu/internal/api"
	"mu/internal/x402"
)

func init() {
	x402.BazaarLookup = func(op string) (string, string, map[string]any, bool) {
		t, ok := api.ToolForWalletOp(op)
		if !ok {
			return "", "", nil, false
		}
		return t.Name, t.Description, api.ToolSchema(t), true
	}
}
