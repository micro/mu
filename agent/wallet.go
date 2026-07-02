package agent

import (
	"encoding/json"
	"net/http"

	"mu/internal/auth"
	"mu/wallet"
)

// WalletHandler returns the logged-in user's Base wallet (address + USDC
// balance), creating one on first use. Auth required. Backs the wallet panel on
// /agent, which shows a fund-me QR and the live balance.
func WalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, acc := auth.TrySession(r)
	if acc == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "login required"})
		return
	}
	// POST toggles pay-with-wallet mode (crypto=on|off).
	if r.Method == http.MethodPost {
		wallet.SetPayWithWallet(acc.ID, r.FormValue("crypto") == "on")
	}
	bw, err := wallet.GetOrCreateWallet(acc.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	usdc, _ := wallet.USDCBalance(bw.Address)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"address":       bw.Address,
		"usdc":          usdc,
		"network":       "base",
		"payWithWallet": wallet.PayWithWallet(acc.ID),
	})
}
