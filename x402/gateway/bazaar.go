package gateway

import (
	"mu/internal/api"
	x402 "mu/x402/payment"
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
